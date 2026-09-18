package biz

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/model"
	"github.com/looplj/axonhub/internal/objects"
)

func TestValidateModelSettingsRoutingPolicy(t *testing.T) {
	settings := &objects.ModelSettings{}
	require.NoError(t, validateModelSettings(settings))
	require.Equal(t, objects.RoutingPolicyDefault, settings.LoadBalancerStrategy)
	require.Equal(t, objects.RoutingPolicyDefault, settings.TraceStickyMode)

	require.Error(t, validateModelSettings(&objects.ModelSettings{
		LoadBalancerStrategy: "unknown",
	}))
	require.Error(t, validateModelSettings(&objects.ModelSettings{
		TraceStickyMode: "unknown",
	}))
}

func TestModelService_QueryModelChannelConnections(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:ent?mode=memory&_fk=0")
	defer client.Close()

	ctx := context.Background()
	ctx = ent.NewContext(ctx, client)
	ctx = authz.WithTestBypass(ctx)
	svc := &ModelService{
		AbstractService: &AbstractService{
			db: client,
		},
	}

	// Create test channels
	channel1, err := client.Channel.Create().
		SetType("openai").
		SetName("OpenAI Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key-1"}}).
		SetSupportedModels([]string{"gpt-4", "gpt-3.5-turbo", "gpt-4-turbo"}).
		SetDefaultTestModel("gpt-4").
		Save(ctx)
	require.NoError(t, err)

	channel2, err := client.Channel.Create().
		SetType("anthropic").
		SetName("Anthropic Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key-2"}}).
		SetSupportedModels([]string{"claude-3-opus", "claude-3-sonnet", "claude-3-haiku"}).
		SetDefaultTestModel("claude-3-opus").
		Save(ctx)
	require.NoError(t, err)

	channel3, err := client.Channel.Create().
		SetType("gemini").
		SetName("Gemini Channel").
		SetStatus("enabled").
		SetCredentials(objects.ChannelCredentials{APIKeys: []string{"test-key-3"}}).
		SetSupportedModels([]string{"gemini-pro", "gemini-1.5-pro", "gemini-1.5-flash"}).
		SetDefaultTestModel("gemini-pro").
		Save(ctx)
	require.NoError(t, err)

	t.Run("empty associations", func(t *testing.T) {
		result, err := svc.QueryModelChannelConnections(ctx, []*objects.ModelAssociation{})
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("channel_model association", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: 1,
					ModelID:   "gpt-4",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[0].Models)
	})

	t.Run("channel_model association with non-existent model", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: 1,
					ModelID:   "non-existent-model",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("channel_regex association", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: 1,
					Pattern:   "^gpt-4.*",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"},
			{RequestModel: "gpt-4-turbo", ActualModel: "gpt-4-turbo", Source: "direct"},
		}, result[0].Models)
	})

	t.Run("regex association matches all channels", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "regex",
				Regex: &objects.RegexAssociation{
					Pattern: ".*pro$",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Only channel3 has models matching the pattern
		require.Equal(t, channel3.ID, result[0].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "gemini-pro", ActualModel: "gemini-pro", Source: "direct"},
			{RequestModel: "gemini-1.5-pro", ActualModel: "gemini-1.5-pro", Source: "direct"},
		}, result[0].Models)
	})

	t.Run("multiple associations preserves order", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4",
				},
			},
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: channel2.ID,
					Pattern:   "^claude-3-.*",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 2)

		// Verify order matches associations order
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[0].Models)

		require.Equal(t, channel2.ID, result[1].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "claude-3-opus", ActualModel: "claude-3-opus", Source: "direct"},
			{RequestModel: "claude-3-sonnet", ActualModel: "claude-3-sonnet", Source: "direct"},
			{RequestModel: "claude-3-haiku", ActualModel: "claude-3-haiku", Source: "direct"},
		}, result[1].Models)
	})

	t.Run("multiple associations reverse order", func(t *testing.T) {
		// Test with reversed order to verify order preservation
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: channel2.ID,
					Pattern:   "^claude-3-.*",
				},
			},
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 2)

		// Verify order matches associations order (channel2 first, then channel1)
		require.Equal(t, channel2.ID, result[0].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "claude-3-opus", ActualModel: "claude-3-opus", Source: "direct"},
			{RequestModel: "claude-3-sonnet", ActualModel: "claude-3-sonnet", Source: "direct"},
			{RequestModel: "claude-3-haiku", ActualModel: "claude-3-haiku", Source: "direct"},
		}, result[0].Models)

		require.Equal(t, channel1.ID, result[1].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[1].Models)
	})

	t.Run("invalid regex pattern", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: 1,
					Pattern:   "[invalid",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Empty(t, result)
	})

	t.Run("disabled channel is included", func(t *testing.T) {
		// Create a disabled channel
		disabledChannel, err := client.Channel.Create().
			SetType("openai").
			SetName("Disabled Channel").
			SetCredentials(objects.ChannelCredentials{APIKey: "test-key-disabled"}).
			SetSupportedModels([]string{"gpt-4-disabled"}).
			SetDefaultTestModel("gpt-4-disabled").
			SetStatus("disabled").
			Save(ctx)
		require.NoError(t, err)

		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: disabledChannel.ID,
					ModelID:   "gpt-4-disabled",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, disabledChannel.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4-disabled", ActualModel: "gpt-4-disabled", Source: "direct"}}, result[0].Models)
	})

	t.Run("regex matches models across multiple channels", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "regex",
				Regex: &objects.RegexAssociation{
					Pattern: ".*-3-.*",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)

		// Only channel2 (anthropic) has models matching the pattern
		require.Equal(t, channel2.ID, result[0].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "claude-3-opus", ActualModel: "claude-3-opus", Source: "direct"},
			{RequestModel: "claude-3-sonnet", ActualModel: "claude-3-sonnet", Source: "direct"},
			{RequestModel: "claude-3-haiku", ActualModel: "claude-3-haiku", Source: "direct"},
		}, result[0].Models)
	})

	t.Run("channel_regex with specific channel", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: 3,
					Pattern:   "gemini-1\\.5-.*",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, channel3.ID, result[0].Channel.ID)
		require.ElementsMatch(t, []ChannelModelEntry{
			{RequestModel: "gemini-1.5-pro", ActualModel: "gemini-1.5-pro", Source: "direct"},
			{RequestModel: "gemini-1.5-flash", ActualModel: "gemini-1.5-flash", Source: "direct"},
		}, result[0].Models)
	})

	t.Run("mixed associations with global deduplication", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4",
				},
			},
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: channel1.ID,
					Pattern:   "^gpt-4$",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		// Global deduplication: same (channel, model) only appears once
		require.Len(t, result, 1)
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[0].Models)
	})

	t.Run("duplicate channel associations preserve order", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4",
				},
			},
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel2.ID,
					ModelID:   "claude-3-opus",
				},
			},
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-3.5-turbo",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 3)

		// Channel order follows association order
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[0].Models)

		require.Equal(t, channel2.ID, result[1].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "claude-3-opus", ActualModel: "claude-3-opus", Source: "direct"}}, result[1].Models)

		require.Equal(t, channel1.ID, result[2].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-3.5-turbo", ActualModel: "gpt-3.5-turbo", Source: "direct"}}, result[2].Models)
	})

	t.Run("model associations produce separate connections in order", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-3.5-turbo",
				},
			},
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4-turbo",
				},
			},
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: channel1.ID,
					ModelID:   "gpt-4",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 3)
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		// Model connections follow association order
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-3.5-turbo", ActualModel: "gpt-3.5-turbo", Source: "direct"}}, result[0].Models)
		require.Equal(t, channel1.ID, result[1].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4-turbo", ActualModel: "gpt-4-turbo", Source: "direct"}}, result[1].Models)
		require.Equal(t, channel1.ID, result[2].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[2].Models)
	})

	t.Run("model association finds all channels supporting model", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gpt-4",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Len(t, result, 1)
		require.Equal(t, channel1.ID, result[0].Channel.ID)
		require.Equal(t, []ChannelModelEntry{{RequestModel: "gpt-4", ActualModel: "gpt-4", Source: "direct"}}, result[0].Models)
	})

	t.Run("model association with non-existent model", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "non-existent-model",
				},
			},
		}

		result, err := svc.QueryModelChannelConnections(ctx, associations)
		require.NoError(t, err)
		require.Empty(t, result)
	})
}

func TestModelService_ListEnabledModels(t *testing.T) {
	ctx, client, modelSvc, channelSvc, systemSvc := setupListEnabledModelsTest(t)

	primary, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Primary").
		SetBaseURL("https://api.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "key"}).
		SetSupportedModels([]string{"registered", "disabled", "archived", "unassociated", "channel-only", "provider/trimmed"}).
		SetDefaultTestModel("registered").
		SetTags([]string{"production", "team-a"}).
		SetSettings(&objects.ChannelSettings{
			ExtraModelPrefix:        "provider",
			AutoTrimedModelPrefixes: []string{"provider"},
			ModelMappings:           []objects.ModelMapping{{From: "channel-alias", To: "channel-only"}},
		}).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)
	secondary, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Secondary").
		SetBaseURL("https://api.secondary.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "key"}).
		SetSupportedModels([]string{"other"}).
		SetDefaultTestModel("other").
		SetTags([]string{"production"}).
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)
	reloadEnabledChannels(t, ctx, client, channelSvc)

	for _, id := range []string{"registered", "disabled", "archived", "other"} {
		createConfiguredModel(t, ctx, client, id, model.TypeChat, &objects.ModelAssociation{
			Type: "model", ModelID: &objects.ModelIDAssociation{ModelID: id},
		})
	}
	createConfiguredModel(t, ctx, client, "unassociated", model.TypeChat, nil)
	_, err = client.Model.Update().Where(model.ModelID("disabled")).SetStatus(model.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	_, err = client.Model.Update().Where(model.ModelID("archived")).SetStatus(model.StatusArchived).Save(ctx)
	require.NoError(t, err)

	// Old persisted settings cannot restore the removed channel-union path.
	err = systemSvc.setSystemValue(ctx, SystemKeyModelSettings,
		`{"query_all_channel_models":true,"fallback_to_channels_on_model_not_found":true,"model_blacklist_regex":".*"}`)
	require.NoError(t, err)

	for _, tc := range []struct {
		name           string
		profile        *objects.APIKeyProfile
		projectProfile *objects.ProjectProfile
		want           []string
	}{
		{name: "registered only", want: []string{"registered", "other"}},
		{name: "inclusion does not create models", profile: &objects.APIKeyProfile{
			ModelIDs: []string{"registered", "disabled", "archived", "unassociated", "channel-only", "channel-alias", "trimmed", "provider/registered", "missing"},
		}, want: []string{"registered"}},
		{name: "key channel IDs", profile: &objects.APIKeyProfile{ChannelIDs: []int{primary.ID}}, want: []string{"registered"}},
		{name: "key tags any", profile: &objects.APIKeyProfile{ChannelTags: []string{"missing", "team-a"}}, want: []string{"registered"}},
		{name: "key tags all", profile: &objects.APIKeyProfile{ChannelTags: []string{"production", "team-a"}, ChannelTagsMatchMode: objects.ChannelTagsMatchModeAll}, want: []string{"registered"}},
		{name: "key tags none", profile: &objects.APIKeyProfile{ChannelTags: []string{"team-a"}, ChannelTagsMatchMode: objects.ChannelTagsMatchModeNone}, want: []string{"other"}},
		{name: "key IDs intersect tags", profile: &objects.APIKeyProfile{ChannelIDs: []int{secondary.ID}, ChannelTags: []string{"team-a"}}},
		{name: "project IDs intersect tags", projectProfile: &objects.ProjectProfile{ChannelIDs: []int{primary.ID, secondary.ID}, ChannelTags: []string{"team-a"}}, want: []string{"registered"}},
		{name: "project tags all", projectProfile: &objects.ProjectProfile{ChannelTags: []string{"production", "team-a"}, ChannelTagsMatchMode: objects.ChannelTagsMatchModeAll}, want: []string{"registered"}},
		{name: "key cannot widen project", projectProfile: &objects.ProjectProfile{ChannelIDs: []int{primary.ID}}, profile: &objects.APIKeyProfile{ChannelIDs: []int{secondary.ID}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requestCtx := ctx
			if tc.profile != nil || tc.projectProfile != nil {
				key := &ent.APIKey{}
				if tc.profile != nil {
					tc.profile.Name = "active"
					key.Profiles = &objects.APIKeyProfiles{ActiveProfile: "active", Profiles: []objects.APIKeyProfile{*tc.profile}}
				}
				if tc.projectProfile != nil {
					tc.projectProfile.Name = "active"
					key.Edges.Project = &ent.Project{Profiles: &objects.ProjectProfiles{ActiveProfile: "active", Profiles: []objects.ProjectProfile{*tc.projectProfile}}}
				}
				requestCtx = contexts.WithAPIKey(ctx, key)
			}
			result, err := modelSvc.ListEnabledModels(requestCtx)
			require.NoError(t, err)
			ids := make([]string, 0, len(result))
			for _, m := range result {
				ids = append(ids, m.ID)
			}
			require.ElementsMatch(t, tc.want, ids)
		})
	}

	channelSvc.SetEnabledChannelsForTest(nil)
	require.Empty(t, listedModelIDs(t, ctx, modelSvc))
}

func TestModelService_ListEnabledModels_MappedTargetAdmission(t *testing.T) {
	ctx, client, modelSvc, channelSvc, _ := setupListEnabledModelsTest(t)
	_, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Mapped targets").
		SetBaseURL("https://api.example.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "key"}).
		SetSupportedModels([]string{"registered", "disabled", "archived", "channel-only", "unassociated"}).
		SetDefaultTestModel("registered").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)
	reloadEnabledChannels(t, ctx, client, channelSvc)
	for _, id := range []string{"registered", "disabled", "archived"} {
		createConfiguredModel(t, ctx, client, id, model.TypeChat, &objects.ModelAssociation{
			Type: "model", ModelID: &objects.ModelIDAssociation{ModelID: id},
		})
	}
	createConfiguredModel(t, ctx, client, "unassociated", model.TypeChat, nil)
	_, err = client.Model.Update().Where(model.ModelID("disabled")).SetStatus(model.StatusDisabled).Save(ctx)
	require.NoError(t, err)
	_, err = client.Model.Update().Where(model.ModelID("archived")).SetStatus(model.StatusArchived).Save(ctx)
	require.NoError(t, err)

	for _, tc := range []struct {
		name    string
		profile objects.APIKeyProfile
		want    map[string]string
	}{
		{name: "aliases require admitted targets", profile: objects.APIKeyProfile{ModelMappings: []objects.ModelMapping{
			{From: "valid-alias", To: "registered"},
			{From: "disabled-alias", To: "disabled"},
			{From: "archived-alias", To: "archived"},
			{From: "channel-alias", To: "channel-only"},
			{From: "missing-alias", To: "missing"},
			{From: "unassociated-alias", To: "unassociated"},
		}}, want: map[string]string{"registered": "registered", "valid-alias": "registered"}},
		{name: "inclusion uses request IDs", profile: objects.APIKeyProfile{ModelIDs: []string{"public-alias", "channel-only", "disabled", "archived"}, ModelMappings: []objects.ModelMapping{
			{From: "public-alias", To: "registered"},
		}}, want: map[string]string{"public-alias": "registered"}},
		{name: "regex inclusion resolves concrete aliases", profile: objects.APIKeyProfile{ModelIDs: []string{"client-v1", "client-v2"}, ModelMappings: []objects.ModelMapping{
			{From: "client-.*", To: "registered"},
		}}, want: map[string]string{"client-v1": "registered", "client-v2": "registered"}},
		{name: "invalid mapping overrides registered source", profile: objects.APIKeyProfile{ModelMappings: []objects.ModelMapping{
			{From: "registered", To: "channel-only"},
		}}},
		{name: "first matching rule wins", profile: objects.APIKeyProfile{ModelMappings: []objects.ModelMapping{
			{From: "alias-.*", To: "disabled"},
			{From: "alias-valid", To: "registered"},
		}}, want: map[string]string{"registered": "registered"}},
		{name: "mapping does not chain", profile: objects.APIKeyProfile{ModelMappings: []objects.ModelMapping{
			{From: "first", To: "second"},
			{From: "second", To: "registered"},
		}}, want: map[string]string{"registered": "registered", "second": "registered"}},
		{name: "unassociated source maps to admitted target", profile: objects.APIKeyProfile{ModelMappings: []objects.ModelMapping{
			{From: "unassociated", To: "registered"},
		}}, want: map[string]string{"registered": "registered", "unassociated": "registered"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.profile.Name = "active"
			key := &ent.APIKey{Profiles: &objects.APIKeyProfiles{ActiveProfile: "active", Profiles: []objects.APIKeyProfile{tc.profile}}}
			result, err := modelSvc.ListEnabledModels(contexts.WithAPIKey(ctx, key))
			require.NoError(t, err)
			resolved := make(map[string]string, len(result))
			for _, m := range result {
				require.NotNil(t, m.ConfiguredModel)
				require.NotContains(t, resolved, m.ID)
				resolved[m.ID] = m.ConfiguredModel.ModelID
			}
			if tc.want == nil {
				require.Empty(t, resolved)
			} else {
				require.Equal(t, tc.want, resolved)
			}
		})
	}
}

func TestFindUnassociatedChannels(t *testing.T) {
	// Create test channels
	channel1 := &ent.Channel{
		ID:              1,
		Type:            "openai",
		Name:            "OpenAI Channel",
		Status:          "enabled",
		SupportedModels: []string{"gpt-4", "gpt-3.5-turbo"},
	}

	channel2 := &ent.Channel{
		ID:              2,
		Type:            "anthropic",
		Name:            "Anthropic Channel",
		Status:          "enabled",
		SupportedModels: []string{"claude-3-opus", "claude-3-sonnet"},
	}

	channel3 := &ent.Channel{
		ID:              3,
		Type:            "gemini",
		Name:            "Gemini Channel",
		Status:          "disabled",
		SupportedModels: []string{"gemini-pro", "gemini-1.5-pro"},
	}

	channels := []*ent.Channel{channel1, channel2, channel3}

	t.Run("no associations - all channels unassociated", func(t *testing.T) {
		result := findUnassociatedChannels(channels, []*objects.ModelAssociation{})
		require.Len(t, result, 3)

		// Verify all channels have unassociated models
		for _, info := range result {
			require.NotEmpty(t, info.Models)
		}
	})

	t.Run("channel_model association", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_model",
				ChannelModel: &objects.ChannelModelAssociation{
					ChannelID: 1,
					ModelID:   "gpt-4",
				},
			},
		}

		result := findUnassociatedChannels(channels, associations)

		// Find channel1 in results
		var channel1Info *UnassociatedChannel

		for _, info := range result {
			if info.Channel.ID == 1 {
				channel1Info = info
				break
			}
		}

		require.NotNil(t, channel1Info)
		// gpt-4 should be associated, so only gpt-3.5-turbo should be unassociated
		require.Contains(t, channel1Info.Models, "gpt-3.5-turbo")
		require.NotContains(t, channel1Info.Models, "gpt-4")
	})

	t.Run("regex association", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "regex",
				Regex: &objects.RegexAssociation{
					Pattern: "^claude-3-.*",
				},
			},
		}

		result := findUnassociatedChannels(channels, associations)

		// Find channel2 in results
		var channel2Info *UnassociatedChannel

		for _, info := range result {
			if info.Channel.ID == 2 {
				channel2Info = info
				break
			}
		}

		// claude-3-opus and claude-3-sonnet should be associated by regex
		if channel2Info != nil {
			require.NotContains(t, channel2Info.Models, "claude-3-opus")
			require.NotContains(t, channel2Info.Models, "claude-3-sonnet")
		}
	})

	t.Run("model association with exclude", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gemini-pro",
					Exclude: []*objects.ExcludeAssociation{
						{
							ChannelIds: []int{3},
						},
					},
				},
			},
		}

		result := findUnassociatedChannels(channels, associations)

		// Find channel3 in results
		var channel3Info *UnassociatedChannel

		for _, info := range result {
			if info.Channel.ID == 3 {
				channel3Info = info
				break
			}
		}

		require.NotNil(t, channel3Info)
		// gemini-pro should be unassociated in channel3 due to exclude
		require.Contains(t, channel3Info.Models, "gemini-pro")
	})

	t.Run("channel_regex association", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "channel_regex",
				ChannelRegex: &objects.ChannelRegexAssociation{
					ChannelID: 1,
					Pattern:   "^gpt-.*",
				},
			},
		}

		result := findUnassociatedChannels(channels, associations)

		// Find channel1 in results
		var channel1Info *UnassociatedChannel

		for _, info := range result {
			if info.Channel.ID == 1 {
				channel1Info = info
				break
			}
		}

		// Both gpt-4 and gpt-3.5-turbo should be associated by regex
		if channel1Info != nil {
			require.NotContains(t, channel1Info.Models, "gpt-4")
			require.NotContains(t, channel1Info.Models, "gpt-3.5-turbo")
		}
	})

	t.Run("multiple associations", func(t *testing.T) {
		associations := []*objects.ModelAssociation{
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gpt-4",
				},
			},
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gpt-3.5-turbo",
				},
			},
			{
				Type: "regex",
				Regex: &objects.RegexAssociation{
					Pattern: "^claude-3-.*",
				},
			},
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gemini-pro",
				},
			},
			{
				Type: "model",
				ModelID: &objects.ModelIDAssociation{
					ModelID: "gemini-1.5-pro",
				},
			},
		}

		result := findUnassociatedChannels(channels, associations)

		// All models should be associated
		require.Empty(t, result)
	})

	t.Run("no channels", func(t *testing.T) {
		result := findUnassociatedChannels([]*ent.Channel{}, []*objects.ModelAssociation{})
		require.Empty(t, result)
	})
}
