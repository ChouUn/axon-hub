package biz

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channelmodelpriceversion"
)

// LoadUsagePriceMultipliers reads historical price snapshots by reference ID.
// The caller must first restrict usage logs to records visible to the requester;
// this bypass grants access only to the matching version snapshots.
func LoadUsagePriceMultipliers(ctx context.Context, client *ent.Client, referenceIDs []string) (map[string]decimal.Decimal, error) {
	multipliers := make(map[string]decimal.Decimal, len(referenceIDs))
	if len(referenceIDs) == 0 {
		return multipliers, nil
	}
	versions, err := authz.RunWithSystemBypass(ctx, "usage-price-multipliers", func(ctx context.Context) ([]*ent.ChannelModelPriceVersion, error) {
		return client.ChannelModelPriceVersion.Query().Where(channelmodelpriceversion.ReferenceIDIn(referenceIDs...)).Select(
			channelmodelpriceversion.FieldReferenceID, channelmodelpriceversion.FieldPrice,
		).All(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("query usage price versions: %w", err)
	}
	for _, version := range versions {
		multipliers[version.ReferenceID] = version.Price.EffectiveMultiplier()
	}
	return multipliers, nil
}
