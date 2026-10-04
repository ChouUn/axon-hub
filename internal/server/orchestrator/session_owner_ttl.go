package orchestrator

import (
	"time"

	"github.com/tidwall/gjson"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

// sessionOwnerProtectionTTL returns the longest valid Anthropic prompt-cache TTL
// in the final outbound request. Call it after pass-through, cache TTL rewriting,
// and body overrides, using RawProviderRequest rather than the inbound request or
// JSONBody. It does not mutate the request. The minimum protection is five
// minutes, including for nil, non-Anthropic, malformed, or uncached requests.
func sessionOwnerProtectionTTL(request *httpclient.Request) time.Duration {
	if request == nil || request.APIFormat != string(llm.APIFormatAnthropicMessage) ||
		!gjson.ValidBytes(request.Body) {
		return 5 * time.Minute
	}

	body := gjson.ParseBytes(request.Body)
	if !body.IsObject() {
		return 5 * time.Minute
	}

	ttl := max(5*time.Minute, sessionOwnerCacheControlTTL(body.Get("cache_control")))
	ttl = max(ttl, sessionOwnerArrayProtectionTTL(body.Get("tools"), "tools"))
	ttl = max(ttl, sessionOwnerArrayProtectionTTL(body.Get("system"), "system"))
	messages := body.Get("messages")
	if messages.IsArray() {
		messages.ForEach(func(_, message gjson.Result) bool {
			if message.IsObject() {
				ttl = max(ttl, sessionOwnerArrayProtectionTTL(message.Get("content"), "content"))
			}
			return true
		})
	}
	return ttl
}

// Only Anthropic's two supported TTLs are accepted; arbitrary duration strings
// must never extend owner protection. Omitted TTL means the default five minutes.
func sessionOwnerCacheControlTTL(control gjson.Result) time.Duration {
	if !control.IsObject() {
		return 0
	}
	kind := control.Get("type")
	if kind.Type != gjson.String || kind.String() != "ephemeral" {
		return 0
	}
	ttl := control.Get("ttl")
	if !ttl.Exists() {
		return 5 * time.Minute
	}
	if ttl.Type != gjson.String {
		return 0
	}
	switch ttl.String() {
	case "5m":
		return 5 * time.Minute
	case "1h":
		return time.Hour
	default:
		return 0
	}
}

// Walk protocol content arrays only. Recursing through arbitrary objects would
// mistake tool inputs, JSON schemas, metadata, or text for cache breakpoints.
func sessionOwnerArrayProtectionTTL(array gjson.Result, location string) time.Duration {
	if !array.IsArray() {
		return 0
	}
	var ttl time.Duration
	array.ForEach(func(_, entry gjson.Result) bool {
		if !entry.IsObject() {
			return true
		}
		kind := entry.Get("type")
		switch location {
		case "tools":
			name := entry.Get("name")
			if name.Type != gjson.String || name.String() == "" {
				return true
			}
		case "system":
			if kind.Type != gjson.String || kind.String() != "text" {
				return true
			}
		case "content":
			if !sessionOwnerContentBlockType(kind) {
				return true
			}
		default:
			return true
		}
		ttl = max(ttl, sessionOwnerCacheControlTTL(entry.Get("cache_control")))
		if location == "content" && kind.String() == "tool_result" {
			ttl = max(ttl, sessionOwnerArrayProtectionTTL(entry.Get("content"), "content"))
		}
		return true
	})
	return ttl
}

// Keep this allowlist aligned with Anthropic content types supported by the
// transformer. Unknown business objects must not become cache breakpoints.
func sessionOwnerContentBlockType(kind gjson.Result) bool {
	if kind.Type != gjson.String {
		return false
	}
	switch kind.String() {
	case "text", "image", "document", "thinking", "redacted_thinking",
		"tool_use", "tool_result", "server_tool_use", "mcp_tool_use",
		"web_search_tool_result", "web_fetch_tool_result", "container_upload", "code_execution_tool_result", "mcp_tool_result",
		"bash_code_execution_tool_result", "text_editor_code_execution_tool_result",
		"tool_search_tool_result", "web_search_result", "search_result":
		return true
	default:
		return false
	}
}
