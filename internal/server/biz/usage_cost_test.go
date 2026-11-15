package biz

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/channelmodelpriceversion"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/intercept"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/llm"
)

func createPriceItem(mode objects.PricingMode, unit float64) objects.ModelPriceItem {
	switch mode {
	case objects.PricingModeFlatFee:
		d := decimal.NewFromFloat(unit)

		return objects.ModelPriceItem{
			ItemCode: objects.PriceItemCodeUsage,
			Pricing: objects.Pricing{
				Mode:    mode,
				FlatFee: &d,
			},
		}
	case objects.PricingModeUsagePerUnit:
		d := decimal.NewFromFloat(unit)

		return objects.ModelPriceItem{
			ItemCode: objects.PriceItemCodeUsage,
			Pricing: objects.Pricing{
				Mode:         mode,
				UsagePerUnit: &d,
			},
		}
	default:
		return objects.ModelPriceItem{}
	}
}

func TestUsageCost_PerUnitPromptAndCompletion(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c1").
		SetSupportedModels([]string{"m1"}).
		SetDefaultTestModel("m1").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	// price: prompt_tokens $0.01 per token, completion_tokens $0.02 per token
	promptUnit := decimal.NewFromFloat(0.01)
	completionUnit := decimal.NewFromFloat(0.02)
	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m1").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &promptUnit},
				},
				{
					ItemCode: objects.PriceItemCodeCompletion,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &completionUnit},
				},
			},
		}).
		SetReferenceID("ref-1").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m1",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (100/1e6)*0.01 + (200/1e6)*0.02 = 0.000005
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.000005, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 2)
}

func TestUsageCost_TieredPrompt(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c2").
		SetSupportedModels([]string{"m2"}).
		SetDefaultTestModel("m2").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m2").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing: objects.Pricing{
						Mode: objects.PricingModeTiered,
						UsageTiered: &objects.TieredPricing{
							Tiers: []objects.PriceTier{
								{UpTo: lo.ToPtr(int64(1000)), PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-2").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     1500,
		CompletionTokens: 0,
		TotalTokens:      1500,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m2",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (1000/1e6)*0.01 + (500/1e6)*0.02 = 0.00002
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.00002, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 1)
	require.Len(t, ul.CostItems[0].TierBreakdown, 2)
}

func TestUsageCost_VolumePrompt(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c8").
		SetSupportedModels([]string{"m8"}).
		SetDefaultTestModel("m8").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m8").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing: objects.Pricing{
						Mode: objects.PricingModeVolume,
						UsageTiered: &objects.TieredPricing{
							Tiers: []objects.PriceTier{
								{UpTo: lo.ToPtr(int64(1000)), PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-8").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     1500,
		CompletionTokens: 0,
		TotalTokens:      1500,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m8",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (1500/1e6)*0.02 = 0.00003 (all tokens billed at tier 2 price)
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.00003, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 1)
	require.Len(t, ul.CostItems[0].TierBreakdown, 1)
	require.Equal(t, int64(1500), ul.CostItems[0].TierBreakdown[0].Units)
}

func TestUsageCost_VolumePromptFirstTier(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c9").
		SetSupportedModels([]string{"m9"}).
		SetDefaultTestModel("m9").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m9").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing: objects.Pricing{
						Mode: objects.PricingModeVolume,
						UsageTiered: &objects.TieredPricing{
							Tiers: []objects.PriceTier{
								{UpTo: lo.ToPtr(int64(1000)), PricePerUnit: decimal.NewFromFloat(0.01)},
								{UpTo: nil, PricePerUnit: decimal.NewFromFloat(0.02)},
							},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-9").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     800,
		CompletionTokens: 0,
		TotalTokens:      800,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m9",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (800/1e6)*0.01 = 0.000008 (all tokens billed at tier 1 price)
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.000008, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 1)
	require.Len(t, ul.CostItems[0].TierBreakdown, 1)
	require.Equal(t, int64(800), ul.CostItems[0].TierBreakdown[0].Units)
}

func TestUsageCost_NoPriceConfigured(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c3").
		SetSupportedModels([]string{"m3"}).
		SetDefaultTestModel("m3").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	// preloadModelPrices not called -> no prices cached
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m3",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)
	require.Nil(t, ul.TotalCost)
	require.Len(t, ul.CostItems, 0)
}

func TestUsageCost_CacheVariant5Min(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c4").
		SetSupportedModels([]string{"m4"}).
		SetDefaultTestModel("m4").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	// price: prompt_tokens $0.01, write_cached_tokens $0.04 (shared), 5m variant $0.03
	promptUnit := decimal.NewFromFloat(0.01)
	writeCacheShared := decimal.NewFromFloat(0.04)
	writeCache5Min := decimal.NewFromFloat(0.03)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m4").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &promptUnit},
				},
				{
					ItemCode: objects.PriceItemCodeWriteCachedTokens,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCacheShared},
					PromptWriteCacheVariants: []objects.PromptWriteCacheVariant{
						{
							VariantCode: objects.PromptWriteCacheVariantCode5Min,
							Pricing:     objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCache5Min},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-4").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
		PromptTokensDetails: &llm.PromptTokensDetails{
			WriteCachedTokens:      50,
			WriteCached5MinTokens:  50,
			WriteCached1HourTokens: 0,
		},
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m4",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: ((100-50)/1e6)*0.01 + (50/1e6)*0.03 = 0.000002
	// Input tokens now exclude WriteCachedTokens: (50/1e6)*0.01 = 0.0000005
	// Write cached 5min: (50/1e6)*0.03 = 0.0000015
	// Total: 0.000002
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.000002, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 2)
	require.Equal(t, int64(50), ul.PromptWriteCachedTokens5m)
}

func TestUsageCost_CacheVariant1Hour(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c5").
		SetSupportedModels([]string{"m5"}).
		SetDefaultTestModel("m5").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	// price: prompt_tokens $0.01, write_cached_tokens $0.04 (shared), 1h variant $0.02
	promptUnit := decimal.NewFromFloat(0.01)
	writeCacheShared := decimal.NewFromFloat(0.04)
	writeCache1Hour := decimal.NewFromFloat(0.02)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m5").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &promptUnit},
				},
				{
					ItemCode: objects.PriceItemCodeWriteCachedTokens,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCacheShared},
					PromptWriteCacheVariants: []objects.PromptWriteCacheVariant{
						{
							VariantCode: objects.PromptWriteCacheVariantCode1Hour,
							Pricing:     objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCache1Hour},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-5").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
		PromptTokensDetails: &llm.PromptTokensDetails{
			WriteCachedTokens:      80,
			WriteCached5MinTokens:  0,
			WriteCached1HourTokens: 80,
		},
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m5",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: ((100-80)/1e6)*0.01 + (80/1e6)*0.02 = 0.0000018
	// Input tokens now exclude WriteCachedTokens: (20/1e6)*0.01 = 0.0000002
	// Write cached 1hour: (80/1e6)*0.02 = 0.0000016
	// Total: 0.0000018
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0000018, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 2)
	require.Equal(t, int64(80), ul.PromptWriteCachedTokens1h)
}

func TestUsageCost_CacheVariantBoth5MinAnd1Hour(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c6").
		SetSupportedModels([]string{"m6"}).
		SetDefaultTestModel("m6").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	// price: prompt_tokens $0.01, write_cached_tokens $0.06 (shared), 5m variant $0.05, 1h variant $0.03
	promptUnit := decimal.NewFromFloat(0.01)
	writeCacheShared := decimal.NewFromFloat(0.06)
	writeCache5Min := decimal.NewFromFloat(0.05)
	writeCache1Hour := decimal.NewFromFloat(0.03)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m6").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &promptUnit},
				},
				{
					ItemCode: objects.PriceItemCodeWriteCachedTokens,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCacheShared},
					PromptWriteCacheVariants: []objects.PromptWriteCacheVariant{
						{
							VariantCode: objects.PromptWriteCacheVariantCode5Min,
							Pricing:     objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCache5Min},
						},
						{
							VariantCode: objects.PromptWriteCacheVariantCode1Hour,
							Pricing:     objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCache1Hour},
						},
					},
				},
			},
		}).
		SetReferenceID("ref-6").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
		PromptTokensDetails: &llm.PromptTokensDetails{
			WriteCachedTokens:      100,
			WriteCached5MinTokens:  40,
			WriteCached1HourTokens: 60,
		},
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m6",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: ((100-100)/1e6)*0.01 + (40/1e6)*0.05 + (60/1e6)*0.03 = 0.0000038
	// Input tokens now exclude WriteCachedTokens: (0/1e6)*0.01 = 0
	// Write cached 5min: (40/1e6)*0.05 = 0.000002
	// Write cached 1hour: (60/1e6)*0.03 = 0.0000018
	// Total: 0.0000038
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0000038, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 3)
	require.Equal(t, int64(40), ul.PromptWriteCachedTokens5m)
	require.Equal(t, int64(60), ul.PromptWriteCachedTokens1h)
}

func TestUsageCost_CacheVariantFallbackToShared(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("c7").
		SetSupportedModels([]string{"m7"}).
		SetDefaultTestModel("m7").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	// price: prompt_tokens $0.01, write_cached_tokens $0.04 (shared, no variants)
	promptUnit := decimal.NewFromFloat(0.01)
	writeCacheShared := decimal.NewFromFloat(0.04)

	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m7").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &promptUnit},
				},
				{
					ItemCode: objects.PriceItemCodeWriteCachedTokens,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &writeCacheShared},
					// No PromptWriteCacheVariants configured
				},
			},
		}).
		SetReferenceID("ref-7").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
		PromptTokensDetails: &llm.PromptTokensDetails{
			WriteCachedTokens:      70,
			WriteCached5MinTokens:  0,
			WriteCached1HourTokens: 0,
		},
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m7",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: ((100-70)/1e6)*0.01 + (70/1e6)*0.04 = 0.0000031
	// Input tokens now exclude WriteCachedTokens: (30/1e6)*0.01 = 0.0000003
	// Write cached (shared): (70/1e6)*0.04 = 0.0000028
	// Total: 0.0000031
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0000031, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 2)
	require.Equal(t, int64(70), ul.PromptWriteCachedTokens)
}

func createModelLibraryPriceEntry(t *testing.T, ctx context.Context, client *ent.Client, modelID string, status model.Status, cost objects.ModelCardCost) *ent.Model {
	t.Helper()
	var price *objects.ModelPrice
	for _, item := range []struct {
		code objects.PriceItemCode
		unit float64
	}{
		{objects.PriceItemCodeUsage, cost.Input},
		{objects.PriceItemCodeCompletion, cost.Output},
		{objects.PriceItemCodePromptCachedToken, cost.CacheRead},
		{objects.PriceItemCodeWriteCachedTokens, cost.CacheWrite},
	} {
		if item.unit <= 0 {
			continue
		}
		if price == nil {
			price = &objects.ModelPrice{}
		}
		price.Items = append(price.Items, objects.ModelPriceItem{
			ItemCode: item.code,
			Pricing: objects.Pricing{
				Mode:         objects.PricingModeUsagePerUnit,
				UsagePerUnit: lo.ToPtr(decimal.NewFromFloat(item.unit)),
			},
		})
	}

	entity, err := client.Model.Create().
		SetDeveloper("test").
		SetModelID(modelID).
		SetName("Test " + modelID).
		SetIcon("test").
		SetGroup("test").
		SetModelCard(&objects.ModelCard{Price: price}).
		SetSettings(&objects.ModelSettings{}).
		SetStatus(status).
		Save(ctx)
	require.NoError(t, err)

	return entity
}

// TestUsageCost_FallbackToModelLibraryPrice verifies that when a channel has no
// explicit price for a model, the cost is computed from the model library price
// configured under the Models menu.
func TestUsageCost_FallbackToModelLibraryPrice(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// Model library has a price, but the channel below has no channel-level price.
	createModelLibraryPriceEntry(t, ctx, client, "m-fallback", model.StatusEnabled, objects.ModelCardCost{
		Input:  1,
		Output: 2,
	})

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("fallback-channel").
		SetSupportedModels([]string{"m-fallback"}).
		SetDefaultTestModel("m-fallback").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	// Intentionally do NOT preload channel prices: the channel has none.
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m-fallback",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (100/1e6)*1 + (200/1e6)*2 = 0.0001 + 0.0004 = 0.0005
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0005, *ul.TotalCost, 1e-12)
	require.Len(t, ul.CostItems, 2)
	require.Equal(t, modelLibraryPriceReferenceID("m-fallback"), ul.CostPriceReferenceID)
}

// TestUsageCost_ChannelPriceWinsOverModelLibrary verifies that an explicit
// channel-level price still takes precedence over the model library fallback.
func TestUsageCost_ChannelPriceWinsOverModelLibrary(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createModelLibraryPriceEntry(t, ctx, client, "m-precedence", model.StatusEnabled, objects.ModelCardCost{
		Input: 100,
	})

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("precedence-channel").
		SetSupportedModels([]string{"m-precedence"}).
		SetDefaultTestModel("m-precedence").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	usageUnit := decimal.NewFromFloat(0.01)
	_, err = client.ChannelModelPrice.Create().
		SetChannelID(ch.ID).
		SetModelID("m-precedence").
		SetPrice(objects.ModelPrice{
			Items: []objects.ModelPriceItem{
				{
					ItemCode: objects.PriceItemCodeUsage,
					Pricing:  objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: &usageUnit},
				},
			},
		}).
		SetReferenceID("channel-ref").
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.preloadModelPrices(ctx, built)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{
		PromptTokens:     100,
		CompletionTokens: 0,
		TotalTokens:      100,
	}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m-precedence",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// Channel price (0.01/1M) must win over the model library price (100/1M).
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.000001, *ul.TotalCost, 1e-12)
	require.Equal(t, "channel-ref", ul.CostPriceReferenceID)
}

// TestUsageCost_FallbackSkipsUnpriceableModelLibraryEntries verifies that the
// fallback stays inert when the model library entry has no usable cost.
func TestUsageCost_FallbackSkipsUnpriceableModelLibraryEntries(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// Enabled model without cost, archived model with cost: neither is priceable.
	createModelLibraryPriceEntry(t, ctx, client, "m-no-cost", model.StatusEnabled, objects.ModelCardCost{})
	createModelLibraryPriceEntry(t, ctx, client, "m-archived", model.StatusArchived, objects.ModelCardCost{Input: 5})

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("unpriceable-channel").
		SetSupportedModels([]string{"m-no-cost", "m-archived", "m-missing"}).
		SetDefaultTestModel("m-no-cost").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 0, TotalTokens: 100}

	for _, modelID := range []string{"m-no-cost", "m-archived", "m-missing"} {
		ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
			RequestID:     1,
			ProjectID:     1,
			ChannelID:     ch.ID,
			ActualModelID: modelID,
			Usage:         usage,
			Source:        "api",
			Format:        "openai/chat_completions",
			APIKeyID:      nil,
		})
		require.NoError(t, err)
		require.NotNil(t, ul)
		require.Nil(t, ul.TotalCost, "model %s should have no cost", modelID)
		require.Empty(t, ul.CostItems)
		require.Empty(t, ul.CostPriceReferenceID)
	}
}

// TestUsageCost_FallbackStripsEffortSuffix verifies that when the model library
// has no exact entry for a suffixed model id, the fallback strips the known
// effort/tier suffix and bills with the base model price.
func TestUsageCost_FallbackStripsEffortSuffix(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	// Only the base model "m-suffix" exists in the library; the channel serves
	// "m-suffix-high".
	createModelLibraryPriceEntry(t, ctx, client, "m-suffix", model.StatusEnabled, objects.ModelCardCost{
		Input:  1,
		Output: 2,
	})

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("suffix-channel").
		SetSupportedModels([]string{"m-suffix-high"}).
		SetDefaultTestModel("m-suffix-high").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m-suffix-high",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// expected total: (100/1e6)*1 + (200/1e6)*2 = 0.0005
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0005, *ul.TotalCost, 1e-12)
	// Reference id keeps the requested (suffixed) model id for traceability.
	require.Equal(t, modelLibraryPriceReferenceID("m-suffix-high"), ul.CostPriceReferenceID)
}

// TestUsageCost_FallbackPrefersExactModelBeforeSuffix verifies that an exact
// model library entry for the suffixed id wins over the stripped base model.
func TestUsageCost_FallbackPrefersExactModelBeforeSuffix(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createModelLibraryPriceEntry(t, ctx, client, "m-exact", model.StatusEnabled, objects.ModelCardCost{Input: 10})
	createModelLibraryPriceEntry(t, ctx, client, "m-exact-high", model.StatusEnabled, objects.ModelCardCost{Input: 1})
	{
		n, qerr := client.Model.Query().Count(ctx)
		require.NoError(t, qerr)
		require.Equal(t, 2, n, "both library entries must exist")
		m, merr := client.Model.Query().Where(model.ModelID("m-exact-high")).Only(ctx)
		require.NoError(t, merr, "exact entry must be queryable")
		require.Equal(t, float64(1), m.ModelCard.Cost.Input)
	}

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenaiFake).
		SetName("exact-channel").
		SetSupportedModels([]string{"m-exact-high"}).
		SetDefaultTestModel("m-exact-high").
		SetStatus(channel.StatusEnabled).
		SetCredentials(objects.ChannelCredentials{}).
		Save(ctx)
	require.NoError(t, err)

	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	built, err := channelService.GetChannel(ctx, ch.ID)
	require.NoError(t, err)
	channelService.SetEnabledChannelsForTest([]*Channel{built})

	usageLogService := NewUsageLogService(client, systemService, channelService)

	usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 0, TotalTokens: 100}

	ul, err := usageLogService.CreateUsageLog(ctx, CreateUsageLogParams{
		RequestID:     1,
		ProjectID:     1,
		ChannelID:     ch.ID,
		ActualModelID: "m-exact-high",
		Usage:         usage,
		Source:        "api",
		Format:        "openai/chat_completions",
		APIKeyID:      nil,
	})
	require.NoError(t, err)
	require.NotNil(t, ul)

	// Exact "m-exact-high" price (1/1M) must win over base "m-exact" (10/1M).
	// (100 / 1e6) * 1 = 0.0001; the base price would yield 0.001.
	require.NotNil(t, ul.TotalCost)
	require.InDelta(t, 0.0001, *ul.TotalCost, 1e-12)
}

// TestModelLibraryPriceModelIDCandidates covers candidate ordering and de-dup.
func TestModelLibraryPriceModelIDCandidates(t *testing.T) {
	require.Equal(t, []string{"m"}, modelLibraryPriceModelIDCandidates("m"))
	require.Equal(t, []string{"m-high", "m"}, modelLibraryPriceModelIDCandidates("m-high"))
	require.Equal(t, []string{"m-low", "m"}, modelLibraryPriceModelIDCandidates("m-low"))
	require.Equal(t, []string{"m-medium", "m"}, modelLibraryPriceModelIDCandidates("m-medium"))
	// A trailing dash is not a known suffix; nothing is stripped.
	require.Equal(t, []string{"m-high-"}, modelLibraryPriceModelIDCandidates("m-high-"))
}

// TestModelLibraryPriceMemo_ReusesLookupWithinRequest verifies that repeated
// resolutions inside one request reuse a single catalog lookup, including a
// miss, while a fresh request uses the updated shared Model cache.
func TestModelLibraryPriceMemo_ReusesLookupWithinRequest(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createModelLibraryPriceEntry(t, ctx, client, "m-memo", model.StatusEnabled, objects.ModelCardCost{Input: 1})

	channelService := NewChannelServiceForTest(client)

	// --- inside a memoized request: the miss is cached ---
	memoCtx := WithModelLibraryPriceMemo(ctx)

	price, ok := channelService.resolveModelLibraryPrice(memoCtx, "m-memo")
	require.True(t, ok)
	require.Equal(t, "1", price.Items[0].Pricing.UsagePerUnit.String())

	// A model absent from the library is cached as a miss within the request.
	_, missing := channelService.resolveModelLibraryPrice(memoCtx, "m-absent")
	require.False(t, missing)

	// Add the previously missing model; the memoized request must not observe it.
	createModelLibraryPriceEntry(t, ctx, client, "m-absent", model.StatusEnabled, objects.ModelCardCost{Input: 9})

	_, stillMissing := channelService.resolveModelLibraryPrice(memoCtx, "m-absent")
	require.False(t, stillMissing, "memoized miss must be reused within the request")

	// The mutation hook invalidated the shared key; a fresh request loads it.
	freshCtx := WithModelLibraryPriceMemo(ctx)
	added, addedOK := channelService.resolveModelLibraryPrice(freshCtx, "m-absent")
	require.True(t, addedOK)
	require.Equal(t, "9", added.Items[0].Pricing.UsagePerUnit.String())
}

// TestWithModelLibraryPriceMemo_Idempotent verifies the memo is installed once
// per context chain.
func TestWithModelLibraryPriceMemo_Idempotent(t *testing.T) {
	ctx := WithModelLibraryPriceMemo(context.Background())
	require.NotNil(t, modelLibraryPriceMemoFromContext(ctx))

	second := WithModelLibraryPriceMemo(ctx)
	require.Same(t, modelLibraryPriceMemoFromContext(ctx), modelLibraryPriceMemoFromContext(second))
}

// TestModelLibraryPriceMemo_DoesNotCacheLookupErrors verifies that a failed
// lookup (e.g. a canceled context) is not cached, so a later calculation with a
// healthy context still resolves the price. This guards the usage-log write,
// which runs with cancellation removed but shares the request memo.
func TestModelLibraryPriceMemo_DoesNotCacheLookupErrors(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)

	createModelLibraryPriceEntry(t, ctx, client, "m-err", model.StatusEnabled, objects.ModelCardCost{Input: 2})

	channelService := NewChannelServiceForTest(client)

	memoCtx := WithModelLibraryPriceMemo(ctx)

	// A canceled context makes the lookup fail; the result must not be memoized.
	canceledCtx, cancel := context.WithCancel(memoCtx)
	cancel()

	_, ok := channelService.resolveModelLibraryPrice(canceledCtx, "m-err")
	require.False(t, ok)

	// The same request, now with a healthy context, must still resolve the price.
	price, ok := channelService.resolveModelLibraryPrice(memoCtx, "m-err")
	require.True(t, ok)
	require.Equal(t, "2", price.Items[0].Pricing.UsagePerUnit.String())
}

func TestUsageCost_ModelLibraryChannelMultiplier(t *testing.T) {
	for _, tc := range []struct {
		name       string
		multiplier *decimal.Decimal
		modelID    string
		reference  string
	}{
		{name: "unset", modelID: "provider:model", reference: "model_library:provider:model"},
		{name: "unit", multiplier: lo.ToPtr(decimal.NewFromInt(1)), modelID: "provider:model", reference: "model_library:provider:model"},
		{name: "half", multiplier: lo.ToPtr(decimal.RequireFromString("0.5")), modelID: "provider:model", reference: "model_library@0.5:provider:model"},
		{name: "zero", multiplier: lo.ToPtr(decimal.Zero), modelID: "provider:model", reference: "model_library@0:provider:model"},
		{name: "suffix", multiplier: lo.ToPtr(decimal.RequireFromString("0.5")), modelID: "provider:model-high", reference: "model_library@0.5:provider:model-high"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := enttest.NewEntClient(t, "sqlite3", "file:fallback-multiplier?mode=memory&_fk=0")
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
			catalog := createModelLibraryPriceEntry(t, ctx, client, "provider:model", model.StatusEnabled, objects.ModelCardCost{
				Input: 1, Output: 2, CacheRead: 0.25, CacheWrite: 1.25,
			})
			create := client.Channel.Create().SetType(channel.TypeOpenaiFake).SetName(tc.name).
				SetSupportedModels([]string{tc.modelID}).SetDefaultTestModel(tc.modelID).
				SetStatus(channel.StatusEnabled).SetCredentials(objects.ChannelCredentials{})
			if tc.multiplier != nil {
				create.SetSettings(&objects.ChannelSettings{ModelPriceMultiplier: tc.multiplier})
			}
			ch := create.SaveX(ctx)
			system := NewSystemService(SystemServiceParams{Ent: client})
			channels := NewChannelServiceForTest(client)
			t.Cleanup(channels.Stop)
			built, err := channels.GetChannel(ctx, ch.ID)
			require.NoError(t, err)
			channels.SetEnabledChannelsForTest([]*Channel{built})
			service := NewUsageLogService(client, system, channels)
			usage := &llm.Usage{
				PromptTokens: 1000, CompletionTokens: 200, TotalTokens: 1200,
				PromptTokensDetails: &llm.PromptTokensDetails{CachedTokens: 200, WriteCachedTokens: 100},
			}
			log, err := service.CreateUsageLog(ctx, CreateUsageLogParams{
				RequestID: 1, ProjectID: 1, ChannelID: ch.ID, ActualModelID: tc.modelID,
				Usage: usage, Source: "api", Format: "openai/chat_completions",
			})
			require.NoError(t, err)
			factor := decimal.NewFromInt(1)
			if tc.multiplier != nil {
				factor = *tc.multiplier
			}
			require.NotNil(t, log.TotalCost)
			require.InDelta(t, decimal.RequireFromString("0.001275").Mul(factor).InexactFloat64(), *log.TotalCost, 1e-12)
			require.Equal(t, tc.reference, log.CostPriceReferenceID)
			require.Len(t, log.CostItems, 4)
			for i, expected := range []struct {
				code     objects.PriceItemCode
				quantity int64
				subtotal string
			}{
				{objects.PriceItemCodeUsage, 700, "0.0007"},
				{objects.PriceItemCodeCompletion, 200, "0.0004"},
				{objects.PriceItemCodePromptCachedToken, 200, "0.00005"},
				{objects.PriceItemCodeWriteCachedTokens, 100, "0.000125"},
			} {
				require.Equal(t, expected.code, log.CostItems[i].ItemCode)
				require.Equal(t, expected.quantity, log.CostItems[i].Quantity)
				require.True(t, log.CostItems[i].Subtotal.Equal(decimal.RequireFromString(expected.subtotal).Mul(factor)))
			}
			require.Nil(t, client.Model.GetX(ctx, catalog.ID).ModelCard.Price.Multiplier)
		})
	}
}

func TestUsageCost_ModelLibraryMemoKeepsChannelMultipliersSeparate(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:fallback-multiplier-memo?mode=memory&_fk=0")
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	catalog := createModelLibraryPriceEntry(t, ctx, client, "model", model.StatusEnabled, objects.ModelCardCost{Input: 1, Output: 2})
	channels := NewChannelServiceForTest(client)
	t.Cleanup(channels.Stop)
	var builtChannels []*Channel
	for _, factor := range []string{"0.5", "2"} {
		ch := client.Channel.Create().SetType(channel.TypeOpenaiFake).SetName(factor).
			SetSupportedModels([]string{"model"}).SetDefaultTestModel("model").SetStatus(channel.StatusEnabled).
			SetCredentials(objects.ChannelCredentials{}).
			SetSettings(&objects.ChannelSettings{ModelPriceMultiplier: lo.ToPtr(decimal.RequireFromString(factor))}).SaveX(ctx)
		built, err := channels.GetChannel(ctx, ch.ID)
		require.NoError(t, err)
		builtChannels = append(builtChannels, built)
	}
	channels.SetEnabledChannelsForTest(builtChannels)
	service := NewUsageLogService(client, NewSystemService(SystemServiceParams{Ent: client}), channels)
	memoCtx := WithModelLibraryPriceMemo(ctx)
	for i, index := range []int{0, 1, 0} {
		ch := builtChannels[index]
		factor := *ch.Settings.ModelPriceMultiplier
		usage := &llm.Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300}
		service.InjectUsageCost(memoCtx, ch.ID, "model", usage)
		if i == 0 {
			// Invalidate the shared cache mid-request: the rest of this request must
			// keep the price its cost injection used, across both channels.
			card := *catalog.ModelCard
			card.Price = &objects.ModelPrice{Items: []objects.ModelPriceItem{
				{ItemCode: objects.PriceItemCodeUsage, Pricing: objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: lo.ToPtr(decimal.NewFromInt(10))}},
				{ItemCode: objects.PriceItemCodeCompletion, Pricing: objects.Pricing{Mode: objects.PricingModeUsagePerUnit, UsagePerUnit: lo.ToPtr(decimal.NewFromInt(20))}},
			}}
			client.Model.UpdateOneID(catalog.ID).SetModelCard(&card).ExecX(ctx)
		}
		expected := decimal.RequireFromString("0.0005").Mul(factor).InexactFloat64()
		require.NotNil(t, usage.Cost)
		require.InDelta(t, expected, *usage.Cost, 1e-12)
		log, err := service.CreateUsageLog(memoCtx, CreateUsageLogParams{
			RequestID: 1, ProjectID: 1, ChannelID: ch.ID, ActualModelID: "model",
			Usage: usage, Source: "api", Format: "openai/chat_completions",
		})
		require.NoError(t, err)
		require.NotNil(t, log.TotalCost)
		require.InDelta(t, expected, *log.TotalCost, 1e-12)
		require.Equal(t, modelLibraryPriceReferenceIDWithMultiplier("model", &factor), log.CostPriceReferenceID)
	}
	cachedModel, err := channels.modelCache.get(ctx, client, "model")
	require.NoError(t, err)
	require.Nil(t, cachedModel.ModelCard.Price.Multiplier, "channel multipliers must not leak into the shared model cache")
	price, found, cached := modelLibraryPriceMemoFromContext(memoCtx).get("model")
	require.True(t, found)
	require.True(t, cached)
	require.Nil(t, price.Multiplier, "the memo must retain the unmodified standard price")
	require.True(t, price.Items[0].Pricing.UsagePerUnit.Equal(decimal.NewFromInt(1)), "the memo must keep the price the request started with")
	freshUsage := &llm.Usage{PromptTokens: 100, CompletionTokens: 200, TotalTokens: 300}
	service.InjectUsageCost(WithModelLibraryPriceMemo(ctx), builtChannels[0].ID, "model", freshUsage)
	require.NotNil(t, freshUsage.Cost)
	require.InDelta(t, decimal.RequireFromString("0.005").Mul(*builtChannels[0].Settings.ModelPriceMultiplier).InexactFloat64(), *freshUsage.Cost, 1e-12,
		"a new request must see the updated model library price")
}

func TestModelLibraryPriceReferenceIDWithMultiplier(t *testing.T) {
	for _, tc := range []struct {
		multiplier *decimal.Decimal
		expected   string
	}{
		{nil, "model_library:provider:model"},
		{lo.ToPtr(decimal.NewFromInt(1)), "model_library:provider:model"},
		{lo.ToPtr(decimal.RequireFromString("0.500")), "model_library@0.5:provider:model"},
		{lo.ToPtr(decimal.Zero), "model_library@0:provider:model"},
	} {
		require.Equal(t, tc.expected, modelLibraryPriceReferenceIDWithMultiplier("provider:model", tc.multiplier))
	}
}

func TestLoadUsagePriceMultipliersModelLibraryReferences(t *testing.T) {
	for _, tc := range []struct {
		reference string
		expected  string
	}{
		{"model_library:model", "1"},
		{"model_library:provider:model", "1"},
		{"model_library@0.5:provider:model", "0.5"},
		{"model_library@0:model", "0"},
		{"model_library@2:model", "2"},
		{"model_library@invalid:model", "1"},
		{"model_library@:model", "1"},
		{"model_library@-1:model", "1"},
		{"model_library@0.5", "1"},
		{"model_library@0.5:", "1"},
	} {
		// A nil client proves that fallback references never query price versions.
		multipliers, err := LoadUsagePriceMultipliers(context.Background(), nil, []string{tc.reference})
		require.NoError(t, err)
		require.Equal(t, tc.expected, multipliers[tc.reference].String(), tc.reference)
	}
}

func TestLoadUsagePriceMultipliersMixedReferences(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:fallback-multiplier-mixed?mode=memory&_fk=0")
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	ctx := authz.WithTestBypass(context.Background())
	ch := client.Channel.Create().SetType(channel.TypeOpenaiFake).SetName("mixed").
		SetSupportedModels([]string{"model"}).SetDefaultTestModel("model").SetCredentials(objects.ChannelCredentials{}).SaveX(ctx)
	price := client.ChannelModelPrice.Create().SetChannelID(ch.ID).SetModelID("model").
		SetPrice(objects.ModelPrice{}).SetReferenceID("current").SaveX(ctx)
	client.ChannelModelPriceVersion.Create().SetChannelID(ch.ID).SetChannelModelPriceID(price.ID).
		SetModelID("model").SetReferenceID("snapshot").SetEffectiveStartAt(time.Now()).
		SetStatus(channelmodelpriceversion.StatusArchived).
		SetPrice(objects.ModelPrice{Multiplier: lo.ToPtr(decimal.RequireFromString("0.25"))}).SaveX(ctx)
	queries := 0
	client.Intercept(intercept.Func(func(_ context.Context, query intercept.Query) error {
		if query.Type() == ent.TypeChannelModelPriceVersion {
			queries++
		}
		return nil
	}))
	multipliers, err := LoadUsagePriceMultipliers(context.Background(), client, []string{
		"snapshot", "model_library:model", "model_library@0.5:provider:model", "model_library@bad:model", "missing",
	})
	require.NoError(t, err)
	require.Equal(t, "0.25", multipliers["snapshot"].String())
	require.Equal(t, "1", multipliers["model_library:model"].String())
	require.Equal(t, "0.5", multipliers["model_library@0.5:provider:model"].String())
	require.Equal(t, "1", multipliers["model_library@bad:model"].String())
	require.NotContains(t, multipliers, "missing")
	require.Equal(t, 1, queries)
}

func TestModelLibraryPriceReferenceCanonicalFormat(t *testing.T) {
	frontendPattern := regexp.MustCompile(`^model_library(?:@(?:0|[1-9]\d*)(?:\.\d+)?)?:.+$`)
	for _, tc := range []struct {
		input     string
		canonical string
	}{
		{"0", "0"},
		{"0.5", "0.5"},
		{"1", "1"},
		{"1.25", "1.25"},
		{"1e-24", "0.000000000000000000000001"},
		{"1234567890123456789012345678901234567890", "1234567890123456789012345678901234567890"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			factor := decimal.RequireFromString(tc.input)
			require.Equal(t, tc.canonical, factor.String())
			require.Regexp(t, frontendPattern, modelLibraryPriceReferenceIDWithMultiplier("provider:model", &factor))
		})
	}
}
