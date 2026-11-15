package biz

import (
	"context"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channelmodelpriceversion"
)

// LoadUsagePriceMultipliers reads fallback references and historical price snapshots.
// The caller must first restrict usage logs to records visible to the requester;
// the fixed host read returns only their matching version snapshots.
func LoadUsagePriceMultipliers(ctx context.Context, client *ent.Client, referenceIDs []string) (map[string]decimal.Decimal, error) {
	multipliers := make(map[string]decimal.Decimal, len(referenceIDs))
	var versionReferences []string
	for _, referenceID := range referenceIDs {
		if multiplier, ok := modelLibraryPriceReferenceMultiplier(referenceID); ok {
			multipliers[referenceID] = multiplier
			continue
		}
		if versionReferences == nil {
			versionReferences = make([]string, 0, len(referenceIDs))
		}
		versionReferences = append(versionReferences, referenceID)
	}
	if len(versionReferences) == 0 {
		return multipliers, nil
	}
	versions, err := authz.RunAsHost(ctx, client, "usage-price-multipliers", func(ctx context.Context) ([]*ent.ChannelModelPriceVersion, error) {
		return client.ChannelModelPriceVersion.Query().Where(channelmodelpriceversion.ReferenceIDIn(versionReferences...)).Select(
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
