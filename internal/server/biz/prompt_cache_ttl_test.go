package biz

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	entrequest "github.com/looplj/axonhub/internal/ent/request"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestWalkAnthropicCacheControls(t *testing.T) {
	t.Parallel()
	body := []byte(`{"cache_control":{"type":"ephemeral"},"tools":[{"cache_control":{"type":"ephemeral"}}],"system":[{"cache_control":{"type":"ephemeral"}}],"messages":[{"content":[{"cache_control":{"type":"ephemeral"},"content":[{"cache_control":{"type":"ephemeral"}}]}]}],"other":{"cache_control":{"type":"ephemeral"}}}`)
	var paths []string
	WalkAnthropicCacheControls(body, func(path string, control gjson.Result) {
		require.Equal(t, "ephemeral", control.Get("type").String())
		paths = append(paths, path)
	})
	require.Equal(t, []string{
		"cache_control", "tools.0.cache_control", "system.0.cache_control",
		"messages.0.content.0.cache_control", "messages.0.content.0.content.0.cache_control",
	}, paths)
}

func TestHasOneHourPromptCache(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		body   string
		format llm.APIFormat
		want   bool
	}{
		{"top-level", `{"cache_control":{"type":"ephemeral","ttl":"1h"}}`, llm.APIFormatAnthropicMessage, true},
		{"tool", `{"tools":[{}, {"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`, llm.APIFormatAnthropicMessage, true},
		{"system", `{"system":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`, llm.APIFormatAnthropicMessage, true},
		{"message content", `{"messages":[{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`, llm.APIFormatAnthropicMessage, true},
		{"nested tool result content", `{"messages":[{"content":[{"type":"tool_result","content":[{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}]}`, llm.APIFormatAnthropicMessage, true},
		{"only five minutes", `{"cache_control":{"type":"ephemeral","ttl":"5m"},"tools":[{"cache_control":{"type":"ephemeral"}}]}`, llm.APIFormatAnthropicMessage, false},
		{"no breakpoints", `{"messages":[{"content":[{"type":"text","text":"hello"}]}]}`, llm.APIFormatAnthropicMessage, false},
		{"wrong type", `{"cache_control":{"type":"persistent","ttl":"1h"}}`, llm.APIFormatAnthropicMessage, false},
		{"wrong format", `{"cache_control":{"type":"ephemeral","ttl":"1h"}}`, llm.APIFormatOpenAIChatCompletion, false},
		{"mixed TTLs", `{"tools":[{"cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`, llm.APIFormatAnthropicMessage, true},
		{"unrelated object", `{"other":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}`, llm.APIFormatAnthropicMessage, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, hasOneHourPromptCache(httpclient.Request{Body: []byte(tc.body)}, tc.format))
		})
	}
}

func TestRequestService_CreateRequestExecutionPersistsOneHourPromptCache(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:request_one_hour_prompt_cache?mode=memory&_fk=0")
	t.Cleanup(func() { client.Close() })

	ctx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	systemService := NewSystemService(SystemServiceParams{Ent: client})
	channelService := NewChannelServiceForTest(client)
	usageLogService := NewUsageLogService(client, systemService, channelService)
	dataStorageService := NewDataStorageService(DataStorageServiceParams{
		SystemService: systemService,
		CacheConfig:   xcache.Config{Mode: xcache.ModeMemory},
		Client:        client,
	})
	requestService := NewRequestService(client, systemService.CacheConfig, systemService,
		usageLogService, dataStorageService, NewLiveStreamRegistry())

	channelEntity, err := client.Channel.Create().
		SetName("prompt-cache-channel").
		SetType(channel.TypeOpencodeGo).
		SetBaseURL("https://example.com").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"claude"}).
		SetDefaultTestModel("claude").
		SetStatus(channel.StatusEnabled).
		Save(ctx)
	require.NoError(t, err)

	requestEntity, err := client.Request.Create().
		SetModelID("claude").
		SetFormat(string(llm.APIFormatAnthropicMessage)).
		SetRequestBody([]byte(`{"model":"claude"}`)).
		SetStatus(entrequest.StatusProcessing).
		SetStream(false).
		Save(ctx)
	require.NoError(t, err)

	cases := []struct {
		name   string
		body   string
		format llm.APIFormat
		want   bool
	}{
		{"anthropic client breakpoint", `{"model":"claude","cache_control":{"type":"ephemeral","ttl":"1h"}}`, llm.APIFormatAnthropicMessage, true},
		{"anthropic short breakpoint", `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`, llm.APIFormatAnthropicMessage, false},
		{"other outbound protocol", `{"cache_control":{"type":"ephemeral","ttl":"1h"}}`, llm.APIFormatOpenAIChatCompletion, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			execution, err := requestService.CreateRequestExecution(ctx, &Channel{Channel: channelEntity},
				"claude", requestEntity, httpclient.Request{Body: []byte(tc.body)}, tc.format, false)
			require.NoError(t, err)
			stored, err := client.RequestExecution.Get(ctx, execution.ID)
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.OneHourPromptCache)
		})
	}
}
