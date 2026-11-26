// 职责: 使用隔离目录与数据库验证批量导入和价格所有权。
// 关联文件:
//
//	internal/server/biz/model_catalog_preview.go
//	internal/server/biz/model_catalog_apply.go
//
// 注意事项:
//
//	上游夹具可以在预览和应用之间变化，以验证仅跳过受影响的模型。

package biz

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
)

const testCatalog = `{"providers":{
 "moonshot":{"models":[{"id":"kimi-k3","canonical_model_id":"moonshotai/kimi-k3","cost":{"input":2.9879}}]},
 "moonshot-ai":{"models":[{"id":"kimi-k3","name":"Kimi K3","canonical_model_id":"moonshotai/kimi-k3","cost":{"input":3,"output":15}},{"id":"new-model","name":"Another model"}]},
 "auriko":{"models":[{"id":"qwen-3.6-plus","canonical_model_id":"alibaba/qwen-3.6-plus","cost":{"input":0.5,"output":3,"cache_read":0.1,"cache_write":1.25,"tiers":[{"tier":{"type":"context","size":256000},"cache_write":2.5}],"context_over_200k":{"cache_write":3.5}}}]},
 "relay":{"models":[{"id":"a/shared","canonical_model_id":"dev/shared","version":"1"},{"id":"b/shared","canonical_model_id":"dev/shared","version":"1"},{"id":"a/variant","canonical_model_id":"dev/variant","version":"1"},{"id":"b/variant","canonical_model_id":"dev/variant","version":"2"}]}
}}`

type importFixture struct {
	ctx      context.Context
	svc      *ModelService
	db       *ent.Client
	body     atomic.Value
	requests atomic.Int64
}

func catalogFixture(t *testing.T) *importFixture {
	t.Helper()
	ctx, sys, catalog := newCatalogTestService(t)
	f := &importFixture{ctx: ctx, db: ent.FromContext(ctx)}
	f.svc = NewModelService(ModelServiceParams{Ent: f.db, Catalog: catalog, SystemService: sys})
	f.body.Store(testCatalog)
	useCatalogUpstream(t, ctx, sys, catalog, func(w http.ResponseWriter, _ *http.Request) {
		f.requests.Add(1)
		_, _ = io.WriteString(w, f.body.Load().(string))
	})
	return f
}

func (f *importFixture) changeCatalog(body string) { f.body.Store(body); f.svc.catalog.Invalidate() }

func (f *importFixture) preview(t *testing.T, scope CatalogImportScope) *CatalogImportPreview {
	t.Helper()
	preview, err := f.svc.CatalogImportPreview(f.ctx, scope)
	require.NoError(t, err)
	return preview
}

func (f *importFixture) channel(t *testing.T, status channel.Status, ids ...string) {
	t.Helper()
	require.NoError(t, f.db.Channel.Create().SetType(channel.TypeOpenai).SetName(string(status)).SetStatus(status).SetCredentials(objects.ChannelCredentials{}).SetSupportedModels(ids).SetDefaultTestModel(ids[0]).Exec(f.ctx))
}

func (f *importFixture) apply(t *testing.T, scope CatalogImportScope, preview *CatalogImportPreview, selections ...CatalogImportSelection) []string {
	t.Helper()
	results, err := f.svc.ApplyCatalogImport(f.ctx, scope, preview.Revision, selections)
	require.NoError(t, err)
	return lo.Map(results, func(result CatalogImportOutcome, _ int) string { return string(result.Status) })
}

func memberIDs(sel CatalogImportSelection) []string {
	return lo.Map(sel.Members, func(m CatalogImportMember, _ int) string { return m.ID })
}

func selection(t *testing.T, preview *CatalogImportPreview, id string) CatalogImportSelection {
	t.Helper()
	row, found := lo.Find(preview.Rows, func(row *CatalogImportRow) bool { return row.ModelID == id })
	require.True(t, found, id)
	sel := CatalogImportSelection{ModelID: id, Name: row.Name, Type: "chat", Version: row.Version, Members: row.Members}
	for _, route := range row.Routes {
		request := ""
		if len(route.Requests) == 1 {
			request = route.Requests[0]
		}
		sel.Routes = append(sel.Routes, CatalogRouteChoice{route.ChannelID, route.Actual, request})
	}
	o := row.Preferred
	if o == nil && len(row.Offers) > 1 {
		// Simulate an administrator picking a source on an ambiguous new row.
		o = row.Offers[0]
	}
	if o != nil {
		if row.Version == "" {
			sel.Developer = o.Developer
		}
		sel.Identity = o.Identity
		sel.Provider, sel.SourceModelID = o.Provider, o.ModelID
		if o.Price != nil {
			sel.Price = &CatalogImportPriceInput{
				Action: CatalogPriceFollow, Provider: o.Provider, ModelID: o.ModelID,
				Identity: o.Identity, ExpectedDigest: o.PriceDigest,
			}
		}
	}
	return sel
}

func TestCatalogImportScopesAndConflicts(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "kimi-k3", "a/shared", "b/shared", "a/variant")
	f.channel(t, channel.StatusDisabled, "kimi-k3", "b/variant", "unknown")
	f.channel(t, channel.StatusArchived, "archived-only")
	scope := CatalogImportScope{Channels: true}
	exact := f.preview(t, scope)
	require.Len(t, exact.Rows, 6)
	scope.IgnorePrefix = true
	preview := f.preview(t, scope)
	require.Len(t, preview.Rows, 5)
	combined := f.preview(t, CatalogImportScope{Channels: true, Providers: []string{"relay"}, IgnorePrefix: true})
	require.Len(t, combined.Rows, 5)
	require.Equal(t, []string{"a/shared", "b/shared"}, memberIDs(selection(t, combined, "shared")))
	kimi := selection(t, preview, "kimi-k3")
	require.Equal(t, "moonshot-ai", kimi.Provider)
	shared := selection(t, preview, "shared")
	require.Equal(t, []string{"a/shared", "b/shared"}, memberIDs(shared))
	shared.Developer = "dev"
	a, b := selection(t, preview, "a/variant"), selection(t, preview, "b/variant")
	a.ModelID = "variant"
	a.ManualMerge = true
	a.Members = append(a.Members, b.Members...)
	a.Routes = append(a.Routes, b.Routes...)
	unknown := selection(t, preview, "unknown")
	unknown.Developer = "custom"
	unknown.Name = kimi.Name
	require.Equal(t, []string{"applied", "conflict", "applied", "applied"}, f.apply(t, scope, preview, kimi, unknown, shared, a))
	require.Equal(t, []string{"conflict"}, f.apply(t, scope, preview, kimi))
	m := f.db.Model.Query().Where(model.ModelIDEQ("shared")).OnlyX(f.ctx)
	require.Equal(t, model.StatusDisabled, m.Status)
	require.Len(t, m.Settings.Associations, 2)
	require.Equal(t, "b/shared", m.Settings.Associations[1].ChannelModel.ModelID)
	after := f.preview(t, scope)
	require.Len(t, after.Rows, 4)
	f.changeCatalog(strings.Replace(testCatalog, `"input":3`, `"input":4`, 1))
	kimi = selection(t, after, "kimi-k3")
	unknown = selection(t, after, "unknown")
	unknown.Developer = "custom"
	require.Equal(t, []string{"catalogChanged", "applied"}, f.apply(t, scope, after, kimi, unknown))
	fresh := f.preview(t, scope)
	m = f.db.Model.Query().Where(model.ModelIDEQ("kimi-k3")).OnlyX(f.ctx)
	f.db.Model.UpdateOneID(m.ID).SetRemark("edited since preview").ExecX(f.ctx)
	require.Equal(t, []string{"conflict"}, f.apply(t, scope, fresh, selection(t, fresh, "kimi-k3")))
}

func TestCatalogImportVolumeTiers(t *testing.T) {
	for _, tc := range []struct {
		name                string
		above               int64
		tierRate, nextRate   string
		removeExplicitTiers bool
	}{
		{name: "explicit context tier", above: 256000, tierRate: "2.5", nextRate: "4.5"},
		{name: "legacy context over 200k", above: 200000, tierRate: "3.5", nextRate: "5.5", removeExplicitTiers: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := catalogFixture(t)
			body := testCatalog
			if tc.removeExplicitTiers {
				body = strings.Replace(body, `"tiers":[{"tier":{"type":"context","size":256000},"cache_write":2.5}],`, "", 1)
			}
			f.changeCatalog(body)
			scope := CatalogImportScope{Providers: []string{"auriko"}}
			preview := f.preview(t, scope)
			require.Len(t, preview.Rows, 1)
			row := preview.Rows[0]
			require.Nil(t, row.Price)
			require.Nil(t, row.Binding)
			offer := row.Preferred
			require.NotNil(t, offer)
			require.NotNil(t, offer.Price)
			require.NotEmpty(t, offer.PriceDigest)
			require.Len(t, offer.Price.Items, 4)
			require.Len(t, offer.Price.VolumeTiers, 1)
			tier := offer.Price.VolumeTiers[0]
			require.Equal(t, tc.above, tier.Above)
			require.Len(t, tier.Items, 4)
			require.Equal(t, "1.25", offer.Price.Items[3].Pricing.UsagePerUnit.String())
			require.Equal(t, tc.tierRate, tier.Items[3].Pricing.UsagePerUnit.String())
			require.True(t, tier.Items[0].Equals(&offer.Price.Items[0]))

			sel := selection(t, preview, "qwen-3.6-plus")
			require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, sel))
			card := f.db.Model.Query().OnlyX(f.ctx).ModelCard
			require.True(t, card.Price.Equals(*offer.Price))
			require.Equal(t, &objects.CatalogPrice{
				Provider: "auriko", ModelID: sel.ModelID, Canonical: "alibaba/qwen-3.6-plus", Follow: true,
			}, card.CatalogPrice)
			require.Equal(t, 1.25, card.Cost.CacheWrite)

			f.changeCatalog(strings.Replace(body, `"cache_write":`+tc.tierRate, `"cache_write":`+tc.nextRate, 1))
			updated := f.preview(t, scope).Rows[0]
			require.True(t, updated.Changed)
			require.True(t, updated.Price.Equals(*card.Price))
			require.Equal(t, card.CatalogPrice, updated.Binding)
		})
	}
}

func TestPriceFromCatalogAnthropicVariants(t *testing.T) {
	entry := map[string]any{"cost": map[string]any{
		"input": 0.1, "output": 0.7, "cache_write": 0.125,
		"tiers": []any{
			map[string]any{"tier": map[string]any{"type": "context", "size": 256000.0}, "input": 0.3},
			map[string]any{"tier": map[string]any{"type": "context", "size": 100000.0}, "input": 0.2},
			map[string]any{"tier": map[string]any{"type": "context", "size": 0.5}, "input": 0.9},
			map[string]any{"tier": map[string]any{"type": "batch", "size": 100.0}, "input": 0.9},
		},
	}}
	price := priceFromCatalog(entry, "anthropic")
	require.NotNil(t, price)
	require.Len(t, price.VolumeTiers, 2)
	require.Equal(t, int64(100000), price.VolumeTiers[0].Above)
	require.Equal(t, int64(256000), price.VolumeTiers[1].Above)
	for _, tc := range []struct {
		items   []objects.ModelPriceItem
		oneHour string
	}{
		{price.Items, "0.2"},
		{price.VolumeTiers[0].Items, "0.4"},
		{price.VolumeTiers[1].Items, "0.6"},
	} {
		require.Len(t, tc.items, 3)
		write := tc.items[2]
		require.Equal(t, objects.PriceItemCodeWriteCachedTokens, write.ItemCode)
		require.Equal(t, "0.125", write.Pricing.UsagePerUnit.String())
		require.Len(t, write.PromptWriteCacheVariants, 1)
		variant := write.PromptWriteCacheVariants[0]
		require.Equal(t, objects.PromptWriteCacheVariantCode1Hour, variant.VariantCode)
		require.Equal(t, tc.oneHour, variant.Pricing.UsagePerUnit.String())
	}
	require.Empty(t, priceFromCatalog(entry, "relay").Items[2].PromptWriteCacheVariants)
}

func TestPriceFromCatalogRejectsInvalidQuotes(t *testing.T) {
	for _, rate := range []float64{-1, math.NaN(), math.Inf(1)} {
		entry := map[string]any{"cost": map[string]any{"input": rate}}
		require.Nil(t, priceFromCatalog(entry, "relay"))
	}
	entry := map[string]any{"cost": map[string]any{
		"input": 1.0,
		"tiers": []any{map[string]any{
			"tier":   map[string]any{"type": "context", "size": 256000.0},
			"output": 2.0,
		}},
	}}
	require.Nil(t, priceFromCatalog(entry, "relay"))
}

func TestCatalogImportUnpricedOffer(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	preview := f.preview(t, scope)
	sel := selection(t, preview, "new-model")
	sel.Developer = "moonshot"
	require.Nil(t, sel.Price)
	sel.Price = &CatalogImportPriceInput{Action: CatalogPriceFix}
	require.Equal(t, []string{"invalid"}, f.apply(t, scope, preview, sel))
	sel.Price = &CatalogImportPriceInput{Action: CatalogPriceFollow, Provider: sel.Provider, ModelID: sel.SourceModelID, Identity: sel.Identity}
	require.Equal(t, []string{"invalid"}, f.apply(t, scope, preview, sel))
	sel.Price = nil
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, sel))
	card := f.db.Model.Query().OnlyX(f.ctx).ModelCard
	require.Nil(t, card.Price)
	require.Nil(t, card.CatalogPrice)
}

func TestCatalogImportFallback(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	preview := f.preview(t, scope)
	sel := selection(t, preview, "kimi-k3")
	sel.Price.Action = CatalogPriceFixSource
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, sel))
	m := f.db.Model.Query().OnlyX(f.ctx)
	require.Equal(t, &objects.CatalogPrice{Provider: "moonshot-ai", ModelID: m.ModelID, Canonical: "moonshotai/kimi-k3"}, m.ModelCard.CatalogPrice)
	require.Equal(t, 3.0, m.ModelCard.Cost.Input)
	require.Equal(t, 15.0, m.ModelCard.Cost.Output)
	f.changeCatalog("offline")
	fallback := f.preview(t, CatalogImportScope{Providers: []string{"moonshot"}})
	require.Equal(t, catalogSourceFallback, fallback.Source)
	selected := selection(t, fallback, m.ModelID)
	require.Equal(t, []string{"conflict"}, f.apply(t, CatalogImportScope{Providers: []string{"moonshot"}}, fallback, selected))
	other, ok := lo.Find(fallback.Rows, func(r *CatalogImportRow) bool { return r.Version == "" && r.Preferred != nil })
	require.True(t, ok)
	selected = selection(t, fallback, other.ModelID)
	selected.Developer = "moonshot"
	require.Equal(t, []string{"applied"}, f.apply(t, CatalogImportScope{Providers: []string{"moonshot"}}, fallback, selected))
}

func TestCatalogModelTypes(t *testing.T) {
	for raw, want := range map[string]string{"image": "image_generation", "video": "video_generation", "reranking": "rerank", "image-generation": "image_generation", "embedding": "embedding", "audio": "audio"} {
		require.Equal(t, want, catalogModelType(raw))
	}
}

func TestCatalogImportPreservesStoredPriceAndAllowsWriteOnlyEditor(t *testing.T) {
	f := catalogFixture(t)
	oldPrice := priceFromCatalog(map[string]any{"cost": map[string]any{"input": 0.0, "output": 9.0}}, "custom")
	aliases := []string{"kimi-alias"}
	m := f.db.Model.Create().SetModelID("kimi-k3").SetName("Existing").SetGroup("custom").SetDeveloper("custom").SetIcon("custom").SetStatus(model.StatusEnabled).
		SetSettings(&objects.ModelSettings{}).SetModelCard(&objects.ModelCard{Price: oldPrice, CatalogAliases: aliases}).SaveX(f.ctx)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	preview := f.preview(t, scope)
	row, ok := lo.Find(preview.Rows, func(r *CatalogImportRow) bool { return r.ModelID == m.ModelID })
	require.True(t, ok)
	require.False(t, row.Changed)
	require.Nil(t, row.Binding)
	require.True(t, row.Price.Equals(*oldPrice))
	sel := selection(t, preview, m.ModelID)
	noPrice := sel
	noPrice.Price = nil
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, noPrice))
	unchanged := f.db.Model.GetX(f.ctx, m.ID).ModelCard
	require.True(t, unchanged.Price.Equals(*oldPrice))
	require.Nil(t, unchanged.CatalogPrice)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, sel))
	m = f.db.Model.GetX(f.ctx, m.ID)
	require.Equal(t, "Existing", m.Name)
	require.Equal(t, "custom", m.Developer)
	require.Equal(t, model.StatusEnabled, m.Status)
	require.True(t, m.ModelCard.Price.Equals(*row.Preferred.Price))
	require.Equal(t, 3.0, m.ModelCard.Cost.Input)
	require.Equal(t, 15.0, m.ModelCard.Cost.Output)
	binding := *m.ModelCard.CatalogPrice
	require.True(t, binding.Follow)
	// A write-only role may submit a whole model card when editing the name.
	ctx := contexts.WithUser(ent.NewContext(context.Background(), f.db), &ent.User{ID: 1, Scopes: []string{"write_channels"}})
	_, readErr := f.db.Model.Query().Where(model.IDEQ(m.ID)).OnlyID(ctx)
	require.Error(t, readErr)
	card := *m.ModelCard
	card.CatalogPrice, card.CatalogAliases = nil, nil
	m, err := f.svc.UpdateModel(ctx, m.ID, &ent.UpdateModelInput{ModelCard: &card, Name: lo.ToPtr("Renamed")})
	require.NoError(t, err)
	require.Equal(t, &binding, m.ModelCard.CatalogPrice)
	require.Equal(t, aliases, m.ModelCard.CatalogAliases)

	card.Price = &objects.ModelPrice{Items: []objects.ModelPriceItem{{
		ItemCode: objects.PriceItemCodeUsage,
		Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: lo.ToPtr(decimal.NewFromInt(7))},
	}}}
	card.CatalogPrice, card.CatalogAliases = nil, nil
	m, err = f.svc.UpdateModel(ctx, m.ID, &ent.UpdateModelInput{ModelCard: &card})
	require.NoError(t, err)
	binding.Follow = false
	require.Equal(t, &binding, m.ModelCard.CatalogPrice)
	require.Equal(t, aliases, m.ModelCard.CatalogAliases)
	require.True(t, m.ModelCard.Price.Equals(*card.Price))
	require.Equal(t, 7.0, m.ModelCard.Cost.Input)
}

func TestCatalogImportNestedChannelIDs(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "a/vendor/x", "b/vendor/x", "a/vendor/only")
	scope := CatalogImportScope{Channels: true, IgnorePrefix: true}
	preview := f.preview(t, scope)
	require.Len(t, preview.Rows, 3)
	a, b := selection(t, preview, "a/vendor/x"), selection(t, preview, "b/vendor/x")
	a.ModelID, a.Developer = "x", "dev"
	a.ManualMerge = true
	a.Members = append(a.Members, b.Members...)
	a.Routes = append(a.Routes, b.Routes...)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, a))
	stored := f.db.Model.Query().Where(model.ModelIDEQ("x")).OnlyX(f.ctx)
	require.ElementsMatch(t, []string{"a/vendor/x", "b/vendor/x"}, stored.ModelCard.CatalogAliases)
	require.Len(t, stored.Settings.Associations, 2)
	for i, id := range memberIDs(a) {
		require.Equal(t, id, stored.Settings.Associations[i].ChannelModel.ModelID)
	}
	selection(t, preview, "a/vendor/only")
}

func TestCatalogImportRecognizesStoredAliasesAcrossScopes(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "a/shared", "b/shared")
	scope := CatalogImportScope{Channels: true, IgnorePrefix: true}
	preview := f.preview(t, scope)
	group := selection(t, preview, "shared")
	group.Developer = "dev"
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, group))
	for _, scope := range []CatalogImportScope{{Providers: []string{"relay"}}, {Channels: true}} {
		for _, row := range f.preview(t, scope).Rows {
			require.NotContains(t, []string{"a/shared", "b/shared"}, row.ModelID)
		}
	}
}

func TestCatalogImportPreservesUnrelatedExistingMergeTarget(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "a/shared", "b/shared")
	f.db.Model.Create().SetModelID("shared").SetName("Different model").SetGroup("other").SetDeveloper("other").SetIcon("other").SetSettings(&objects.ModelSettings{}).SetModelCard(&objects.ModelCard{}).SaveX(f.ctx)
	p := f.preview(t, CatalogImportScope{Channels: true, IgnorePrefix: true})
	require.Len(t, p.Rows, 2)
	for _, row := range p.Rows {
		require.True(t, row.MergeTargetOccupied)
		require.Equal(t, "shared", row.MergeTarget)
	}
	for _, id := range []string{"a/shared", "b/shared"} {
		require.Empty(t, selection(t, p, id).Version)
	}
}

func TestCatalogAutomaticallyMergesVerifiedNestedIDs(t *testing.T) {
	f := catalogFixture(t)
	f.changeCatalog(strings.NewReplacer("a/shared", "a/vendor/shared", "b/shared", "b/vendor/shared").Replace(testCatalog))
	f.channel(t, channel.StatusEnabled, "a/vendor/shared", "b/vendor/shared")
	scope := CatalogImportScope{Channels: true, IgnorePrefix: true}
	preview := f.preview(t, scope)
	require.Len(t, preview.Rows, 1)
	group := selection(t, preview, "shared")
	require.Equal(t, []string{"a/vendor/shared", "b/vendor/shared"}, memberIDs(group))
	group.Developer = "dev"
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, group))
}

func TestCatalogImportLowercasedRoute(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "Vendor/X")
	ch := f.db.Channel.Query().OnlyX(f.ctx)
	ch = ch.Update().SetSettings(&objects.ChannelSettings{LowercaseModelID: true}).SaveX(f.ctx)
	scope := CatalogImportScope{Channels: true}
	p := f.preview(t, scope)
	sel := selection(t, p, "Vendor/X")
	sel.Developer = "dev"
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, sel))
	m := f.db.Model.Query().OnlyX(f.ctx)
	route := &Channel{Channel: ch}
	require.Contains(t, route.GetModelEntries(), "vendor/x")
	require.Len(t, MatchAssociations(m.Settings.Associations, []*Channel{route}), 1)
}

func TestCatalogImportUnverifiedOffer(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "private/vendor/x")
	f.changeCatalog(`{"providers":{"openai":{"models":[{"id":"other/x","canonical_model_id":"other/x","name":"Unrelated X","cost":{"input":45}}]}}}`)
	p := f.preview(t, CatalogImportScope{Channels: true, IgnorePrefix: true})
	require.Len(t, p.Rows, 1)
	require.Len(t, p.Rows[0].Offers, 1)
	require.Nil(t, p.Rows[0].Preferred)
}

func TestCatalogImportConcurrentEdit(t *testing.T) {
	f := catalogFixture(t)
	start, update := model.DefaultUpdatedAt, model.UpdateDefaultUpdatedAt
	t.Cleanup(func() { model.DefaultUpdatedAt, model.UpdateDefaultUpdatedAt = start, update })
	tick := time.Now().UTC().Truncate(time.Second)
	model.DefaultUpdatedAt = func() time.Time { return tick }
	model.UpdateDefaultUpdatedAt = func() time.Time { return tick }
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	p := f.preview(t, scope)
	sel := selection(t, p, "kimi-k3")
	sel.Price = nil
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, sel))
	before := f.preview(t, scope)
	stale := selection(t, before, "kimi-k3")
	m := f.db.Model.Query().OnlyX(f.ctx)
	card := *m.ModelCard
	card.Price = &objects.ModelPrice{Items: []objects.ModelPriceItem{{
		ItemCode: objects.PriceItemCodeUsage,
		Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: lo.ToPtr(decimal.NewFromInt(99))},
	}}}
	_, err := f.svc.UpdateModelWithVersion(f.ctx, m.ID, &ent.UpdateModelInput{ModelCard: &card}, lo.ToPtr(lo.Must(ModelEditVersion(m))))
	require.NoError(t, err)
	require.True(t, f.db.Model.GetX(f.ctx, m.ID).ModelCard.Price.Equals(*card.Price))
	require.Equal(t, []string{"conflict"}, f.apply(t, scope, before, stale))
	require.True(t, f.db.Model.GetX(f.ctx, m.ID).ModelCard.Price.Equals(*card.Price))
	_, err = f.svc.UpdateModelWithVersion(f.ctx, m.ID, &ent.UpdateModelInput{Name: lo.ToPtr("stale")}, &stale.Version)
	require.ErrorContains(t, err, "model changed")
	f.db.Model.UpdateOneID(m.ID).SetModelCard(m.ModelCard).ExecX(f.ctx)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, before, stale))
}

func TestCatalogSelectedQuoteAndLatestMetadata(t *testing.T) {
	for _, tc := range []struct{ name, old, next, status string }{
		{"selected input", `"input":3,"output":15`, `"input":4,"output":15`, "catalogChanged"},
		{"selected output", `"input":3,"output":15`, `"input":3,"output":16`, "catalogChanged"},
		{"unselected provider", `"input":2.9879`, `"input":2.9878`, "applied"},
		{"selected identity", `"name":"Kimi K3"`, `"name":"Kimi K3","version":"2"`, "catalogChanged"},
		{"capability and display name", `"name":"Kimi K3"`, `"name":"New display name","limit":{"context":256000}`, "applied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := catalogFixture(t)
			f.channel(t, channel.StatusEnabled, "kimi-k3")
			scope := CatalogImportScope{Channels: true}
			p := f.preview(t, scope)
			sel := selection(t, p, "kimi-k3")
			f.body.Store(strings.Replace(testCatalog, tc.old, tc.next, 1))
			require.Equal(t, []string{tc.status}, f.apply(t, scope, p, sel))
			if tc.name == "capability and display name" {
				m := f.db.Model.Query().OnlyX(f.ctx)
				require.Equal(t, 256000, m.ModelCard.Limit.Context)
				require.Equal(t, "Kimi K3", m.Name)
			}
		})
	}
}

func TestCatalogManualSubsetAndAmbiguousRoutes(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Channels: true, IgnorePrefix: true}
	f.channel(t, channel.StatusEnabled, "a/shared", "b/shared", "c/shared")
	ch := f.db.Channel.Query().OnlyX(f.ctx)
	settings := &objects.ChannelSettings{HideOriginalModels: true, ModelMappings: []objects.ModelMapping{{From: "public1", To: "a/shared"}, {From: "public2", To: "a/shared"}}}
	ch.Update().SetSettings(settings).ExecX(f.ctx)
	p := f.preview(t, scope)
	require.Len(t, p.Rows, 3)
	a, b := selection(t, p, "a/shared"), selection(t, p, "b/shared")
	a.ModelID, a.Developer = "shared", "dev"
	a.ManualMerge = true
	a.Members = append(a.Members, b.Members...)
	a.Routes = append(a.Routes, b.Routes...)
	require.Equal(t, []string{"conflict"}, f.apply(t, scope, p, a))
	a.Routes[0].Request = "public2"
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, a))
	m := f.db.Model.Query().OnlyX(f.ctx)
	require.Len(t, m.Settings.Associations, 1)
	require.Equal(t, "public2", m.Settings.Associations[0].ChannelModel.ModelID)
	require.Equal(t, []string{"a/shared", "b/shared"}, m.ModelCard.CatalogAliases)
	require.Empty(t, selection(t, f.preview(t, scope), "c/shared").Version)
}

func TestCatalogFixCurrentAfterProviderRemovesModel(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	p := f.preview(t, scope)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, selection(t, p, "kimi-k3")))
	original := f.db.Model.Query().OnlyX(f.ctx).ModelCard
	p = f.preview(t, scope)
	edit := selection(t, p, "kimi-k3")
	f.body.Store(`{"providers":{"moonshot-ai":{"models":[]}}}`)
	require.Equal(t, []string{"catalogChanged"}, f.apply(t, scope, p, edit))
	edit.Price = &CatalogImportPriceInput{Action: CatalogPriceFix}
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, edit))
	m := f.db.Model.Query().OnlyX(f.ctx)
	fixed := *original.CatalogPrice
	fixed.Follow = false
	require.Equal(t, &fixed, m.ModelCard.CatalogPrice)
	require.True(t, m.ModelCard.Price.Equals(*original.Price))
}

func TestCatalogPreviewAfterProviderRemovesModel(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	p := f.preview(t, scope)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, selection(t, p, "kimi-k3")))
	m := f.db.Model.Query().OnlyX(f.ctx)
	m.ModelCard.CatalogAliases = []string{"kimi-k3", "other-alias"}
	m.Update().SetModelID("local-kimi").SetModelCard(m.ModelCard).ExecX(f.ctx)

	f.changeCatalog(`{"providers":{"moonshot-ai":{"models":[{"id":"other-alias"}]}}}`)
	p = f.preview(t, scope)
	require.Len(t, p.Rows, 1)
	require.Equal(t, []string{"other-alias"}, memberIDs(selection(t, p, "local-kimi")))
	f.changeCatalog(`{"providers":{"other":{"models":[]}}}`)
	require.Empty(t, f.preview(t, scope).Rows)
	f.changeCatalog(`{"providers":{"moonshot-ai":{"models":[]},"other":{"models":[]}}}`)
	fallback, err := f.svc.catalogPreviewFromSnapshot(f.ctx, scope, CatalogSnapshot{Source: catalogSourceFallback, Data: []byte(f.body.Load().(string))})
	require.NoError(t, err)
	require.Empty(t, fallback.Rows)
	require.Empty(t, f.preview(t, CatalogImportScope{Providers: []string{"other"}}).Rows)
	require.Empty(t, f.preview(t, CatalogImportScope{Channels: true}).Rows)
	m.Update().SetStatus(model.StatusArchived).ExecX(f.ctx)
	require.Empty(t, f.preview(t, scope).Rows)
	m.Update().SetStatus(model.StatusEnabled).ExecX(f.ctx)

	p = f.preview(t, scope)
	require.Len(t, p.Rows, 1)
	require.Empty(t, p.Rows[0].Offers)
	require.False(t, p.Rows[0].Changed)
	require.Equal(t, m.ModelCard.CatalogPrice, p.Rows[0].Binding)
	require.True(t, p.Rows[0].Price.Equals(*m.ModelCard.Price))
	edit := selection(t, p, "local-kimi")
	require.NotEmpty(t, edit.Version)
	edit.Price = &CatalogImportPriceInput{Action: CatalogPriceFix}
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, edit))
	card := f.db.Model.Query().OnlyX(f.ctx).ModelCard
	require.False(t, card.CatalogPrice.Follow)
	require.True(t, card.Price.Equals(*m.ModelCard.Price))
	require.Empty(t, f.preview(t, scope).Rows)
}

func TestCatalogDoesNotConvertAutomaticMergeToManualAtApply(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "a/shared", "b/shared")
	scope := CatalogImportScope{Channels: true, IgnorePrefix: true}
	p := f.preview(t, scope)
	sel := selection(t, p, "shared")
	sel.Developer = "dev"
	f.channel(t, channel.StatusDisabled, "c/shared")
	require.Equal(t, []string{"conflict"}, f.apply(t, scope, p, sel))
}

func TestCatalogImportRechecksRoutesPerRow(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "first", "second", "third", "replacement")
	ch := f.db.Channel.Query().OnlyX(f.ctx)
	settings := &objects.ChannelSettings{HideMappedModels: true, ModelMappings: []objects.ModelMapping{{From: "public", To: "second"}}}
	ch.Update().SetSettings(settings).ExecX(f.ctx)
	scope := CatalogImportScope{Channels: true}
	p := f.preview(t, scope)
	var selections []CatalogImportSelection
	for _, id := range []string{"first", "second", "third"} {
		sel := selection(t, p, id)
		sel.Developer = "dev"
		selections = append(selections, sel)
	}
	require.Equal(t, "public", selections[1].Routes[0].Request)
	f.db.Model.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, mutation ent.Mutation) (ent.Value, error) {
			value, err := next.Mutate(ctx, mutation)
			if id, _ := mutation.(*ent.ModelMutation).ModelID(); err == nil && id == "first" {
				runAfterCommit(ctx, func(context.Context) {
					settings.ModelMappings[0].To = "replacement"
					ch.Update().SetSettings(settings).SetRemark("edited during import").ExecX(f.ctx)
				})
			}
			return value, err
		})
	})
	require.Equal(t, []string{"applied", "conflict", "applied"}, f.apply(t, scope, p, selections...))
	require.False(t, f.db.Model.Query().Where(model.ModelIDEQ("second")).ExistX(f.ctx))
}

func TestCatalogPriceIdentityAndWithdrawal(t *testing.T) {
	f := catalogFixture(t)
	scope := CatalogImportScope{Providers: []string{"moonshot-ai"}}
	p := f.preview(t, scope)
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, selection(t, p, "kimi-k3")))
	for _, canonical := range []string{"other/new-model", "", "moonshotai/kimi-k3"} {
		body := strings.ReplaceAll(testCatalog, "moonshotai/kimi-k3", canonical)
		f.changeCatalog(body)
		p = f.preview(t, scope)
		edit := selection(t, p, "kimi-k3")
		require.Equal(t, []string{"catalogChanged"}, f.apply(t, scope, p, edit))
		edit.Price.Action = CatalogPriceRebind
		require.Equal(t, []string{"applied"}, f.apply(t, scope, p, edit))
		require.Equal(t, &objects.CatalogPrice{
			Provider: "moonshot-ai", ModelID: "kimi-k3", Canonical: canonical, Follow: true,
		}, f.db.Model.Query().OnlyX(f.ctx).ModelCard.CatalogPrice)
	}
	m := f.db.Model.Query().OnlyX(f.ctx)
	m.ModelCard.CatalogPrice.Canonical = ""
	m.Update().SetModelCard(m.ModelCard).ExecX(f.ctx)
	f.changeCatalog(strings.Replace(testCatalog, `"input":3,"output":15`, `"input":30,"output":16`, 1))
	p = f.preview(t, scope)
	edit := selection(t, p, "kimi-k3")
	require.False(t, p.Rows[0].Changed)
	require.Equal(t, []string{"catalogChanged"}, f.apply(t, scope, p, edit))
	edit.Price.Action = CatalogPriceRebind
	require.Equal(t, []string{"applied"}, f.apply(t, scope, p, edit))
	updated := f.db.Model.Query().OnlyX(f.ctx)
	require.Equal(t, 30.0, updated.ModelCard.Cost.Input)
	require.Equal(t, 16.0, updated.ModelCard.Cost.Output)
	require.True(t, updated.ModelCard.Price.Equals(*p.Rows[0].Preferred.Price))

	p = f.preview(t, scope)
	edit = selection(t, p, "kimi-k3")
	edit.Price.ExpectedDigest = "different digest"
	require.Equal(t, []string{"catalogChanged"}, f.apply(t, scope, p, edit))
	f.changeCatalog(strings.Replace(testCatalog, `"input":3,"output":15`, `"image":16`, 1))
	row, ok := lo.Find(f.preview(t, scope).Rows, func(r *CatalogImportRow) bool { return r.ModelID == "kimi-k3" })
	require.True(t, ok)
	require.False(t, row.Changed)
	require.Nil(t, row.Preferred.Price)
	require.Empty(t, row.Preferred.PriceDigest)
	require.True(t, row.Price.Equals(*updated.ModelCard.Price))
}

func TestCatalogImportOverlappingAliases(t *testing.T) {
	for _, mergeFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("mergeFirst=%t", mergeFirst), func(t *testing.T) {
			f := catalogFixture(t)
			f.channel(t, channel.StatusEnabled, "first", "a/shared", "b/shared")
			scope := CatalogImportScope{Channels: true, IgnorePrefix: !mergeFirst}
			other := CatalogImportScope{Channels: true, IgnorePrefix: mergeFirst}
			p, q := f.preview(t, scope), f.preview(t, other)
			first := selection(t, p, "first")
			second := selection(t, p, lo.Ternary(mergeFirst, "a/shared", "shared"))
			interleaved := selection(t, q, lo.Ternary(mergeFirst, "shared", "a/shared"))
			first.Developer, second.Developer, interleaved.Developer = "dev", "dev", "dev"
			second.Name, interleaved.Name = "Second", "Interleaved"
			f.db.Model.Use(func(next ent.Mutator) ent.Mutator {
				return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
					v, err := next.Mutate(ctx, m)
					if id, _ := m.(*ent.ModelMutation).ModelID(); err == nil && id == "first" {
						runAfterCommit(ctx, func(context.Context) { require.Equal(t, []string{"applied"}, f.apply(t, other, q, interleaved)) })
					}
					return v, err
				})
			})
			require.Equal(t, []string{"applied", "conflict"}, f.apply(t, scope, p, first, second))
			require.Equal(t, 2, f.db.Model.Query().CountX(f.ctx))
		})
	}
}

func TestCatalogImportUsesOnlyReviewedRoutes(t *testing.T) {
	f := catalogFixture(t)
	f.channel(t, channel.StatusEnabled, "kimi-k3")
	original := f.db.Channel.Query().OnlyX(f.ctx)
	scope := CatalogImportScope{Channels: true}
	preview := f.preview(t, scope)
	sel := selection(t, preview, "kimi-k3")
	f.channel(t, channel.StatusDisabled, "kimi-k3")
	require.Equal(t, []string{"applied"}, f.apply(t, scope, preview, sel))
	associations := f.db.Model.Query().OnlyX(f.ctx).Settings.Associations
	require.Len(t, associations, 1)
	require.Equal(t, original.ID, associations[0].ChannelModel.ChannelID)
}

func TestCatalogImportPartialFailureAndDetails(t *testing.T) {
	f := catalogFixture(t)
	f.changeCatalog(`{"providers":{"openai":{"models":[{"id":"one","name":"Vector","type":"embedding","limit":{"context":256000},"reasoning_options":[{"type":"effort","values":["high",null,"unknown","low","high"]}],"cost":{"input":3}},{"id":"two"},{"id":"three"}]}}}`)
	scope := CatalogImportScope{Providers: []string{"openai"}}
	p := f.preview(t, scope)
	one, two, three := selection(t, p, "one"), selection(t, p, "two"), selection(t, p, "three")
	one.Type, one.Price = "embedding", nil
	f.db.Model.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if id, _ := m.(*ent.ModelMutation).ModelID(); id == "two" {
				return nil, errors.New("test row failure")
			}
			return next.Mutate(ctx, m)
		})
	})
	require.Equal(t, []string{"applied", "failed", "applied"}, f.apply(t, scope, p, one, two, three))
	m := f.db.Model.Query().Where(model.ModelIDEQ("one")).OnlyX(f.ctx)
	require.Empty(t, m.Settings.Associations)
	require.Equal(t, model.TypeEmbedding, m.Type)
	require.Equal(t, 256000, m.ModelCard.Limit.Context)
	require.Equal(t, []string{"low", "high"}, m.ModelCard.ReasoningEfforts)
	require.Nil(t, m.ModelCard.Price)
	require.Nil(t, m.ModelCard.CatalogPrice)
}

func TestCatalogImportMalformedMetadataFailsOnlyItsRow(t *testing.T) {
	f := catalogFixture(t)
	f.changeCatalog(`{"providers":{"openai":{"models":[{"id":"one"},{"id":"two","limit":{"context":"128000"}},{"id":"three"}]}}}`)
	scope := CatalogImportScope{Providers: []string{"openai"}}
	p := f.preview(t, scope)
	require.Equal(t, []string{"applied", "failed", "applied"}, f.apply(t, scope, p,
		selection(t, p, "one"), selection(t, p, "two"), selection(t, p, "three")))
	ids := f.db.Model.Query().Select(model.FieldModelID).StringsX(f.ctx)
	require.ElementsMatch(t, []string{"one", "three"}, ids)
}

func TestCatalogCanonicalAliasMatchesOnlyUnambiguousVariants(t *testing.T) {
	f := catalogFixture(t)
	f.changeCatalog(string(fallbackCatalogJSON()))
	f.channel(t, channel.StatusEnabled, "kimi-k3", "moonshotai/kimi-k3", "kimi-k3.1-pro", "moonshotai/kimi-k3.1-pro")
	p := f.preview(t, CatalogImportScope{Channels: true, IgnorePrefix: true})
	require.Len(t, p.Rows, 3)
	require.Equal(t, []string{"kimi-k3", "moonshotai/kimi-k3"}, memberIDs(selection(t, p, "kimi-k3")))
	require.Len(t, selection(t, p, "moonshotai/kimi-k3.1-pro").Members, 1)
}
