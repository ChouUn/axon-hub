package gql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/channelmodelpriceversion"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/ent/requestexecution"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
)

func TestPluginUsageLogCostPriceMultiplier(t *testing.T) {
	fx := newPluginEndpointFixture(t, "plugin_usage_cost_multiplier")
	require.NoError(t, fx.manager.Enable(fx.setupCtx, fx.pages.ID, []string{"read_requests"}, nil))
	project := fx.db.Project.Query().OnlyX(fx.setupCtx)
	req := fx.db.Request.Create().SetProjectID(project.ID).SetModelID("gpt-4").
		SetRequestBody([]byte(`{}`)).SetStream(false).SetStatus(request.StatusCompleted).SaveX(fx.setupCtx)
	price := fx.db.ChannelModelPrice.Create().SetChannelID(fx.channelID).SetModelID("gpt-4").
		SetReferenceID("current-price").SetPrice(objects.ModelPrice{}).SaveX(fx.setupCtx)
	multiplier := decimal.RequireFromString("0.5")
	version := fx.db.ChannelModelPriceVersion.Create().SetChannelID(fx.channelID).SetChannelModelPriceID(price.ID).
		SetModelID("gpt-4").SetReferenceID("historical-price").SetEffectiveStartAt(time.Now()).
		SetStatus(channelmodelpriceversion.StatusArchived).
		SetPrice(objects.ModelPrice{Multiplier: &multiplier}).SaveX(fx.setupCtx)
	fx.db.UsageLog.Create().SetRequestID(req.ID).SetProjectID(project.ID).SetChannelID(fx.channelID).
		SetModelID("gpt-4").SetTotalCost(0.25).SetCostPriceReferenceID(version.ReferenceID).SaveX(fx.setupCtx)
	query := `{ usageLogs(first: 1) { edges { node { totalCost costPriceMultiplier } } } }`
	for _, endpoint := range []struct {
		name string
		path string
	}{
		{name: "admin", path: "/admin/graphql"},
		{name: "plugin", path: "/admin/plugins/" + pluginPagesID + "/graphql"},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			code, payload, raw := fx.post(endpoint.path, fx.ownerToken, query, nil)
			require.Equal(t, http.StatusOK, code, raw)
			require.NotContains(t, payload, "errors", raw)
			edges := payload["data"].(map[string]any)["usageLogs"].(map[string]any)["edges"].([]any)
			require.Len(t, edges, 1, raw)
			node := edges[0].(map[string]any)["node"].(map[string]any)
			require.Equal(t, float64(0.25), node["totalCost"], raw)
			require.Equal(t, "0.5", fmt.Sprint(node["costPriceMultiplier"]), raw)
		})
	}
	for _, tc := range []struct {
		name   string
		token  string
		grants []string
	}{
		{name: "user lacks request scope", token: fx.readerToken, grants: []string{"read_requests"}},
		{name: "plugin lacks request grant", token: fx.ownerToken},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, fx.manager.Enable(fx.setupCtx, fx.pages.ID, tc.grants, nil))
			code, payload, raw := fx.postPlugin(tc.token, pluginPagesID, query)
			require.Equal(t, http.StatusOK, code, raw)
			require.Contains(t, payload, "errors", raw)
			require.NotContains(t, raw, `"totalCost":0.25`, raw)
		})
	}
}

func TestPluginAnalyticsChannelTagQueries(t *testing.T) {
	fx := newPluginEndpointFixture(t, "plugin_analytics_channel_tags")
	installed, err := fx.manager.Install(fx.setupCtx, pluginEndpointZip(t, "acme.analytics-tags", []string{"read_dashboard"}, true), nil, nil)
	require.NoError(t, err)
	require.NoError(t, fx.manager.Enable(fx.setupCtx, installed.Plugin.ID, []string{"read_dashboard"}, nil))
	fx.db.Channel.UpdateOneID(fx.channelID).SetTags([]string{"visible"}).ExecX(fx.setupCtx)
	fx.db.Channel.Create().SetType(channel.TypeOpenai).SetName("excluded channel").
		SetCredentials(objects.ChannelCredentials{APIKey: "excluded"}).SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").SetTags([]string{"excluded"}).SaveX(fx.setupCtx)
	proj := fx.db.Project.Query().OnlyX(fx.setupCtx)
	fx.db.Project.UpdateOne(proj).SetProfiles(&objects.ProjectProfiles{
		ActiveProfile: "analytics",
		Profiles:      []objects.ProjectProfile{{Name: "analytics", ChannelIDs: []int{fx.channelID}}},
	}).ExecX(fx.setupCtx)
	req := fx.db.Request.Create().SetProjectID(proj.ID).SetModelID("gpt-4").
		SetRequestBody([]byte(`{}`)).SetStatus(request.StatusCompleted).SaveX(fx.setupCtx)
	fx.db.RequestExecution.Create().SetRequestID(req.ID).SetProjectID(proj.ID).SetChannelID(fx.channelID).
		SetModelID("gpt-4").SetRequestBody([]byte(`{}`)).SetStatus(requestexecution.StatusCompleted).SaveX(fx.setupCtx)
	post := func(token, query string) (map[string]any, string) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"query": query})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/admin/plugins/acme.analytics-tags/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Project-ID", fmt.Sprintf("gid://axonhub/Project/%d", proj.ID))
		recorder := httptest.NewRecorder()
		fx.engine.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload map[string]any
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payload))
		return payload, recorder.Body.String()
	}
	queries := []struct {
		name  string
		query string
	}{
		{name: "tags", query: `{ analyticsChannelTags }`},
		{name: "filtered models", query: `{ analyticsModelStats(filter: {channelTags: ["visible"]}) { id requestCount channels { name } } }`},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			payload, raw := post(fx.ownerToken, tc.query)
			require.NotContains(t, payload, "errors", raw)
			data := payload["data"].(map[string]any)
			if tc.name == "tags" {
				require.Equal(t, []any{"visible"}, data["analyticsChannelTags"], raw)
			} else {
				require.Equal(t, []any{map[string]any{
					"id": "gpt-4", "requestCount": float64(1),
					"channels": []any{map[string]any{"name": "seeded channel"}},
				}}, data["analyticsModelStats"], raw)
			}
		})
		t.Run(tc.name+" without user scope", func(t *testing.T) {
			payload, raw := post(fx.readerToken, tc.query)
			require.Contains(t, payload, "errors", raw)
		})
	}
	require.NoError(t, fx.manager.Enable(fx.setupCtx, installed.Plugin.ID, nil, nil))
	for _, tc := range queries {
		t.Run(tc.name+" without plugin grant", func(t *testing.T) {
			payload, raw := post(fx.ownerToken, tc.query)
			require.Contains(t, payload, "errors", raw)
		})
	}
}

func TestPluginChannelHealthGateQueries(t *testing.T) {
	fx := newPluginEndpointFixture(t, "plugin_channel_health_gate")
	require.NoError(t, fx.manager.Enable(fx.setupCtx, fx.pages.ID, []string{"read_channels"}, nil))
	policy := biz.DefaultHealthGatePolicy()
	policy.FailureThreshold = 1
	policy.OpenDurationSeconds = 600
	writer := biz.NewSystemService(biz.SystemServiceParams{Ent: fx.db, CacheConfig: xcache.Config{Mode: xcache.ModeMemory}})
	require.NoError(t, writer.SetRetryPolicy(fx.setupCtx, &biz.RetryPolicy{HealthGate: &policy}))
	ch := fx.db.Channel.GetX(fx.setupCtx, fx.channelID)
	cfg := biz.ResolveHealthGateConfig(policy, &biz.Channel{Channel: ch})
	gate := fx.channelService.HealthGate()
	ticket, ok := healthGateBegin(t, gate, biz.HealthGateKey{ChannelID: ch.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg), false, false)
	require.True(t, ok)
	healthGateFinish(t, gate, ticket, healthGateResolver(cfg), biz.HealthGateOutcomeFailure, biz.HealthGateErrorInfo{StatusCode: 503})
	fx.db.Channel.Create().SetType(channel.TypeOpenai).SetName("healthy channel").
		SetCredentials(objects.ChannelCredentials{APIKey: "healthy"}).SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").SaveX(fx.setupCtx)
	statusQuery := fmt.Sprintf(`{ node(id: "gid://axonhub/Channel/%d") { ... on Channel {
		name healthGate { failureThreshold openCount models { actualModel state } }
	} } }`, ch.ID)
	t.Run("status", func(t *testing.T) {
		code, payload, raw := fx.postPlugin(fx.ownerToken, pluginPagesID, statusQuery)
		require.Equal(t, http.StatusOK, code, raw)
		require.NotContains(t, payload, "errors", raw)
		node := payload["data"].(map[string]any)["node"].(map[string]any)
		health := node["healthGate"].(map[string]any)
		require.Equal(t, float64(1), health["failureThreshold"], raw)
		require.Equal(t, float64(1), health["openCount"], raw)
		require.Equal(t, []any{map[string]any{"actualModel": "gpt-4", "state": "open"}}, health["models"], raw)
		zero := 0
		fx.db.Channel.UpdateOneID(ch.ID).SetSettings(&objects.ChannelSettings{HealthGateFailureThreshold: &zero}).ExecX(fx.setupCtx)
		code, payload, raw = fx.postPlugin(fx.ownerToken, pluginPagesID, statusQuery)
		require.Equal(t, http.StatusOK, code, raw)
		require.NotContains(t, payload, "errors", raw)
		health = payload["data"].(map[string]any)["node"].(map[string]any)["healthGate"].(map[string]any)
		require.Equal(t, float64(0), health["failureThreshold"], raw)
		require.Equal(t, float64(0), health["openCount"], raw)
		require.Empty(t, health["models"], raw)
	})
	t.Run("abnormal filter", func(t *testing.T) {
		fx.db.Channel.UpdateOneID(ch.ID).SetSettings(&objects.ChannelSettings{}).ExecX(fx.setupCtx)
		gate.Reset(ch.ID, "")
		ticket, ok := healthGateBegin(t, gate, biz.HealthGateKey{ChannelID: ch.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg), false, false)
		require.True(t, ok)
		healthGateFinish(t, gate, ticket, healthGateResolver(cfg), biz.HealthGateOutcomeFailure, biz.HealthGateErrorInfo{StatusCode: 503})
		code, payload, raw := fx.postPlugin(fx.ownerToken, pluginPagesID, `{ queryChannels(input: {healthGateAbnormal: true}) { edges { node { name } } } }`)
		require.Equal(t, http.StatusOK, code, raw)
		require.NotContains(t, payload, "errors", raw)
		edges := payload["data"].(map[string]any)["queryChannels"].(map[string]any)["edges"].([]any)
		require.Equal(t, []any{map[string]any{"node": map[string]any{"name": ch.Name}}}, edges, raw)
		zero := 0
		fx.db.Channel.UpdateOneID(ch.ID).SetSettings(&objects.ChannelSettings{HealthGateFailureThreshold: &zero}).ExecX(fx.setupCtx)
		code, payload, raw = fx.postPlugin(fx.ownerToken, pluginPagesID, `{ queryChannels(input: {healthGateAbnormal: true}) { edges { node { name } } } }`)
		require.Equal(t, http.StatusOK, code, raw)
		require.NotContains(t, payload, "errors", raw)
		edges = payload["data"].(map[string]any)["queryChannels"].(map[string]any)["edges"].([]any)
		require.Empty(t, edges, "disabled gate must stop matching the abnormal filter: %s", raw)
	})
	t.Run("plugin lacks channel grant", func(t *testing.T) {
		require.NoError(t, fx.manager.Enable(fx.setupCtx, fx.pages.ID, nil, nil))
		code, payload, raw := fx.postPlugin(fx.ownerToken, pluginPagesID, statusQuery)
		require.Equal(t, http.StatusOK, code, raw)
		require.Contains(t, payload, "errors", raw)
		require.NotContains(t, raw, `"actualModel":"gpt-4"`, raw)
	})
}
