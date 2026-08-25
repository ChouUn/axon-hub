package gql

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
)

func TestAPIKeyUsesTemplate(t *testing.T) {
	templateID := 42

	tests := []struct {
		name     string
		apiKey   *ent.APIKey
		template map[int]struct{}
		expected bool
	}{
		{
			name: "matches a loaded template profile",
			apiKey: &ent.APIKey{
				Profiles: &objects.APIKeyProfiles{
					Profiles: []objects.APIKeyProfile{
						{TemplateID: &templateID},
					},
				},
			},
			template: map[int]struct{}{templateID: {}},
			expected: true,
		},
		{
			name: "does not match an independently managed profile",
			apiKey: &ent.APIKey{
				Profiles: &objects.APIKeyProfiles{
					Profiles: []objects.APIKeyProfile{
						{Name: "custom"},
					},
				},
			},
			template: map[int]struct{}{templateID: {}},
			expected: false,
		},
		{
			name:     "does not match a missing profile collection",
			apiKey:   &ent.APIKey{},
			template: map[int]struct{}{templateID: {}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(
				t,
				tt.expected,
				apiKeyUsesTemplate(tt.apiKey, tt.template),
			)
		})
	}
}

func TestIntersectAPIKeyIDs(t *testing.T) {
	result := intersectAPIKeyIDs([]int{1, 2, 3}, []int{2, 3, 4}, []int{3, 5})
	require.ElementsMatch(t, []int{3}, result)

	require.Empty(t, intersectAPIKeyIDs([]int{1}, []int{}))
	require.Nil(t, intersectAPIKeyIDs())
}

func TestSortAPIKeyStatsByCost(t *testing.T) {
	stats := []*AnalyticsAPIKeyStat{
		{Name: "low cost", Cost: 1, RequestCount: 100, TotalTokens: 1000},
		{
			Name: "high cost low requests", Cost: 10,
			RequestCount: 5, TotalTokens: 500,
		},
		{
			Name: "high cost high requests", Cost: 10,
			RequestCount: 10, TotalTokens: 100,
		},
	}

	sortAPIKeyStatsByCost(stats)

	require.Equal(t, "high cost high requests", stats[0].Name)
	require.Equal(t, "high cost low requests", stats[1].Name)
	require.Equal(t, "low cost", stats[2].Name)
}
