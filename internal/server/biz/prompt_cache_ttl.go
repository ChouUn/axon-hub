package biz

import (
	"strconv"

	"github.com/tidwall/gjson"

	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

// WalkAnthropicCacheControls visits cache controls at Anthropic's prompt-cache
// breakpoint positions, including content nested inside tool results.
func WalkAnthropicCacheControls(body []byte, visit func(path string, control gjson.Result)) {
	walkAnthropicCacheControl(gjson.GetBytes(body, "cache_control"), "cache_control", visit)
	walkAnthropicCacheArray(gjson.GetBytes(body, "tools"), "tools", false, visit)
	walkAnthropicCacheArray(gjson.GetBytes(body, "system"), "system", false, visit)

	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return
	}
	index := 0
	messages.ForEach(func(_, message gjson.Result) bool {
		walkAnthropicCacheArray(message.Get("content"), "messages."+strconv.Itoa(index)+".content", true, visit)
		index++
		return true
	})
}

func walkAnthropicCacheControl(control gjson.Result, path string, visit func(string, gjson.Result)) {
	if control.IsObject() {
		visit(path, control)
	}
}

func walkAnthropicCacheArray(array gjson.Result, path string, nestedContent bool, visit func(string, gjson.Result)) {
	if !array.IsArray() {
		return
	}
	index := 0
	array.ForEach(func(_, entry gjson.Result) bool {
		entryPath := path + "." + strconv.Itoa(index)
		walkAnthropicCacheControl(entry.Get("cache_control"), entryPath+".cache_control", visit)
		if nestedContent {
			walkAnthropicCacheArray(entry.Get("content"), entryPath+".content", true, visit)
		}
		index++
		return true
	})
}

// hasOneHourPromptCache reads the final upstream body, regardless of whether
// the channel's force-one-hour setting was enabled.
func hasOneHourPromptCache(channelRequest httpclient.Request, format llm.APIFormat) bool {
	if format != llm.APIFormatAnthropicMessage {
		return false
	}
	found := false
	WalkAnthropicCacheControls(channelRequest.Body, func(_ string, control gjson.Result) {
		if control.Get("type").String() == "ephemeral" && control.Get("ttl").String() == "1h" {
			found = true
		}
	})
	return found
}
