package objects

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelPrice_Equals(t *testing.T) {
	d1 := decimal.NewFromFloat(0.01)
	d2 := decimal.NewFromFloat(0.02)
	upTo1000 := int64(1000)

	tests := []struct {
		name     string
		p1       ModelPrice
		p2       ModelPrice
		expected bool
	}{
		{
			name: "Equal simple",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not equal mode",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeUsagePerUnit,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Not equal usage per unit",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d1,
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode:         PricingModeUsagePerUnit,
							UsagePerUnit: &d2,
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Equal tiered",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
									{UpTo: nil, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
									{UpTo: nil, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Not equal tiered price",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d1},
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeUsage,
						Pricing: Pricing{
							Mode: PricingModeTiered,
							UsageTiered: &TieredPricing{
								Tiers: []PriceTier{
									{UpTo: &upTo1000, PricePerUnit: d2},
								},
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Equal with variants",
			p1: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeWriteCachedTokens,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
						PromptWriteCacheVariants: []PromptWriteCacheVariant{
							{
								VariantCode: PromptWriteCacheVariantCode5Min,
								Pricing: Pricing{
									Mode:         PricingModeUsagePerUnit,
									UsagePerUnit: &d1,
								},
							},
						},
					},
				},
			},
			p2: ModelPrice{
				Items: []ModelPriceItem{
					{
						ItemCode: PriceItemCodeWriteCachedTokens,
						Pricing: Pricing{
							Mode: PricingModeFlatFee,
						},
						PromptWriteCacheVariants: []PromptWriteCacheVariant{
							{
								VariantCode: PromptWriteCacheVariantCode5Min,
								Pricing: Pricing{
									Mode:         PricingModeUsagePerUnit,
									UsagePerUnit: &d1,
								},
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.p1.Equals(tt.p2))
			assert.Equal(t, tt.expected, tt.p2.Equals(tt.p1))
		})
	}
}

func TestModelPrice_EqualsVolume(t *testing.T) {
	d1 := decimal.NewFromFloat(0.01)
	d2 := decimal.NewFromFloat(0.02)
	upTo1000 := int64(1000)

	p1 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	p2 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	assert.True(t, p1.Equals(p2))

	// Different mode should not be equal
	p3 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeVolume,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	p4 := ModelPrice{
		Items: []ModelPriceItem{
			{
				ItemCode: PriceItemCodeUsage,
				Pricing: Pricing{
					Mode: PricingModeTiered,
					UsageTiered: &TieredPricing{
						Tiers: []PriceTier{
							{UpTo: &upTo1000, PricePerUnit: d1},
							{UpTo: nil, PricePerUnit: d2},
						},
					},
				},
			},
		},
	}

	assert.False(t, p3.Equals(p4))
}

func TestModelPrice_Validate(t *testing.T) {
	t.Run("flat_fee requires flatFee", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeFlatFee},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "flatFee is required")
	})

	t.Run("usage_per_unit requires usagePerUnit", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeUsagePerUnit},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usagePerUnit is required")
	})

	t.Run("tiered requires last upTo null and others non-null", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeTiered,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[0].upTo is required")
	})

	t.Run("tiered last upTo must be null", func(t *testing.T) {
		upTo1000 := int64(1000)
		upTo2000 := int64(2000)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeTiered,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: &upTo1000, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: &upTo2000, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[1].upTo must be null")
	})

	t.Run("volume requires usageTiered", func(t *testing.T) {
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing:  Pricing{Mode: PricingModeVolume},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "usageTiered is required")
	})

	t.Run("volume validates tiers same as tiered", func(t *testing.T) {
		upTo1000 := int64(1000)
		upTo2000 := int64(2000)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeUsage,
					Pricing: Pricing{
						Mode: PricingModeVolume,
						UsageTiered: &TieredPricing{
							Tiers: []PriceTier{
								{UpTo: &upTo1000, PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: &upTo2000, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "tiers[1].upTo must be null")
	})

	t.Run("variant pricing is also validated", func(t *testing.T) {
		d := decimal.NewFromFloat(0.01)
		mp := ModelPrice{
			Items: []ModelPriceItem{
				{
					ItemCode: PriceItemCodeWriteCachedTokens,
					Pricing:  Pricing{Mode: PricingModeUsagePerUnit, UsagePerUnit: &d},
					PromptWriteCacheVariants: []PromptWriteCacheVariant{
						{
							VariantCode: PromptWriteCacheVariantCode5Min,
							Pricing:     Pricing{Mode: PricingModeUsagePerUnit},
						},
					},
				},
			},
		}

		err := mp.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "promptWriteCacheVariants[0]")
		assert.Contains(t, err.Error(), "usagePerUnit is required")
	})
}

func TestModelPrice_ValidateVolumeTiers(t *testing.T) {
	newPrice := func() ModelPrice {
		value := decimal.NewFromInt(1)
		pricing := Pricing{Mode: PricingModeUsagePerUnit, UsagePerUnit: &value}
		items := func() []ModelPriceItem {
			return []ModelPriceItem{
				{ItemCode: PriceItemCodeUsage, Pricing: pricing},
				{
					ItemCode: PriceItemCodeWriteCachedTokens,
					Pricing:  pricing,
					PromptWriteCacheVariants: []PromptWriteCacheVariant{
						{VariantCode: PromptWriteCacheVariantCode5Min, Pricing: pricing},
						{VariantCode: PromptWriteCacheVariantCode1Hour, Pricing: pricing},
					},
				},
			}
		}
		return ModelPrice{
			Items:       items(),
			VolumeTiers: []ModelPriceVolumeTier{{Above: 0, Items: items()}},
		}
	}

	t.Run("zero threshold and reordered sets are valid", func(t *testing.T) {
		price := newPrice()
		items := price.VolumeTiers[0].Items
		variants := items[1].PromptWriteCacheVariants
		variants[0], variants[1] = variants[1], variants[0]
		items[0], items[1] = items[1], items[0]
		require.NoError(t, price.Validate())
	})

	for _, tt := range []struct {
		name   string
		mutate func(*ModelPrice)
	}{
		{"negative threshold", func(p *ModelPrice) { p.VolumeTiers[0].Above = -1 }},
		{"duplicate threshold", func(p *ModelPrice) {
			p.VolumeTiers = append(p.VolumeTiers, p.VolumeTiers[0])
		}},
		{"descending thresholds", func(p *ModelPrice) {
			p.VolumeTiers = append(p.VolumeTiers, p.VolumeTiers[0])
			p.VolumeTiers[0].Above = 1000
		}},
		{"empty base and tier", func(p *ModelPrice) {
			p.Items, p.VolumeTiers[0].Items = nil, nil
		}},
		{"missing item", func(p *ModelPrice) {
			p.VolumeTiers[0].Items = p.VolumeTiers[0].Items[:1]
		}},
		{"extra item", func(p *ModelPrice) {
			item := p.Items[0]
			item.ItemCode = PriceItemCodeCompletion
			p.VolumeTiers[0].Items = append(p.VolumeTiers[0].Items, item)
		}},
		{"different item set", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[0].ItemCode = PriceItemCodeCompletion
		}},
		{"duplicate base item", func(p *ModelPrice) { p.Items = append(p.Items, p.Items[0]) }},
		{"duplicate tier item", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[1] = p.VolumeTiers[0].Items[0]
		}},
		{"missing variant", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[1].PromptWriteCacheVariants = p.VolumeTiers[0].Items[1].PromptWriteCacheVariants[:1]
		}},
		{"extra variant", func(p *ModelPrice) {
			p.Items[1].PromptWriteCacheVariants = p.Items[1].PromptWriteCacheVariants[:1]
		}},
		{"different variant set", func(p *ModelPrice) {
			p.Items[1].PromptWriteCacheVariants = p.Items[1].PromptWriteCacheVariants[:1]
			p.VolumeTiers[0].Items[1].PromptWriteCacheVariants = p.VolumeTiers[0].Items[1].PromptWriteCacheVariants[1:]
		}},
		{"duplicate base variant", func(p *ModelPrice) {
			p.Items[1].PromptWriteCacheVariants[1] = p.Items[1].PromptWriteCacheVariants[0]
		}},
		{"duplicate tier variant", func(p *ModelPrice) {
			variants := p.VolumeTiers[0].Items[1].PromptWriteCacheVariants
			variants[1] = variants[0]
		}},
		{"variants on another item", func(p *ModelPrice) {
			p.Items[0].PromptWriteCacheVariants = p.Items[1].PromptWriteCacheVariants
		}},
		{"missing tier rate", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[0].Pricing.UsagePerUnit = nil
		}},
		{"negative tier rate", func(p *ModelPrice) {
			value := decimal.NewFromInt(-1)
			p.VolumeTiers[0].Items[0].Pricing.UsagePerUnit = &value
		}},
		{"negative variant rate", func(p *ModelPrice) {
			value := decimal.NewFromInt(-1)
			p.VolumeTiers[0].Items[1].PromptWriteCacheVariants[0].Pricing.UsagePerUnit = &value
		}},
		{"missing variant rate", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[1].PromptWriteCacheVariants[0].Pricing.UsagePerUnit = nil
		}},
		{"flat fee base item", func(p *ModelPrice) {
			p.Items[0].Pricing = Pricing{Mode: PricingModeFlatFee, FlatFee: p.Items[0].Pricing.UsagePerUnit}
		}},
		{"legacy volume tier item", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[0].Pricing = Pricing{
				Mode:        PricingModeVolume,
				UsageTiered: &TieredPricing{Tiers: []PriceTier{{PricePerUnit: decimal.NewFromInt(1)}}},
			}
		}},
		{"legacy tiered variant", func(p *ModelPrice) {
			p.VolumeTiers[0].Items[1].PromptWriteCacheVariants[0].Pricing = Pricing{
				Mode:        PricingModeTiered,
				UsageTiered: &TieredPricing{Tiers: []PriceTier{{PricePerUnit: decimal.NewFromInt(1)}}},
			}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			price := newPrice()
			tt.mutate(&price)
			require.Error(t, price.Validate())
		})
	}
}

func TestModelPrice_EqualsVolumeTiers(t *testing.T) {
	newPrice := func() ModelPrice {
		value := decimal.NewFromInt(1)
		item := func() ModelPriceItem {
			return ModelPriceItem{
				ItemCode: PriceItemCodeWriteCachedTokens,
				Pricing:  Pricing{Mode: PricingModeUsagePerUnit, UsagePerUnit: &value},
				PromptWriteCacheVariants: []PromptWriteCacheVariant{{
					VariantCode: PromptWriteCacheVariantCode1Hour,
					Pricing:     Pricing{Mode: PricingModeUsagePerUnit, UsagePerUnit: &value},
				}},
			}
		}
		return ModelPrice{
			Items:       []ModelPriceItem{item()},
			VolumeTiers: []ModelPriceVolumeTier{{Above: 1000, Items: []ModelPriceItem{item()}}},
		}
	}
	price := newPrice()
	require.True(t, price.Equals(newPrice()))

	for _, tt := range []struct {
		name   string
		mutate func(*ModelPrice)
	}{
		{"removed tier", func(p *ModelPrice) { p.VolumeTiers = nil }},
		{"changed threshold", func(p *ModelPrice) { p.VolumeTiers[0].Above++ }},
		{"changed tier rate", func(p *ModelPrice) {
			value := decimal.NewFromInt(2)
			p.VolumeTiers[0].Items[0].Pricing.UsagePerUnit = &value
		}},
		{"changed variant rate", func(p *ModelPrice) {
			value := decimal.NewFromInt(2)
			p.VolumeTiers[0].Items[0].PromptWriteCacheVariants[0].Pricing.UsagePerUnit = &value
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			other := newPrice()
			tt.mutate(&other)
			require.False(t, price.Equals(other))
		})
	}
}
