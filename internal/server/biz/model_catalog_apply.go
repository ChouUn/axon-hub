// 职责: 将已预览的目录模型和价格选择逐行写入全局模型库。
// 关联文件:
//
//	internal/server/biz/model_catalog_preview.go
//	internal/server/biz/model.go
//	internal/objects/model.go
//
// 注意事项:
//
//	新建模型须在行事务内复核公开请求名；单行冲突不撤销其他行，更新价格保留已有渠道关联。
//	目录报价按模型整体写入 ModelCard.price，并保存一条来源绑定。

package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"modernc.org/sqlite"

	sqlite3 "modernc.org/sqlite/lib"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/ent/system"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/llm"
)

var (
	errCatalogConflict     = errors.New("model changed since preview")
	errCatalogQuoteChanged = errors.New("catalog offer changed since preview")
	errCatalogSelection    = errors.New("invalid catalog selection")
)

// SQLite cannot upgrade a stale read snapshot to a writer after another connection commits.
func catalogWriteBusy(err error) bool {
	var e *sqlite.Error
	if !errors.As(err, &e) {
		return false
	}
	code := e.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}

// ApplyCatalogImport re-derives the prices from the reviewed catalog. The
// client supplies choices and versions, never a replacement price snapshot.
func (svc *ModelService) ApplyCatalogImport(ctx context.Context, scope CatalogImportScope, revision string, selections []CatalogImportSelection) ([]CatalogImportOutcome, error) {
	snapshot, err := svc.catalog.snapshot(ctx, false, true)
	if err != nil {
		return nil, err
	}
	preview, err := svc.catalogPreviewFromSnapshot(ctx, scope, snapshot)
	if err != nil {
		return nil, err
	}
	rows := map[string]*CatalogImportRow{}
	for _, row := range preview.Rows {
		rows[row.ModelID] = row
		for _, id := range row.Members {
			rows[id.ID] = row
		}
	}
	// Pin the batch to the reviewed source after the download, before writing any row.
	currentURL, err := svc.catalog.currentUpstreamURL(ctx)
	if err != nil {
		return nil, err
	}
	sourceChanged := revision != preview.Revision || snapshot.upstreamURL != currentURL
	used := map[string]bool{}
	result := make([]CatalogImportOutcome, 0, len(selections))
	for _, selection := range selections {
		status := CatalogApplied
		switch {
		case slices.ContainsFunc(selection.Members, func(m CatalogImportMember) bool { return used[m.ID] }):
			status = CatalogConflict
		case sourceChanged:
			status = CatalogChanged
		default:
			err := svc.RunInTransaction(ctx, func(ctx context.Context) error { return svc.applyCatalogRow(ctx, preview.Source, rows, selection) })
			switch {
			case errors.Is(err, errCatalogQuoteChanged):
				status = CatalogChanged
			case errors.Is(err, errCatalogSelection):
				status = CatalogInvalid
			case errors.Is(err, errCatalogConflict), ent.IsConstraintError(err), ent.IsNotFound(err), catalogWriteBusy(err):
				status = CatalogConflict
			case err != nil:
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				log.Error(ctx, "failed to import catalog model", log.String("model_id", selection.ModelID), log.Cause(err))
				status = CatalogFailed
			}
		}
		if status == CatalogApplied {
			for _, id := range selection.Members {
				used[id.ID] = true
			}
		}
		result = append(result, CatalogImportOutcome{ModelID: selection.ModelID, Status: status})
	}
	return result, nil
}

func (svc *ModelService) applyCatalogRow(ctx context.Context, source string, rows map[string]*CatalogImportRow, sel CatalogImportSelection) error {
	if len(sel.Members) == 0 || strings.TrimSpace(sel.ModelID) == "" {
		return errCatalogSelection
	}
	if sel.Version != "" {
		return svc.updateCatalogPrice(ctx, source, rows[sel.ModelID], sel)
	}
	var parts []*CatalogImportRow
	submitted := map[string]CatalogImportMember{}
	for _, member := range sel.Members {
		if _, duplicate := submitted[member.ID]; duplicate {
			return errCatalogSelection
		}
		if rows[member.ID] == nil {
			return errCatalogConflict
		}
		submitted[member.ID] = member
		parts = append(parts, rows[member.ID])
	}
	parts = lo.Uniq(parts)
	if len(parts) > 1 && !sel.ManualMerge {
		return errCatalogConflict
	}
	choices := map[catalogOfferKey]*CatalogImportOffer{}
	reviewedChannels := map[int]bool{}
	memberCount := 0
	for _, row := range parts {
		if row.Version != "" || (row.ModelID != sel.ModelID && row.MergeTargetOccupied) {
			return errCatalogConflict
		}
		if row.ModelID != sel.ModelID && (len(parts) < 2 || row.MergeTarget != sel.ModelID) {
			return errCatalogSelection
		}
		memberCount += len(row.Members)
		for _, member := range row.Members {
			original, present := submitted[member.ID]
			if !present || (len(sel.Members) > 1 && original != member) {
				return errCatalogConflict
			}
		}
		for _, o := range row.Offers {
			choices[catalogOfferKey{o.Provider, o.ModelID}] = o
		}
		for _, route := range row.Routes {
			reviewedChannels[route.ChannelID] = true
		}
	}
	if memberCount != len(sel.Members) || (len(parts) > 1 && rows[sel.ModelID] != nil && !slices.Contains(parts, rows[sel.ModelID])) {
		return errCatalogSelection
	}
	db := svc.entFromContext(ctx)
	if strings.TrimSpace(sel.Name) == "" || strings.TrimSpace(sel.Developer) == "" {
		return errCatalogSelection
	}
	primary := choices[catalogOfferKey{sel.Provider, sel.SourceModelID}]
	if len(choices) > 1 && sel.Provider == "" {
		for _, offer := range choices {
			_, byID := submitted[offer.ModelID]
			_, byCanonical := submitted[offer.Canonical]
			if byID || byCanonical {
				return errCatalogSelection
			}
		}
	}
	if sel.Provider != "" && (primary == nil || primary.Identity != sel.Identity) {
		return errCatalogQuoteChanged
	}
	card, err := newImportCard(primary)
	if err != nil {
		return err
	}
	ids := sortedStringKeys(submitted)
	if len(sel.Members) > 1 {
		card.CatalogAliases = ids
	}
	var priceOffer *CatalogImportOffer
	if sel.Price != nil {
		priceOffer = choices[catalogOfferKey{sel.Price.Provider, sel.Price.ModelID}]
	}
	if err := applyCatalogPrice(card, priceOffer, sel.Price, false); err != nil {
		return err
	}
	settings := &objects.ModelSettings{LoadBalancerStrategy: "default", TraceStickyMode: "default"}
	for _, choice := range sel.Routes {
		if _, present := submitted[choice.Actual]; !present || !reviewedChannels[choice.ChannelID] {
			return errCatalogConflict
		}
	}
	if err := claimCatalogModelIDs(ctx, db, lo.Uniq(append(ids, sel.ModelID))); err != nil {
		return err
	}
	associations, err := catalogAssociations(ctx, db, sel)
	if err != nil {
		return err
	}
	settings.Associations = associations
	typ := model.Type(sel.Type)
	if model.TypeValidator(typ) != nil {
		return errCatalogSelection
	}
	group := sel.Developer
	if primary != nil {
		if family := modelString(primary.entry, "family"); family != "" {
			group = family
		}
	}
	return db.Model.Create().SetDeveloper(sel.Developer).SetModelID(sel.ModelID).SetName(sel.Name).SetGroup(group).SetIcon(sel.Developer).SetType(typ).SetSettings(settings).SetModelCard(card).Exec(ctx)
}

func (svc *ModelService) updateCatalogPrice(ctx context.Context, source string, row *CatalogImportRow, sel CatalogImportSelection) error {
	db := svc.entFromContext(ctx)
	current, err := lockedModel(ctx, db, db.Model.Query().Where(model.ModelIDEQ(sel.ModelID)))
	if err != nil {
		return err
	}
	version, err := ModelEditVersion(current)
	if err != nil {
		return err
	}
	if version != sel.Version || current.Status == model.StatusArchived || source == catalogSourceFallback {
		return errCatalogConflict
	}
	if sel.Price == nil {
		return nil
	}
	var offer *CatalogImportOffer
	if row != nil && row.ModelID == sel.ModelID {
		for _, o := range row.Offers {
			if o.Provider == sel.Price.Provider && o.ModelID == sel.Price.ModelID {
				offer = o
				break
			}
		}
	}
	var previous objects.ModelCard
	if current.ModelCard != nil {
		previous = *current.ModelCard
	}
	next := previous
	if previous.CatalogPrice != nil {
		binding := *previous.CatalogPrice
		next.CatalogPrice = &binding
	}
	if err := applyCatalogPrice(&next, offer, sel.Price, true); err != nil {
		return err
	}
	if equalModelPrices(previous.Price, next.Price) && reflect.DeepEqual(previous.CatalogPrice, next.CatalogPrice) {
		return nil
	}
	return db.Model.UpdateOneID(current.ID).SetModelCard(&next).Exec(ctx)
}

// Serialize catalog creates before checking both model IDs and aliases stored in JSON.
func claimCatalogModelIDs(ctx context.Context, db *ent.Client, ids []string) error {
	_, err := authz.RunAsHost(ctx, db, "serialize-catalog-import", func(hostCtx context.Context) (struct{}, error) {
		return struct{}{}, db.System.Create().SetKey("model_catalog_import_lock").SetValue(uuid.NewString()).
			OnConflictColumns(system.FieldKey).UpdateValue().Exec(hostCtx)
	})
	if err != nil {
		return fmt.Errorf("claim catalog import: %w", err)
	}
	query := db.Model.Query().Where(func(s *sql.Selector) {
		predicates := []*sql.Predicate{sql.In(s.C(model.FieldModelID), lo.ToAnySlice(ids)...)}
		for _, id := range ids {
			predicates = append(predicates, sqljson.ValueContains(s.C(model.FieldModelCard), id, sqljson.Path("catalogAliases")))
		}
		s.Where(sql.Or(predicates...))
		if db.Driver().Dialect() == dialect.MySQL {
			s.ForUpdate()
		}
	})
	occupied, err := query.Exist(ctx)
	if err != nil {
		return fmt.Errorf("check catalog model IDs: %w", err)
	}
	if occupied {
		return errCatalogConflict
	}
	return nil
}

// Resolve each reviewed (channel, upstream model) against its currently exposed request names.
func catalogAssociations(ctx context.Context, db *ent.Client, sel CatalogImportSelection) ([]*objects.ModelAssociation, error) {
	type key struct {
		channel int
		actual  string
	}
	choices := map[key]string{}
	channelIDs := make([]int, 0, len(sel.Routes))
	for _, choice := range sel.Routes {
		choices[key{choice.ChannelID, choice.Actual}] = choice.Request
		channelIDs = append(channelIDs, choice.ChannelID)
	}
	if len(choices) != len(sel.Routes) {
		return nil, errCatalogConflict
	}
	var current map[string][]CatalogImportRoute
	if len(channelIDs) > 0 {
		query := db.Channel.Query().Where(channel.IDIn(lo.Uniq(channelIDs)...))
		if db.Driver().Dialect() != dialect.SQLite {
			query.Modify(func(s *sql.Selector) { s.ForUpdate() })
		}
		var err error
		current, err = catalogChannelRoutes(ctx, query)
		if err != nil {
			return nil, err
		}
	}
	associations := []*objects.ModelAssociation{}
	for _, member := range sel.Members {
		routes := current[member.ID]
		for _, route := range routes {
			k := key{route.ChannelID, route.Actual}
			request, ok := choices[k]
			if !ok {
				continue
			}
			delete(choices, k)
			if request == "" && len(route.Requests) == 0 {
				continue
			}
			if !slices.Contains(route.Requests, request) {
				return nil, errCatalogConflict
			}
			associations = append(associations, &objects.ModelAssociation{Type: "channel_model", ChannelModel: &objects.ChannelModelAssociation{ChannelID: route.ChannelID, ModelID: request}})
		}
	}
	if len(choices) > 0 {
		return nil, errCatalogConflict
	}
	return associations, nil
}

func applyCatalogPrice(card *objects.ModelCard, offer *CatalogImportOffer, input *CatalogImportPriceInput, existing bool) error {
	if input == nil {
		return nil
	}
	switch input.Action {
	case CatalogPriceFix:
		if !existing {
			return errCatalogSelection
		}
		if card.CatalogPrice != nil {
			card.CatalogPrice.Follow = false
		}
		return nil
	case CatalogPriceFollow, CatalogPriceRebind, CatalogPriceFixSource:
		if offer == nil || offer.Identity != input.Identity || offer.PriceDigest != input.ExpectedDigest {
			return errCatalogQuoteChanged
		}
		if offer.Price == nil {
			return errCatalogSelection
		}
		binding := card.CatalogPrice
		if input.Action == CatalogPriceFollow && binding != nil && binding.Follow &&
			(binding.Provider != offer.Provider || binding.ModelID != offer.ModelID || binding.Canonical != offer.Canonical) {
			return errCatalogQuoteChanged
		}
		card.Price = offer.Price
		card.CatalogPrice = &objects.CatalogPrice{
			Provider:  offer.Provider,
			ModelID:   offer.ModelID,
			Canonical: offer.Canonical,
			Follow:    input.Action != CatalogPriceFixSource,
		}
		return nil
	default:
		return errCatalogSelection
	}
}

func newImportCard(o *CatalogImportOffer) (*objects.ModelCard, error) {
	card := &objects.ModelCard{}
	if o == nil {
		return card, nil
	}
	// Catalog keys shared with the global card include modalities, limit and reasoning.
	var metadata struct {
		Reasoning        objects.ModelCardReasoning  `json:"reasoning"`
		Modalities       objects.ModelCardModalities `json:"modalities"`
		Limit            objects.ModelCardLimit      `json:"limit"`
		Vision           *bool                       `json:"vision"`
		Temperature      bool                        `json:"temperature"`
		Knowledge        string                      `json:"knowledge"`
		ToolCall         bool                        `json:"tool_call"`
		ReleaseDate      string                      `json:"release_date"`
		LastUpdated      string                      `json:"last_updated"`
		ReasoningOptions []struct {
			Type   string   `json:"type"`
			Values []string `json:"values"`
		} `json:"reasoning_options"`
	}
	raw, err := json.Marshal(o.entry)
	if err != nil {
		return nil, fmt.Errorf("encode catalog model metadata: %w", err)
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode catalog model metadata: %w", err)
	}
	card.Reasoning, card.Modalities, card.Limit = metadata.Reasoning, metadata.Modalities, metadata.Limit
	card.Vision = slices.Contains(card.Modalities.Input, "image")
	if metadata.Vision != nil {
		card.Vision = *metadata.Vision
	}
	card.Temperature, card.ToolCall, card.Knowledge = metadata.Temperature, metadata.ToolCall, metadata.Knowledge
	card.ReleaseDate, card.LastUpdated = metadata.ReleaseDate, metadata.LastUpdated
	for _, option := range metadata.ReasoningOptions {
		if option.Type == "effort" {
			for _, effort := range []string{llm.ReasoningEffortNone, llm.ReasoningEffortMinimal, llm.ReasoningEffortLow, llm.ReasoningEffortMedium, llm.ReasoningEffortHigh, llm.ReasoningEffortXHigh, llm.ReasoningEffortMax} {
				if slices.Contains(option.Values, effort) {
					card.ReasoningEfforts = append(card.ReasoningEfforts, effort)
				}
			}
			break
		}
	}
	return card, nil
}
