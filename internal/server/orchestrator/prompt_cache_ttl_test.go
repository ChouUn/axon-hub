package orchestrator

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func oneHourCacheOutbound(enabled bool) *PersistentOutboundTransformer {
	return newTestOutbound(&biz.Channel{Channel: &ent.Channel{
		ID: 7, Name: "cache-channel",
		Settings: &objects.ChannelSettings{ForceOneHourPromptCache: enabled},
	}})
}

func TestPromptCacheOneHourRewritesExistingBreakpointsOnly(t *testing.T) {
	input := `{"model":"claude","cache_control":{"type":"ephemeral"},"tools":[{"name":"search","cache_control":{"type":"ephemeral","ttl":"5m","extra":12}},{"cache_control":{"type":"permanent","ttl":"5m"}}],"system":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","cache_control":{"type":"ephemeral","ttl":"5m"}}],"messages":[{"role":"user","content":[{"type":"text","text":"preserve spacing","cache_control":{"type":"ephemeral"}},{"type":"tool_result","content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"5m"}},{"type":"tool_result","content":[{"type":"text","cache_control":{"type":"ephemeral"}}]}]}]},{"role":"assistant","content":"unmodified"}],"other":{"cache_control":{"type":"ephemeral","ttl":"5m"}}}`
	want := `{"model":"claude","cache_control":{"type":"ephemeral","ttl":"1h"},"tools":[{"name":"search","cache_control":{"type":"ephemeral","ttl":"1h","extra":12}},{"cache_control":{"type":"permanent","ttl":"5m"}}],"system":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}}],"messages":[{"role":"user","content":[{"type":"text","text":"preserve spacing","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"tool_result","content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"tool_result","content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]},{"role":"assistant","content":"unmodified"}],"other":{"cache_control":{"type":"ephemeral","ttl":"5m"}}}`
	request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(input)}

	result, err := applyForceOneHourPromptCache(oneHourCacheOutbound(true)).OnOutboundRawRequest(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, want, string(result.Body))
	require.Empty(t, result.Headers)
}

func TestPromptCacheOneHourLeavesUnmatchedRequestsUntouched(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		format  string
		body    string
	}{
		{"disabled", false, string(llm.APIFormatAnthropicMessage), `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`},
		{"other format", true, string(llm.APIFormatOpenAIChatCompletion), `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`},
		{"no breakpoints", true, string(llm.APIFormatAnthropicMessage), ` { "system": "prompt", "messages": [{"content":[{"text":"hello"}]}] } `},
		{"string system", true, string(llm.APIFormatAnthropicMessage), `{"system":"hello","messages":[{"content":"hello"}]}`},
		{"invalid JSON", true, string(llm.APIFormatAnthropicMessage), `{"cache_control":`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			request := &httpclient.Request{APIFormat: tc.format, Body: body}
			result, err := applyForceOneHourPromptCache(oneHourCacheOutbound(tc.enabled)).OnOutboundRawRequest(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(result.Body))
			require.True(t, bytes.Equal(body, result.Body))
		})
	}
	for _, channel := range []*biz.Channel{nil, {Channel: &ent.Channel{Settings: nil}}} {
		request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(`{"cache_control":{"type":"ephemeral"}}`)}
		result, err := applyForceOneHourPromptCache(newTestOutbound(channel)).OnOutboundRawRequest(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, `{"cache_control":{"type":"ephemeral"}}`, string(result.Body))
	}
}

func TestPromptCacheOneHourDoesNotMutateSharedRetryBody(t *testing.T) {
	original := []byte(`{"cache_control":{"type":"ephemeral","ttl":"5m"}}`)
	shared := make([]byte, len(original), len(original)+256)
	copy(shared, original)
	first := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: shared}
	second := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: shared}

	result, err := applyForceOneHourPromptCache(oneHourCacheOutbound(true)).OnOutboundRawRequest(t.Context(), first)
	require.NoError(t, err)
	require.Contains(t, string(result.Body), `"ttl":"1h"`)
	require.Equal(t, original, shared)
	require.Equal(t, original, second.Body)

	retryResult, err := applyForceOneHourPromptCache(oneHourCacheOutbound(false)).OnOutboundRawRequest(t.Context(), second)
	require.NoError(t, err)
	require.Equal(t, original, retryResult.Body)
}
