// 职责: 将目录报价转换为模型价格，供导入预览和确认使用。
// 关联文件:
//
//	internal/server/biz/model_catalog_preview.go
//	internal/server/biz/model_catalog_apply.go
//	internal/objects/price.go
//	frontend/src/features/models/data/pricing.ts
//
// 注意事项:
//
//	使用相同的目录字段转换规则维持批量添加和导入价格的一致性。

package biz

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"slices"

	"github.com/shopspring/decimal"

	"github.com/looplj/axonhub/internal/objects"
)

var catalogPriceItems = [...]struct {
	key  string
	code objects.PriceItemCode
}{
	{"input", objects.PriceItemCodeUsage},
	{"output", objects.PriceItemCodeCompletion},
	{"cache_read", objects.PriceItemCodePromptCachedToken},
	{"cache_write", objects.PriceItemCodeWriteCachedTokens},
}

func priceFromCatalog(entry map[string]any, provider string) *objects.ModelPrice {
	cost, _ := entry["cost"].(map[string]any)
	items := catalogItemsFor(cost, nil, provider)
	if len(items) == 0 {
		return nil
	}
	const maxSafeInteger = 1<<53 - 1
	var tiers []objects.ModelPriceVolumeTier
	rawTiers, _ := cost["tiers"].([]any)
	for _, rawTier := range rawTiers {
		tier, _ := rawTier.(map[string]any)
		threshold, _ := tier["tier"].(map[string]any)
		size, ok := threshold["size"].(float64)
		if !ok || threshold["type"] != "context" || size < 0 || size > maxSafeInteger || math.Trunc(size) != size {
			continue
		}
		tiers = append(tiers, objects.ModelPriceVolumeTier{Above: int64(size), Items: catalogItemsFor(cost, tier, provider)})
	}
	slices.SortFunc(tiers, func(a, b objects.ModelPriceVolumeTier) int { return cmp.Compare(a.Above, b.Above) })
	if len(tiers) == 0 {
		if legacy, ok := cost["context_over_200k"].(map[string]any); ok && legacy != nil {
			tiers = append(tiers, objects.ModelPriceVolumeTier{Above: 200000, Items: catalogItemsFor(cost, legacy, provider)})
		}
	}
	price := &objects.ModelPrice{Items: items, VolumeTiers: tiers}
	if err := price.Validate(); err != nil {
		return nil
	}
	return price
}

func catalogItemsFor(cost, overrides map[string]any, provider string) []objects.ModelPriceItem {
	var items []objects.ModelPriceItem
	var input *decimal.Decimal
	for _, field := range catalogPriceItems {
		raw, overridden := overrides[field.key]
		if !overridden {
			raw = cost[field.key]
		}
		value, ok := raw.(float64)
		if !ok || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
			continue
		}
		rate := decimal.NewFromFloat(value)
		item := objects.ModelPriceItem{
			ItemCode: field.code,
			Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &rate},
		}
		if field.code == objects.PriceItemCodeUsage {
			input = &rate
		}
		// Anthropic's catalog supplies the five-minute write rate. Its one-hour rate is twice input.
		if provider == "anthropic" && field.code == objects.PriceItemCodeWriteCachedTokens && input != nil {
			oneHour := input.Add(*input)
			item.PromptWriteCacheVariants = []objects.PromptWriteCacheVariant{{
				VariantCode: objects.PromptWriteCacheVariantCode1Hour,
				Pricing:     objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &oneHour},
			}}
		}
		if items == nil {
			items = make([]objects.ModelPriceItem, 0, len(catalogPriceItems))
		}
		items = append(items, item)
	}
	return items
}

func catalogPriceDigest(price *objects.ModelPrice) string {
	if price == nil {
		return ""
	}
	data, err := json.Marshal(price)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func equalModelPrices(a, b *objects.ModelPrice) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equals(*b)
}
