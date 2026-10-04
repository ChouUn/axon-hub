package orchestrator

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

func TestSessionOwnerProtectionTTLCacheMarkers(t *testing.T) {
	locations := []struct {
		name string
		body string
	}{
		{"automatic cache", `{"cache_control":%s}`},
		{"tool definition", `{"tools":[{"name":"lookup","input_schema":{"type":"object"},"cache_control":%s}]}`},
		{"later tool definition", `{"tools":[{"name":"first"},{"name":"lookup","cache_control":%s}]}`},
		{"native tool definition", `{"tools":[{"type":"web_search_20250305","name":"web_search","cache_control":%s}]}`},
		{"system block", `{"system":[{"type":"text","text":"prompt","cache_control":%s}]}`},
		{"later system block", `{"system":[{"type":"text","text":"first"},{"type":"text","text":"prompt","cache_control":%s}]}`},
		{"user content block", `{"messages":[{"role":"user","content":[{"type":"text","text":"hello","cache_control":%s}]}]}`},
		{"assistant tool use block", `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"lookup","input":{},"cache_control":%s}]}]}`},
		{"web fetch result", `{"messages":[{"role":"user","content":[{"type":"web_fetch_tool_result","tool_use_id":"srvtoolu_123","content":{"type":"web_fetch_tool_result_error","error_code":"invalid_input"},"cache_control":%s}]}]}`},
		{"container upload", `{"messages":[{"role":"user","content":[{"type":"container_upload","file_id":"file_123","cache_control":%s}]}]}`},
		{"image content block", `{"messages":[{"content":[{"type":"image","cache_control":%s}]}]}`},
		{"document content block", `{"messages":[{"content":[{"type":"document","cache_control":%s}]}]}`},
		{"thinking content block", `{"messages":[{"content":[{"type":"thinking","cache_control":%s}]}]}`},
		{"redacted thinking content block", `{"messages":[{"content":[{"type":"redacted_thinking","cache_control":%s}]}]}`},
		{"server tool use block", `{"messages":[{"content":[{"type":"server_tool_use","cache_control":%s}]}]}`},
		{"native tool result block", `{"messages":[{"content":[{"type":"web_search_tool_result","cache_control":%s}]}]}`},
		{"later message and content block", `{"messages":[{"role":"user","content":"first"},{"role":"assistant","content":[{"type":"text","text":"first"},{"type":"text","text":"last","cache_control":%s}]}]}`},
		{"tool result block", `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":"result","cache_control":%s}]}]}`},
		{"tool result nested text", `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"text","text":"result","cache_control":%s}]}]}]}`},
		{"tool result nested image", `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"image","source":{"type":"url","url":"https://example.com/image.png"},"cache_control":%s}]}]}]}`},
		{"recursively nested tool result", `{"messages":[{"role":"user","content":[{"type":"tool_result","content":[{"type":"tool_result","content":[{"type":"text","text":"result","cache_control":%s}]}]}]}]}`},
	}
	markers := []struct {
		name    string
		control string
		want    time.Duration
	}{
		{"default TTL", `{"type":"ephemeral"}`, 5 * time.Minute},
		{"five minute TTL", `{"type":"ephemeral","ttl":"5m"}`, 5 * time.Minute},
		{"one hour TTL", `{"type":"ephemeral","ttl":"1h"}`, time.Hour},
		{"extra control fields", `{"type":"ephemeral","ttl":"1h","extra":true}`, time.Hour},
		{"missing type", `{"ttl":"1h"}`, 5 * time.Minute},
		{"wrong type", `{"type":"permanent","ttl":"1h"}`, 5 * time.Minute},
		{"wrong type case", `{"type":"Ephemeral","ttl":"1h"}`, 5 * time.Minute},
		{"null type", `{"type":null,"ttl":"1h"}`, 5 * time.Minute},
		{"numeric type", `{"type":1,"ttl":"1h"}`, 5 * time.Minute},
		{"boolean type", `{"type":true,"ttl":"1h"}`, 5 * time.Minute},
		{"object type", `{"type":{"value":"ephemeral"},"ttl":"1h"}`, 5 * time.Minute},
		{"array type", `{"type":["ephemeral"],"ttl":"1h"}`, 5 * time.Minute},
		{"empty TTL", `{"type":"ephemeral","ttl":""}`, 5 * time.Minute},
		{"null TTL", `{"type":"ephemeral","ttl":null}`, 5 * time.Minute},
		{"numeric TTL", `{"type":"ephemeral","ttl":3600}`, 5 * time.Minute},
		{"boolean TTL", `{"type":"ephemeral","ttl":true}`, 5 * time.Minute},
		{"array TTL", `{"type":"ephemeral","ttl":["1h"]}`, 5 * time.Minute},
		{"object TTL", `{"type":"ephemeral","ttl":{"value":"1h"}}`, 5 * time.Minute},
		{"unsupported long TTL", `{"type":"ephemeral","ttl":"24h"}`, 5 * time.Minute},
		{"unsupported minute TTL", `{"type":"ephemeral","ttl":"60m"}`, 5 * time.Minute},
		{"wrong TTL case", `{"type":"ephemeral","ttl":"1H"}`, 5 * time.Minute},
		{"TTL whitespace", `{"type":"ephemeral","ttl":" 1h "}`, 5 * time.Minute},
		{"negative TTL", `{"type":"ephemeral","ttl":"-1h"}`, 5 * time.Minute},
		{"null control", `null`, 5 * time.Minute},
		{"array control", `[{"type":"ephemeral","ttl":"1h"}]`, 5 * time.Minute},
		{"string control", `"ephemeral"`, 5 * time.Minute},
		{"numeric control", `3600`, 5 * time.Minute},
		{"boolean control", `true`, 5 * time.Minute},
		{"empty control", `{}`, 5 * time.Minute},
	}
	for _, location := range locations {
		t.Run(location.name, func(t *testing.T) {
			for _, marker := range markers {
				t.Run(marker.name, func(t *testing.T) {
					body := fmt.Sprintf(location.body, marker.control)
					request := &httpclient.Request{
						APIFormat: string(llm.APIFormatAnthropicMessage),
						Body:      []byte(body),
					}
					require.Equal(t, marker.want, sessionOwnerProtectionTTL(request))
					require.Equal(t, body, string(request.Body), "TTL inspection must not mutate the body")
				})
			}
		})
	}
}

func TestSessionOwnerProtectionTTLMaximum(t *testing.T) {
	cases := []struct {
		name string
		body string
		want time.Duration
	}{
		{
			name: "hour before short markers",
			body: `{"cache_control":{"type":"ephemeral","ttl":"1h"},"tools":[{"name":"lookup","cache_control":{"type":"ephemeral","ttl":"5m"}}],"system":[{"type":"text","cache_control":{"type":"ephemeral"}}],"messages":[{"content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]}`,
			want: time.Hour,
		},
		{
			name: "hour after short markers",
			body: `{"cache_control":{"type":"ephemeral"},"tools":[{"name":"lookup","cache_control":{"type":"ephemeral","ttl":"5m"}}],"system":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`,
			want: time.Hour,
		},
		{
			name: "nested hour before later short content",
			body: `{"messages":[{"content":[{"type":"tool_result","content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}}]},{"type":"text","cache_control":{"type":"ephemeral"}}]},{"content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"5m"}}]}]}`,
			want: time.Hour,
		},
		{
			name: "multiple short markers do not add up",
			body: `{"cache_control":{"type":"ephemeral"},"tools":[{"name":"lookup","cache_control":{"type":"ephemeral","ttl":"5m"}}],"system":[{"type":"text","cache_control":{"type":"ephemeral"}}]}`,
			want: 5 * time.Minute,
		},
		{
			name: "invalid markers cannot exceed the hour limit",
			body: `{"cache_control":{"type":"ephemeral","ttl":"24h"},"tools":[{"name":"lookup","cache_control":{"type":"ephemeral","ttl":"1h"}}],"system":[{"type":"text","cache_control":{"type":"permanent","ttl":"999h"}}]}`,
			want: time.Hour,
		},
		{
			name: "invalid marker does not hide a valid sibling",
			body: `{"cache_control":{"type":"ephemeral","ttl":true},"messages":[{"content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","cache_control":[] }]}]}`,
			want: time.Hour,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(tc.body)}
			require.Equal(t, tc.want, sessionOwnerProtectionTTL(request))
		})
	}
}

func TestSessionOwnerProtectionTTLIgnoresNonProtocolCacheControl(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"system text", `{"system":"cache_control: {type: ephemeral, ttl: 1h}"}`},
		{"message text", `{"messages":[{"content":"{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}"}]}`},
		{"content block text", `{"messages":[{"content":[{"type":"text","text":"{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}"}]}]}`},
		{"metadata", `{"metadata":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}`},
		{"arbitrary business JSON", `{"payload":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}`},
		{"tool schema", `{"tools":[{"name":"lookup","input_schema":{"cache_control":{"type":"ephemeral","ttl":"1h"},"properties":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}}]}`},
		{"tool input", `{"messages":[{"content":[{"type":"tool_use","input":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}]}]}`},
		{"tool input content array", `{"messages":[{"content":[{"type":"tool_use","input":{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}}]}]}`},
		{"non tool result content array", `{"messages":[{"content":[{"type":"tool_use","content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}`},
		{"unknown block content array", `{"messages":[{"content":[{"type":"business","content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}`},
		{"system nested business content", `{"system":[{"type":"text","content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`},
		{"tool nested business content", `{"tools":[{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`},
		{"message level marker", `{"messages":[{"cache_control":{"type":"ephemeral","ttl":"1h"},"content":"hello"}]}`},
		{"tool result JSON object", `{"messages":[{"content":[{"type":"tool_result","content":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}]}]}`},
		{"tool result text", `{"messages":[{"content":[{"type":"tool_result","content":"{\"cache_control\":{\"type\":\"ephemeral\",\"ttl\":\"1h\"}}"}]}]}`},
		{"nested tool result business payload", `{"messages":[{"content":[{"type":"tool_result","content":[{"type":"text","payload":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}]}]}]}`},
		{"message content without type", `{"messages":[{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`},
		{"message content unknown type", `{"messages":[{"content":[{"type":"business","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`},
		{"message content non string type", `{"messages":[{"content":[{"type":["text"],"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`},
		{"nested result without type", `{"messages":[{"content":[{"type":"tool_result","content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}`},
		{"nested result unknown type", `{"messages":[{"content":[{"type":"tool_result","content":[{"type":"business","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}`},
		{"nested result non string type", `{"messages":[{"content":[{"type":"tool_result","content":[{"type":false,"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}]}`},
		{"system block without type", `{"system":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
		{"system block unknown type", `{"system":[{"type":"business","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
		{"system block non text type", `{"system":[{"type":"image","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
		{"tool definition without name", `{"tools":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
		{"tool definition non string name", `{"tools":[{"name":true,"cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
		{"tool definition empty name", `{"tools":[{"name":"","cache_control":{"type":"ephemeral","ttl":"1h"}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(tc.body)}
			require.Equal(t, 5*time.Minute, sessionOwnerProtectionTTL(request))
		})
	}
}

func TestSessionOwnerProtectionTTLInvalidRequest(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		require.Equal(t, 5*time.Minute, sessionOwnerProtectionTTL(nil))
	})
	for _, format := range []string{
		"", "anthropic", "anthropic/messages/other", "Anthropic/messages",
		string(llm.APIFormatOpenAIChatCompletion), string(llm.APIFormatOpenAIResponse),
	} {
		t.Run("format "+format, func(t *testing.T) {
			request := &httpclient.Request{
				APIFormat: format,
				Body:      []byte(`{"cache_control":{"type":"ephemeral","ttl":"1h"}}`),
			}
			require.Equal(t, 5*time.Minute, sessionOwnerProtectionTTL(request))
		})
	}
	cases := []struct {
		name string
		body string
	}{
		{"empty body", ""},
		{"empty object", `{}`},
		{"invalid JSON", `{"cache_control":`},
		{"valid marker followed by invalid JSON", `{"cache_control":{"type":"ephemeral","ttl":"1h"},`},
		{"trailing garbage", `{"cache_control":{"type":"ephemeral","ttl":"1h"}} garbage`},
		{"root null", `null`},
		{"root array", `[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]`},
		{"tools object", `{"tools":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}`},
		{"system object", `{"system":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}`},
		{"messages object", `{"messages":{"content":[{"cache_control":{"type":"ephemeral","ttl":"1h"}}]}}`},
		{"message content object", `{"messages":[{"content":{"cache_control":{"type":"ephemeral","ttl":"1h"}}}]}`},
		{"non object entries", `{"tools":[null,1,"cache_control",[]],"system":[false],"messages":[null,1,{"content":[null,1,"cache_control",[]]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(tc.body)}
			require.Equal(t, 5*time.Minute, sessionOwnerProtectionTTL(request))
		})
	}
}

func TestSessionOwnerProtectionTTLUsesFinalOverrideBody(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		forceHour bool
		op        objects.OverrideOperation
		before    time.Duration
		want      time.Duration
	}{
		{
			name:   "override adds hour marker",
			body:   `{}`,
			op:     objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "cache_control", Value: `{"type":"ephemeral","ttl":"1h"}`},
			before: 5 * time.Minute,
			want:   time.Hour,
		},
		{
			name:   "override extends short marker",
			body:   `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			op:     objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "cache_control.ttl", Value: "1h"},
			before: 5 * time.Minute,
			want:   time.Hour,
		},
		{
			name:      "override shortens forced hour marker",
			body:      `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			forceHour: true,
			op:        objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "cache_control.ttl", Value: "5m"},
			before:    time.Hour,
			want:      5 * time.Minute,
		},
		{
			name:      "override deletes forced hour marker",
			body:      `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			forceHour: true,
			op:        objects.OverrideOperation{Op: objects.OverrideOpDelete, Path: "cache_control"},
			before:    time.Hour,
			want:      5 * time.Minute,
		},
		{
			name:      "override invalidates forced TTL",
			body:      `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			forceHour: true,
			op:        objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "cache_control.ttl", Value: "24h"},
			before:    time.Hour,
			want:      5 * time.Minute,
		},
		{
			name:      "override invalidates cache type",
			body:      `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			forceHour: true,
			op:        objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "cache_control.type", Value: "permanent"},
			before:    time.Hour,
			want:      5 * time.Minute,
		},
		{
			name:      "override restores default TTL",
			body:      `{"cache_control":{"type":"ephemeral","ttl":"5m"}}`,
			forceHour: true,
			op:        objects.OverrideOperation{Op: objects.OverrideOpDelete, Path: "cache_control.ttl"},
			before:    time.Hour,
			want:      5 * time.Minute,
		},
		{
			name:   "override adds nested tool result marker",
			body:   `{"messages":[{"content":[{"type":"tool_result","content":[{"type":"text","text":"result"}]}]}]}`,
			op:     objects.OverrideOperation{Op: objects.OverrideOpSet, Path: "messages.0.content.0.content.0.cache_control", Value: `{"type":"ephemeral","ttl":"1h"}`},
			before: 5 * time.Minute,
			want:   time.Hour,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			outbound := newTestOutbound(&biz.Channel{Channel: &ent.Channel{
				ID: 7, Name: "owner-ttl-channel",
				Settings: &objects.ChannelSettings{
					ForceOneHourPromptCache: tc.forceHour,
					BodyOverrideOperations:  []objects.OverrideOperation{tc.op},
				},
			}})
			// The inbound body and logging JSONBody deliberately disagree with the
			// final body. Protection must follow the bytes actually sent upstream.
			inbound := &httpclient.Request{Body: []byte(`{"cache_control":{"type":"ephemeral","ttl":"1h"}}`)}
			outbound.state.LlmRequest = &llm.Request{Model: "claude", RawRequest: inbound}
			request := &httpclient.Request{
				APIFormat:   string(llm.APIFormatAnthropicMessage),
				ContentType: "application/json",
				Body:        []byte(tc.body),
				JSONBody:    append([]byte(nil), inbound.Body...),
			}
			outbound.state.RawProviderRequest = request

			forced, err := applyForceOneHourPromptCache(outbound).OnOutboundRawRequest(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, tc.before, sessionOwnerProtectionTTL(forced))

			final, err := applyOverrideRequestBody(outbound).OnOutboundRawRequest(t.Context(), forced)
			require.NoError(t, err)
			require.Same(t, final, outbound.state.RawProviderRequest)
			require.Equal(t, tc.want, sessionOwnerProtectionTTL(outbound.state.RawProviderRequest))
			require.Equal(t, inbound.Body, final.JSONBody)
		})
	}
}

func TestSessionOwnerProtectionTTLSupportedContentTypes(t *testing.T) {
	for _, kind := range []string{
		"text", "image", "document", "thinking", "redacted_thinking",
		"tool_use", "tool_result", "server_tool_use", "mcp_tool_use",
		"web_search_tool_result", "code_execution_tool_result", "mcp_tool_result",
		"bash_code_execution_tool_result", "text_editor_code_execution_tool_result",
		"tool_search_tool_result", "web_search_result", "search_result",
	} {
		t.Run(kind, func(t *testing.T) {
			body := fmt.Sprintf(`{"messages":[{"content":[{"type":%q,"cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`, kind)
			request := &httpclient.Request{APIFormat: string(llm.APIFormatAnthropicMessage), Body: []byte(body)}
			require.Equal(t, time.Hour, sessionOwnerProtectionTTL(request))
		})
	}
}
