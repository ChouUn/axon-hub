package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
)

func TestModelMapper_MapModel(t *testing.T) {
	ctx := context.Background()
	mapper := NewModelMapper()

	tests := []struct {
		name          string
		apiKey        *ent.APIKey
		originalModel string
		expectedModel string
	}{
		{
			name:          "nil api key",
			apiKey:        nil,
			originalModel: "gpt-4",
			expectedModel: "gpt-4",
		},
		{
			name: "no profiles",
			apiKey: &ent.APIKey{
				Name:     "test-key",
				Profiles: nil,
			},
			originalModel: "gpt-4",
			expectedModel: "gpt-4",
		},
		{
			name: "no active profile",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "gpt-4", To: "claude-3"},
							},
						},
					},
				},
			},
			originalModel: "gpt-4",
			expectedModel: "gpt-4",
		},
		{
			name: "active profile with exact match",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "profile1",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "gpt-4", To: "claude-3-opus"},
							},
						},
					},
				},
			},
			originalModel: "gpt-4",
			expectedModel: "claude-3-opus",
		},
		{
			name: "active profile with regexp match",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "profile1",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "gpt-.*", To: "claude-3-opus"},
							},
						},
					},
				},
			},
			originalModel: "gpt-4-turbo",
			expectedModel: "claude-3-opus",
		},
		{
			name: "active profile with regexp match 2",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "profile1",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "claude.*-haiku.*", To: "deepseek-chat"},
							},
						},
					},
				},
			},
			originalModel: "claude-haiku-4-5-20251001",
			expectedModel: "deepseek-chat",
		},
		{
			name: "active profile with no matching mapping",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "profile1",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "gpt-4", To: "claude-3-opus"},
							},
						},
					},
				},
			},
			originalModel: "gpt-3.5-turbo",
			expectedModel: "gpt-3.5-turbo",
		},
		{
			name: "active profile not found in profiles list",
			apiKey: &ent.APIKey{
				Name: "test-key",
				Profiles: &objects.APIKeyProfiles{
					ActiveProfile: "nonexistent",
					Profiles: []objects.APIKeyProfile{
						{
							Name: "profile1",
							ModelMappings: []objects.ModelMapping{
								{From: "gpt-4", To: "claude-3-opus"},
							},
						},
					},
				},
			},
			originalModel: "gpt-4",
			expectedModel: "gpt-4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapper.MapModel(ctx, tt.apiKey, tt.originalModel)
			assert.Equal(t, tt.expectedModel, result)
		})
	}
}
