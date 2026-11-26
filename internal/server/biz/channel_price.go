// 职责: 管理渠道模型定价，并从共享 Model 索引缓存解析回退价格。
// 关联文件:
//
//	internal/server/biz/channel.go
//	internal/server/biz/model_cache.go
//	internal/server/biz/usage_log.go
//	llm/reasoning.go
//
// 注意事项:
//
//	精确模型优先于去推理强度后缀的基础模型；归档模型不参与价格回退。
//	去后缀规则与自动推理强度共用 llm.SplitReasoningEffortModelSuffix。
//	请求内价格复用保持费用注入与 usage 记录一致，查询错误不缓存。
//	端点同步仅覆盖已报价的基础费率，保留阶梯和时段价格，按渠道当前倍率生成版本。
//	合并后不符合定价约束的报价会跳过，首次建价可回退到模型库价格。
//	模型库补项只在补项后仍满足阶梯约束时采用，否则只叠加端点报价。

package biz

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/channelmodelprice"
	"github.com/looplj/axonhub/internal/ent/channelmodelpriceversion"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xtime"
	"github.com/looplj/axonhub/internal/scopes"
	"github.com/looplj/axonhub/llm"
)

type SaveChannelModelPriceInput struct {
	ModelID string             `json:"modelId"`
	Price   objects.ModelPrice `json:"price"`
}

type ActionType string

const (
	ActionTypeCreate ActionType = "create"
	ActionTypeUpdate ActionType = "update"
	ActionTypeDelete ActionType = "delete"
	ActionTypeSkip   ActionType = "skip"
)

type PriceChangeAction struct {
	Type          ActionType
	ModelID       string
	Price         objects.ModelPrice
	ExistingPrice *ent.ChannelModelPrice // nil if create
}

const channelModelPriceQueryBatchSize = 500

type channelModelPriceTemplate struct {
	ModelID string
	Price   objects.ModelPrice
}

func uniqueNonEmptyModelIDs(modelIDs []string) []string {
	return lo.Uniq(lo.Filter(modelIDs, func(modelID string, _ int) bool {
		return modelID != ""
	}))
}

func mergeChannelModelPriceItems(base, overlay objects.ModelPrice, replaceExisting bool) objects.ModelPrice {
	if len(overlay.Items) == 0 {
		return base
	}

	positions := make(map[objects.PriceItemCode]int, len(base.Items)+len(overlay.Items))
	items := make([]objects.ModelPriceItem, 0, len(base.Items)+len(overlay.Items))
	for _, item := range base.Items {
		positions[item.ItemCode] = len(items)
		items = append(items, item)
	}
	for _, item := range overlay.Items {
		position, exists := positions[item.ItemCode]
		if exists {
			if replaceExisting {
				items[position] = item
			}
			continue
		}
		positions[item.ItemCode] = len(items)
		items = append(items, item)
	}

	base.Items = items
	return base
}

func mergeEndpointChannelModelPrice(base, endpointPrice objects.ModelPrice, libraryPrice *objects.ModelPrice) objects.ModelPrice {
	merged := mergeChannelModelPriceItems(base, endpointPrice, true)
	merged.Multiplier = endpointPrice.Multiplier
	if libraryPrice == nil {
		return merged
	}
	// Library gap codes extend only the base items; existing volume tiers do not
	// carry them, so keep the endpoint overlay when the gap fill breaks the tiers.
	filled := mergeChannelModelPriceItems(merged, *libraryPrice, false)
	if len(filled.VolumeTiers) != 0 && filled.Validate() != nil && merged.Validate() == nil {
		return merged
	}
	return filled
}

// endpointChanged checks that a merged endpoint quote would produce a valid new snapshot.
func endpointChanged(ctx context.Context, existing *ent.ChannelModelPrice, merged objects.ModelPrice) bool {
	// Price equality treats nil and an explicit multiplier of one as equivalent.
	// Sync snapshots the actual channel setting.
	sameMultiplierPresence := (existing.Price.Multiplier == nil) == (merged.Multiplier == nil)
	if sameMultiplierPresence && existing.Price.Equals(merged) {
		return false
	}
	return validAutoChannelModelPrice(ctx, existing.ChannelID, existing.ModelID, merged)
}

func validAutoChannelModelPrice(ctx context.Context, channelID int, modelID string, price objects.ModelPrice) bool {
	if err := price.Validate(); err != nil {
		log.Warn(ctx, "skipping invalid automatic channel model price",
			log.Int("channel_id", channelID),
			log.String("model_id", modelID),
			log.Cause(err))
		return false
	}
	return true
}

func modelCardToChannelModelPrice(card *objects.ModelCard) (objects.ModelPrice, bool) {
	if card == nil || card.Price == nil {
		return objects.ModelPrice{}, false
	}
	if len(card.Price.Items) == 0 && len(card.Price.VolumeTiers) == 0 &&
		(card.Price.Schedule == nil || len(card.Price.Schedule.Overrides) == 0) {
		return objects.ModelPrice{}, false
	}

	return *card.Price, true
}

// modelLibraryPriceReferencePrefix marks usage-log cost that fell back to the
// model library price because the channel had no explicit price for the model.
const modelLibraryPriceReferencePrefix = "model_library:"

// modelLibraryPriceReferenceID builds the synthetic reference id recorded on
// usage logs whose cost was resolved from the model library fallback.
func modelLibraryPriceReferenceID(modelID string) string {
	return modelLibraryPriceReferencePrefix + modelID
}

// modelLibraryPriceMultiplierReferencePrefix is the fork's multiplier-bearing
// fallback format. Fallback prices have no price-version snapshot.
const modelLibraryPriceMultiplierReferencePrefix = "model_library@"

// modelLibraryPriceReferenceIDWithMultiplier snapshots the channel multiplier in
// the fork's fallback reference because fallback has no price-version snapshot.
func modelLibraryPriceReferenceIDWithMultiplier(modelID string, multiplier *decimal.Decimal) string {
	if multiplier == nil || multiplier.Equal(decimal.NewFromInt(1)) {
		return modelLibraryPriceReferenceID(modelID)
	}
	return modelLibraryPriceMultiplierReferencePrefix + multiplier.String() + ":" + modelID
}

// modelLibraryPriceReferenceMultiplier reads the fork's fallback multiplier
// snapshot. Malformed fallback references retain the legacy unit multiplier.
func modelLibraryPriceReferenceMultiplier(referenceID string) (decimal.Decimal, bool) {
	one := decimal.NewFromInt(1)
	if strings.HasPrefix(referenceID, modelLibraryPriceReferencePrefix) {
		return one, true
	}
	encoded, ok := strings.CutPrefix(referenceID, modelLibraryPriceMultiplierReferencePrefix)
	if !ok {
		return one, false
	}
	value, modelID, ok := strings.Cut(encoded, ":")
	if !ok || modelID == "" {
		return one, true
	}
	multiplier, err := decimal.NewFromString(value)
	if err != nil || multiplier.IsNegative() {
		return one, true
	}
	return multiplier, true
}

// modelLibraryPriceModelIDCandidates returns the model ids to try, in order,
// when resolving a model library price. The exact id is tried first, then the
// base model with a reasoning effort suffix stripped (e.g.
// "gemini-3.8-flash-high" -> "gemini-3.8-flash"). Providers expose the same
// underlying model under several suffixed names, and the model library usually
// only carries the base id. The suffix rule is llm.SplitReasoningEffortModelSuffix,
// shared with auto reasoning effort and channel effort-suffix aliases.
func modelLibraryPriceModelIDCandidates(modelID string) []string {
	if base, _, ok := llm.SplitReasoningEffortModelSuffix(modelID); ok {
		return []string{modelID, base}
	}

	return []string{modelID}
}

// modelLibraryPriceMemo caches model-library price resolutions for the duration
// of a single request. Cost injection and usage-log creation both resolve the
// price for the same channel and model; without the memo they repeat the
// catalog lookup, adding avoidable synchronous database reads on the response
// path (and once per usage-bearing chunk for streams).
type modelLibraryPriceMemo struct {
	mu      sync.RWMutex
	entries map[string]modelLibraryPriceMemoEntry
}

type modelLibraryPriceMemoEntry struct {
	price *objects.ModelPrice
	found bool
}

type modelLibraryPriceMemoKey struct{}

// WithModelLibraryPriceMemo installs a request-scoped model-library price memo.
// Call it once per inbound request so repeated cost calculations reuse a single
// lookup. It is idempotent: the memo already present (if any) is kept.
func WithModelLibraryPriceMemo(ctx context.Context) context.Context {
	if _, ok := ctx.Value(modelLibraryPriceMemoKey{}).(*modelLibraryPriceMemo); ok {
		return ctx
	}

	return context.WithValue(ctx, modelLibraryPriceMemoKey{}, &modelLibraryPriceMemo{})
}

func modelLibraryPriceMemoFromContext(ctx context.Context) *modelLibraryPriceMemo {
	memo, _ := ctx.Value(modelLibraryPriceMemoKey{}).(*modelLibraryPriceMemo)

	return memo
}

func (m *modelLibraryPriceMemo) get(modelID string) (*objects.ModelPrice, bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[modelID]
	if !ok {
		return nil, false, false
	}

	return entry.price, entry.found, true
}

func (m *modelLibraryPriceMemo) set(modelID string, price *objects.ModelPrice, found bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entries == nil {
		m.entries = make(map[string]modelLibraryPriceMemoEntry)
	}

	m.entries[modelID] = modelLibraryPriceMemoEntry{price: price, found: found}
}

// resolveModelLibraryPrice resolves a price from the model library by model id.
// It is used as a billing-time fallback when a channel has no explicit price
// for the model. The exact model id is matched first; when it has no priceable
// entry, a known effort/tier suffix is stripped and the base model is tried.
// Only enabled and disabled models are considered, mirroring the
// auto-configuration eligibility rules. It returns false when no priceable
// model library entry exists.
func (svc *ChannelService) resolveModelLibraryPrice(ctx context.Context, modelID string) (*objects.ModelPrice, bool) {
	if modelID == "" {
		return nil, false
	}

	memo := modelLibraryPriceMemoFromContext(ctx)
	if memo != nil {
		if price, found, cached := memo.get(modelID); cached {
			return price, found
		}
	}

	price, found, err := svc.lookupModelLibraryPrice(ctx, modelID)
	if err != nil {
		// The lookup failed (e.g. canceled context or a transient database
		// error). Do not cache the result: a later calculation with a healthy
		// context (the usage-log write runs with cancellation removed) must be
		// able to resolve the price instead of reusing this failed attempt.
		return nil, false
	}

	if memo != nil {
		memo.set(modelID, price, found)
	}

	return price, found
}

// lookupModelLibraryPrice resolves candidates against the shared Model indexed cache.
// Transactional callers read their own database state without publishing it. The returned
// error separates a genuine miss (nil error, found=false) from a failed lookup
// (non-nil error), so callers can avoid caching results affected by an error.
func (svc *ChannelService) lookupModelLibraryPrice(ctx context.Context, modelID string) (*objects.ModelPrice, bool, error) {
	db := svc.entFromContext(ctx)

	var lookupErr error

	for _, candidate := range modelLibraryPriceModelIDCandidates(modelID) {
		catalogModel, err := authz.RunAsHost(ctx, db, "model-price-fallback", func(hostCtx context.Context) (*ent.Model, error) {
			return svc.modelCache.get(hostCtx, db, candidate)
		})
		if err != nil {
			if !ent.IsNotFound(err) {
				log.Warn(ctx, "failed to resolve model library price",
					log.String("model_id", modelID),
					log.String("candidate_model_id", candidate),
					log.Cause(err))

				lookupErr = fmt.Errorf("failed to query model library price for %q: %w", candidate, err)
			}

			continue
		}

		if catalogModel.Status != model.StatusEnabled && catalogModel.Status != model.StatusDisabled {
			continue
		}

		price, ok := modelCardToChannelModelPrice(catalogModel.ModelCard)
		if !ok {
			continue
		}

		return &price, true, nil
	}

	return nil, false, lookupErr
}

// createChannelModelPriceIfMissing atomically creates a current price when the
// live (channel, model) pair is absent. The candidate reference ID identifies
// whether this transaction won the upsert race, so only the winner creates the
// initial version.
func (svc *ChannelService) createChannelModelPriceIfMissing(
	ctx context.Context,
	channelID int,
	template channelModelPriceTemplate,
	now time.Time,
) (bool, error) {
	candidateReferenceID := generateReferenceID()
	db := svc.entFromContext(ctx)

	err := db.ChannelModelPrice.Create().
		SetChannelID(channelID).
		SetModelID(template.ModelID).
		SetPrice(template.Price).
		SetReferenceID(candidateReferenceID).
		OnConflictColumns(
			channelmodelprice.FieldChannelID,
			channelmodelprice.FieldModelID,
			channelmodelprice.FieldDeletedAt,
		).
		Ignore().
		Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to create channel model price: %w", err)
	}

	entity, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (*ent.ChannelModelPrice, error) {
		return db.ChannelModelPrice.Query().
			Where(
				channelmodelprice.ChannelID(channelID),
				channelmodelprice.ModelID(template.ModelID),
			).
			Only(queryCtx)
	})
	if err != nil {
		return false, fmt.Errorf("failed to load channel model price after upsert: %w", err)
	}

	if entity.ReferenceID != candidateReferenceID {
		return false, nil
	}

	if err := svc.createChannelModelPriceVersion(ctx, entity, now); err != nil {
		return false, err
	}

	return true, nil
}

func (svc *ChannelService) resolveChannelModelPriceTemplates(
	ctx context.Context,
	modelIDs []string,
) ([]channelModelPriceTemplate, error) {
	modelIDs = uniqueNonEmptyModelIDs(modelIDs)
	if len(modelIDs) == 0 {
		return nil, nil
	}

	db := svc.entFromContext(ctx)
	modelsByID := make(map[string]*ent.Model, len(modelIDs))
	for start := 0; start < len(modelIDs); start += channelModelPriceQueryBatchSize {
		end := min(start+channelModelPriceQueryBatchSize, len(modelIDs))
		catalogModels, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) ([]*ent.Model, error) {
			return db.Model.Query().
				Where(
					model.ModelIDIn(modelIDs[start:end]...),
					model.StatusIn(model.StatusEnabled, model.StatusDisabled),
				).
				All(queryCtx)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query model prices from model library: %w", err)
		}

		for _, catalogModel := range catalogModels {
			modelsByID[catalogModel.ModelID] = catalogModel
		}
	}

	templates := make([]channelModelPriceTemplate, 0, len(modelsByID))
	for _, modelID := range modelIDs {
		catalogModel, exists := modelsByID[modelID]
		if !exists {
			continue
		}

		price, ok := modelCardToChannelModelPrice(catalogModel.ModelCard)
		if !ok {
			continue
		}

		templates = append(templates, channelModelPriceTemplate{
			ModelID: modelID,
			Price:   price,
		})
	}

	return templates, nil
}

// applyChannelModelPriceTemplates must run inside the logical channel mutation
// transaction so each current price and initial version commit together.
func (svc *ChannelService) applyChannelModelPriceTemplates(
	ctx context.Context,
	channelID int,
	templates []channelModelPriceTemplate,
) (bool, error) {
	if len(templates) == 0 {
		return false, nil
	}

	ch, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (*ent.Channel, error) {
		return svc.entFromContext(queryCtx).Channel.Get(queryCtx, channelID)
	})
	if err != nil {
		return false, fmt.Errorf("failed to load channel pricing settings: %w", err)
	}
	var multiplier *decimal.Decimal
	if ch.Settings != nil {
		multiplier = ch.Settings.ModelPriceMultiplier
	}

	changed := false
	now := xtime.UTCNow()
	for _, template := range templates {
		template.Price.Multiplier = multiplier
		created, err := svc.createChannelModelPriceIfMissing(ctx, channelID, template, now)
		if err != nil {
			return false, fmt.Errorf("failed to auto-configure channel model price: model_id=%s: %w", template.ModelID, err)
		}

		changed = changed || created
	}

	return changed, nil
}

func (svc *ChannelService) createChannelModelPriceVersion(
	ctx context.Context,
	entity *ent.ChannelModelPrice,
	now time.Time,
) error {
	_, err := svc.entFromContext(ctx).ChannelModelPriceVersion.Create().
		SetChannelID(entity.ChannelID).
		SetModelID(entity.ModelID).
		SetChannelModelPriceID(entity.ID).
		SetPrice(entity.Price).
		SetStatus(channelmodelpriceversion.StatusActive).
		SetEffectiveStartAt(now).
		SetReferenceID(entity.ReferenceID).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create channel model price version: %w", err)
	}

	return nil
}

// updateChannelModelPriceEntity replaces the current channel price and its active version.
// With replaceExisting it applies the whole price. Otherwise it overlays the endpoint's
// items onto the locked row, fills missing item codes from libraryPrice, and takes the
// channel's multiplier from newPrice. Invalid or equal merged prices skip the write.
// The caller must run it inside the logical channel mutation transaction so
// the price and version history commit together.
func (svc *ChannelService) updateChannelModelPriceEntity(
	ctx context.Context,
	price *ent.ChannelModelPrice,
	newPrice objects.ModelPrice,
	replaceExisting bool,
	libraryPrice *objects.ModelPrice,
	now time.Time,
) (*ent.ChannelModelPrice, bool, error) {
	db := svc.entFromContext(ctx)
	if db.Driver().Dialect() != dialect.SQLite {
		var lockedIDs []int
		_, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (struct{}, error) {
			err := db.ChannelModelPrice.Query().
				Where(channelmodelprice.IDEQ(price.ID)).
				Modify(func(selector *sql.Selector) {
					selector.Select(selector.C(channelmodelprice.FieldID)).ForUpdate()
				}).
				Scan(queryCtx, &lockedIDs)
			return struct{}{}, err
		})
		if err != nil {
			return nil, false, fmt.Errorf("failed to lock channel model price: %w", err)
		}
		if len(lockedIDs) == 0 {
			return nil, false, fmt.Errorf("channel model price %d no longer exists", price.ID)
		}
	}

	lockedPrice, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (*ent.ChannelModelPrice, error) {
		return db.ChannelModelPrice.Query().Where(channelmodelprice.IDEQ(price.ID)).Only(queryCtx)
	})
	if ent.IsNotFound(err) {
		return nil, false, fmt.Errorf("channel model price %d no longer exists", price.ID)
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to load channel model price for update: %w", err)
	}

	if !replaceExisting {
		newPrice = mergeEndpointChannelModelPrice(lockedPrice.Price, newPrice, libraryPrice)
		if !endpointChanged(ctx, lockedPrice, newPrice) {
			return lockedPrice, false, nil
		}
	}

	_, err = db.ChannelModelPriceVersion.Update().
		Where(
			channelmodelpriceversion.ChannelModelPriceIDEQ(lockedPrice.ID),
			channelmodelpriceversion.StatusEQ(channelmodelpriceversion.StatusActive),
		).
		SetStatus(channelmodelpriceversion.StatusArchived).
		SetEffectiveEndAt(now).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to archive old channel model price versions: %w", err)
	}

	updated, err := db.ChannelModelPrice.UpdateOneID(lockedPrice.ID).
		SetPrice(newPrice).
		SetReferenceID(generateReferenceID()).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to update channel model price: %w", err)
	}

	if err := svc.createChannelModelPriceVersion(ctx, updated, now); err != nil {
		return nil, false, err
	}

	return updated, true, nil
}

// createChannelModelPrice creates the current price and its initial active
// version. The caller must run it inside the logical channel mutation
// transaction so the two records cannot be committed independently.
func (svc *ChannelService) createChannelModelPrice(
	ctx context.Context,
	channelID int,
	modelID string,
	price objects.ModelPrice,
	now time.Time,
) (*ent.ChannelModelPrice, error) {
	entity, err := svc.entFromContext(ctx).ChannelModelPrice.Create().
		SetChannelID(channelID).
		SetModelID(modelID).
		SetPrice(price).
		SetReferenceID(generateReferenceID()).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create channel model price: %w", err)
	}

	if err := svc.createChannelModelPriceVersion(ctx, entity, now); err != nil {
		return nil, err
	}

	return entity, nil
}

// ensureChannelModelPrices creates missing prices and fills library gaps for endpoint-priced models.
// Existing item codes are preserved unless the endpoint reported that code.
func (svc *ChannelService) ensureChannelModelPrices(
	ctx context.Context,
	channelID int,
	modelIDs []string,
	endpointTemplateSets ...[]channelModelPriceTemplate,
) (bool, error) {
	modelIDs = uniqueNonEmptyModelIDs(modelIDs)
	if len(modelIDs) == 0 {
		return false, nil
	}

	endpointTemplates := make(map[string]objects.ModelPrice)
	for _, templates := range endpointTemplateSets {
		for _, template := range templates {
			endpointTemplates[template.ModelID] = template.Price
		}
	}

	changed := false
	err := svc.RunInTransaction(ctx, func(ctx context.Context) error {
		db := svc.entFromContext(ctx)
		ch, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (*ent.Channel, error) {
			query := db.Channel.Query().Where(channel.IDEQ(channelID))
			query.Select(channel.FieldSettings)
			// SaveChannelModelPrices locks the channel first as well. Hold the
			// setting stable until the price rows and versions commit.
			if db.Driver().Dialect() != dialect.SQLite {
				query.Modify(func(selector *sql.Selector) { selector.ForUpdate() })
			}
			return query.Only(queryCtx)
		})
		if err != nil {
			return fmt.Errorf("failed to load channel pricing settings: %w", err)
		}
		var multiplier *decimal.Decimal
		if ch.Settings != nil {
			multiplier = ch.Settings.ModelPriceMultiplier
		}

		existingPrices := make(map[string]*ent.ChannelModelPrice, len(modelIDs))
		for start := 0; start < len(modelIDs); start += channelModelPriceQueryBatchSize {
			end := min(start+channelModelPriceQueryBatchSize, len(modelIDs))
			prices, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) ([]*ent.ChannelModelPrice, error) {
				return db.ChannelModelPrice.Query().
					Where(
						channelmodelprice.ChannelID(channelID),
						channelmodelprice.ModelIDIn(modelIDs[start:end]...),
					).
					All(queryCtx)
			})
			if err != nil {
				return fmt.Errorf("failed to query existing channel model prices: %w", err)
			}

			for _, price := range prices {
				existingPrices[price.ModelID] = price
			}
		}

		fallbackModelIDs := make([]string, 0, len(modelIDs))
		for _, modelID := range modelIDs {
			_, hasEndpointPrice := endpointTemplates[modelID]
			_, hasExistingPrice := existingPrices[modelID]
			if hasEndpointPrice || !hasExistingPrice {
				fallbackModelIDs = append(fallbackModelIDs, modelID)
			}
		}

		libraryTemplates, err := svc.resolveChannelModelPriceTemplates(ctx, fallbackModelIDs)
		if err != nil {
			return err
		}
		libraryPrices := make(map[string]objects.ModelPrice, len(libraryTemplates))
		for _, template := range libraryTemplates {
			libraryPrices[template.ModelID] = template.Price
		}

		now := xtime.UTCNow()
		for _, modelID := range modelIDs {
			existing := existingPrices[modelID]
			endpointPrice, hasEndpointPrice := endpointTemplates[modelID]
			libraryPrice, hasLibraryPrice := libraryPrices[modelID]
			if existing == nil && !hasEndpointPrice && !hasLibraryPrice {
				continue
			}
			endpointPrice.Multiplier = multiplier
			libraryPrice.Multiplier = multiplier

			if existing != nil {
				if !hasEndpointPrice {
					// The row is current; library gaps are filled at creation only.
					continue
				}
				// updateChannelModelPriceEntity merges the endpoint items against the
				// locked row, so concurrent edits to unreported items survive, and then
				// fills the library gap for codes neither source reports.
				var libraryOverlay *objects.ModelPrice
				if hasLibraryPrice {
					libraryOverlay = &libraryPrice
				}
				merged := mergeEndpointChannelModelPrice(existing.Price, endpointPrice, libraryOverlay)
				if !endpointChanged(ctx, existing, merged) {
					continue
				}
				_, priceChanged, err := svc.updateChannelModelPriceEntity(ctx, existing, endpointPrice, false, libraryOverlay, now)
				if err != nil {
					return err
				}
				changed = changed || priceChanged
				continue
			}

			mergedPrice := libraryPrice
			if hasEndpointPrice {
				mergedPrice = mergeChannelModelPriceItems(mergedPrice, endpointPrice, true)
			}

			if len(mergedPrice.Items) == 0 && len(mergedPrice.VolumeTiers) == 0 && mergedPrice.Schedule == nil {
				continue
			}
			if !validAutoChannelModelPrice(ctx, channelID, modelID, mergedPrice) {
				if !hasEndpointPrice || !hasLibraryPrice || !validAutoChannelModelPrice(ctx, channelID, modelID, libraryPrice) {
					continue
				}
				mergedPrice = libraryPrice
			}

			created, err := svc.createChannelModelPriceIfMissing(ctx, channelID, channelModelPriceTemplate{
				ModelID: modelID,
				Price:   mergedPrice,
			}, now)
			if err != nil {
				return fmt.Errorf("failed to auto-configure channel model price: model_id=%s: %w", modelID, err)
			}
			changed = changed || created
		}

		return nil
	})

	return changed, err
}

func calculatePriceChanges(prices []*ent.ChannelModelPrice, inputs []SaveChannelModelPriceInput) []PriceChangeAction {
	existingMap := lo.KeyBy(prices, func(p *ent.ChannelModelPrice) string {
		return p.ModelID
	})

	inputSet := make(map[string]struct{}, len(inputs))

	var actions []PriceChangeAction

	// 1. Identify updates and creates
	// Iterate over inputs in order to keep deterministic action ordering.
	for _, input := range inputs {
		modelID := input.ModelID
		inputSet[modelID] = struct{}{}

		existing, ok := existingMap[modelID]
		if !ok {
			actions = append(actions, PriceChangeAction{
				Type:          ActionTypeCreate,
				ModelID:       modelID,
				Price:         input.Price,
				ExistingPrice: nil,
			})
		} else {
			// Only update if price changed
			if existing.Price.Equals(input.Price) {
				actions = append(actions, PriceChangeAction{
					Type:          ActionTypeSkip,
					ModelID:       modelID,
					Price:         input.Price,
					ExistingPrice: existing,
				})
			} else {
				actions = append(actions, PriceChangeAction{
					Type:          ActionTypeUpdate,
					ModelID:       modelID,
					Price:         input.Price,
					ExistingPrice: existing,
				})
			}
		}
	}

	// 2. Identify deletes: present in existing but not in inputs
	for _, existing := range prices {
		if _, ok := inputSet[existing.ModelID]; !ok {
			actions = append(actions, PriceChangeAction{
				Type:          ActionTypeDelete,
				ModelID:       existing.ModelID,
				ExistingPrice: existing,
			})
		}
	}

	return actions
}

func (svc *ChannelService) SaveChannelModelPrices(
	ctx context.Context,
	channelID int,
	inputs []SaveChannelModelPriceInput,
	multiplier *decimal.Decimal,
) ([]*ent.ChannelModelPrice, error) {
	if multiplier != nil && multiplier.IsNegative() {
		return nil, fmt.Errorf("model price multiplier must be non-negative")
	}

	seenModelIDs := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if _, ok := seenModelIDs[input.ModelID]; ok {
			return nil, fmt.Errorf("duplicate model price input: model_id=%s", input.ModelID)
		}

		seenModelIDs[input.ModelID] = struct{}{}

		if err := input.Price.Validate(); err != nil {
			return nil, fmt.Errorf("invalid model price: model_id=%s: %w", input.ModelID, err)
		}
	}

	var (
		results []*ent.ChannelModelPrice
		now     = xtime.UTCNow()
	)

	err := svc.RunInTransaction(ctx, func(ctx context.Context) error {
		db := svc.entFromContext(ctx)
		ch, err := authz.RunWithScopeDecision(ctx, scopes.ScopeWriteChannels, func(queryCtx context.Context) (*ent.Channel, error) {
			return db.Channel.Get(queryCtx, channelID)
		})
		if err != nil {
			return fmt.Errorf("failed to load channel pricing settings: %w", err)
		}
		settings := objects.ChannelSettings{}
		if ch.Settings != nil {
			settings = *ch.Settings
		}
		if multiplier != nil {
			settings.ModelPriceMultiplier = lo.ToPtr(*multiplier)
		}

		// Lock the channel before reading prices, and commit settings and every
		// price version together, even when the submitted price list is empty.
		if err := db.Channel.UpdateOneID(channelID).
			Where(channel.UpdatedAtEQ(ch.UpdatedAt)).
			SetSettings(&settings).
			SetUpdatedAt(now).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to save channel pricing settings: %w", err)
		}

		prices, err := db.ChannelModelPrice.Query().
			Where(channelmodelprice.ChannelID(channelID)).
			All(ctx)
		if err != nil {
			return fmt.Errorf("failed to query existing channel model prices: %w", err)
		}
		versionedInputs := slices.Clone(inputs)
		for idx := range versionedInputs {
			versionedInputs[idx].Price.Multiplier = settings.ModelPriceMultiplier
		}
		actions := calculatePriceChanges(prices, versionedInputs)

		for _, action := range actions {
			var (
				entity *ent.ChannelModelPrice
				err    error
			)

			switch action.Type {
			case ActionTypeSkip:
				results = append(results, action.ExistingPrice)
				continue

			case ActionTypeDelete:
				// Archive old versions
				_, err = db.ChannelModelPriceVersion.Update().
					Where(
						channelmodelpriceversion.ChannelModelPriceIDEQ(action.ExistingPrice.ID),
						channelmodelpriceversion.StatusEQ(channelmodelpriceversion.StatusActive),
					).
					SetStatus(channelmodelpriceversion.StatusArchived).
					SetEffectiveEndAt(now).
					Save(ctx)
				if err != nil {
					return fmt.Errorf("failed to archive channel model price versions for delete: %w", err)
				}

				err = db.ChannelModelPrice.DeleteOne(action.ExistingPrice).Exec(ctx)
				if err != nil {
					return fmt.Errorf("failed to delete channel model price: %w", err)
				}

				continue

			case ActionTypeCreate:
				entity, err = svc.createChannelModelPrice(ctx, channelID, action.ModelID, action.Price, now)
				if err != nil {
					return err
				}

				results = append(results, entity)
				continue

			case ActionTypeUpdate:
				// Editor prices are authoritative for the whole row.
				entity, _, err = svc.updateChannelModelPriceEntity(ctx, action.ExistingPrice, action.Price, true, nil, now)
				if err != nil {
					return err
				}

				results = append(results, entity)
				continue
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Do not expose uncommitted prices through the live cache in an outer transaction.
	svc.reloadChannelsAfterCommit(ctx)

	return results, nil
}

// preloadModelPrices loads active model prices for a channel and caches them.
func (svc *ChannelService) preloadModelPrices(ctx context.Context, ch *Channel) {
	prices, err := svc.entFromContext(ctx).ChannelModelPrice.Query().
		Where(
			channelmodelprice.ChannelID(ch.ID),
			channelmodelprice.DeletedAtEQ(0),
		).
		All(ctx)
	if err != nil {
		log.Warn(ctx, "failed to preload model prices", log.Int("channel_id", ch.ID), log.Cause(err))
		return
	}

	cache := make(map[string]*ent.ChannelModelPrice, len(prices))
	for _, p := range prices {
		cache[p.ModelID] = p
	}

	ch.cachedModelPrices = cache
	if log.DebugEnabled(ctx) {
		log.Debug(ctx, "preloaded model prices", log.Int("channel_id", ch.ID), log.Int("count", len(cache)))
	}
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func generateReferenceID() string {
	b := make([]byte, 8)
	for i := range b {
		//nolint:gosec // not a security issue.
		b[i] = letters[rand.IntN(len(letters))]
	}

	return string(b)
}
