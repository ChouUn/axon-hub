package orchestrator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
)

// TestTestChannelOutlivesRequestDeadline reproduces a channel test whose
// upstream answers later than the admin request deadline: the test must be
// bounded by ChannelTestTimeout, not by the caller's deadline.
func TestTestChannelOutlivesRequestDeadline(t *testing.T) {
	ctx, client := setupTest(t)
	project := createTestProject(t, ctx, client)
	ctx = contexts.WithProjectID(ctx, project.ID)
	ctx = contexts.WithSource(ctx, request.SourceTest)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buildMockOpenAIResponse("chatcmpl-slow", "gpt-4", "pong", 5, 1))
	}))
	t.Cleanup(upstream.Close)

	slowChannel, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Slow channel").
		SetBaseURL(upstream.URL + "/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gpt-4"}).
		SetDefaultTestModel("gpt-4").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	channelService, requestService, systemService, usageLogService := setupTestServices(t, client)
	promptProtection := biz.NewPromptProtectionRuleService(biz.PromptProtectionRuleServiceParams{
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
		Ent:         client,
	})
	t.Cleanup(promptProtection.Stop)
	processor := NewTestChannelOrchestrator(
		channelService,
		requestService,
		systemService,
		usageLogService,
		promptProtection,
		httpclient.NewHttpClient(),
		ChannelTestTimeout(5*time.Second),
	)

	requestCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	result, err := processor.TestChannel(requestCtx, objects.GUID{ID: slowChannel.ID}, nil, nil)
	require.NoError(t, err)
	require.Truef(t, result.Success, "channel test failed: %s", lo.FromPtr(result.Error))
}
