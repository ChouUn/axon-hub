package orchestrator

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

// TestBuildTestRequestUsesConfiguredPrompts verifies ordinary channel tests retain their configured prompts.
func TestBuildTestRequestUsesConfiguredPrompts(t *testing.T) {
	req := buildChannelTestRequest("test-model", true, "system prompt", "user prompt", false)

	require.Equal(t, "test-model", req.Model)
	require.Len(t, req.Messages, 2)
	require.Equal(t, "system", req.Messages[0].Role)
	require.Equal(t, "system prompt", *req.Messages[0].Content.Content)
	require.Equal(t, "user", req.Messages[1].Role)
	require.Equal(t, "user prompt", *req.Messages[1].Content.Content)
	require.Equal(t, int64(256), *req.MaxCompletionTokens)
	require.True(t, *req.Stream)
}

func TestBuildChannelTestImageRequestUsesLowCostDefaults(t *testing.T) {
	req := buildChannelTestImageRequest("gpt-image-2", "a red square")

	require.Equal(t, "gpt-image-2", req.Model)
	require.Equal(t, "a red square", req.Prompt)
	require.Equal(t, int64(1), *req.N)
	require.Equal(t, "1024x1024", req.Size)
	require.Equal(t, "low", req.Quality)
	require.Equal(t, "b64_json", req.ResponseFormat)
}

func TestChannelUsesImageTest(t *testing.T) {
	require.False(t, channelUsesImageTest(nil))
	require.False(t, channelUsesImageTest(&objects.ChannelSettings{}))
	require.False(t, channelUsesImageTest(&objects.ChannelSettings{PrimaryAPIFormat: "openai/chat_completions"}))
	require.True(t, channelUsesImageTest(&objects.ChannelSettings{PrimaryAPIFormat: objects.PrimaryAPIFormatImageGeneration}))
}

func TestInterpretTestChannelBodyImage(t *testing.T) {
	result, err := interpretTestChannelBody([]byte(`{"created":1,"data":[{"revised_prompt":"ok"}]}`), 0.5, true)
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, "ok", *result.Message)

	empty, err := interpretTestChannelBody([]byte(`{"created":1,"data":[]}`), 0.5, true)
	require.NoError(t, err)
	require.False(t, empty.Success)
	require.Equal(t, "No image in response", *empty.Error)
}

// TestBuildTestRequestUsesPingForResponsesWebSocket verifies WebSocket tests use a minimal compatible payload.
func TestBuildTestRequestUsesPingForResponsesWebSocket(t *testing.T) {
	req := buildChannelTestRequest("test-model", false, "system prompt", "user prompt", true)

	require.Equal(t, "test-model", req.Model)
	require.Len(t, req.Messages, 1)
	require.Equal(t, "user", req.Messages[0].Role)
	require.Equal(t, responsesWebSocketTestPrompt, *req.Messages[0].Content.Content)
	require.Nil(t, req.MaxCompletionTokens)
	require.True(t, *req.Stream)
}

// TestUsesResponsesWebSocket verifies explicit and URL-inferred WebSocket transports.
func TestUsesResponsesWebSocket(t *testing.T) {
	t.Run("inferred from channel base URL", func(t *testing.T) {
		ch := &biz.Channel{Channel: &ent.Channel{
			Type:    channel.TypeOpenaiResponses,
			BaseURL: "wss://api.openai.com/v1",
		}}

		require.True(t, usesResponsesWebSocket(ch))
	})

	t.Run("explicit transport", func(t *testing.T) {
		ch := &biz.Channel{Channel: &ent.Channel{
			Type:    channel.TypeOpenai,
			BaseURL: "https://api.example.com/v1",
			Endpoints: []objects.ChannelEndpoint{{
				APIFormat: llm.APIFormatOpenAIResponse.String(),
				Transport: objects.ChannelEndpointTransportWebSocket,
			}},
		}}

		require.True(t, usesResponsesWebSocket(ch))
	})

	t.Run("HTTP transport", func(t *testing.T) {
		ch := &biz.Channel{Channel: &ent.Channel{
			Type:    channel.TypeOpenaiResponses,
			BaseURL: "https://api.openai.com/v1",
		}}

		require.False(t, usesResponsesWebSocket(ch))
	})
}
