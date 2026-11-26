package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

func TestSplitAutoReasoningEffortModel(t *testing.T) {
	tests := []struct {
		name           string
		model          string
		wantBaseModel  string
		wantEffort     string
		wantNormalized bool
	}{
		{
			name:           "supports xhigh",
			model:          "gpt-5.4-xhigh",
			wantBaseModel:  "gpt-5.4",
			wantEffort:     "xhigh",
			wantNormalized: true,
		},
		{
			name:           "supports max",
			model:          "gpt-5.4-max",
			wantBaseModel:  "gpt-5.4",
			wantEffort:     "max",
			wantNormalized: true,
		},
		{
			name:           "ignores qwen max model",
			model:          "qwen3.7-max",
			wantNormalized: false,
		},
		{
			name:           "ignores namespaced qwen max model",
			model:          "qwen/qwen3-max",
			wantNormalized: false,
		},
		{
			name:           "supports uppercase suffix",
			model:          "gpt-5.4-HIGH",
			wantBaseModel:  "gpt-5.4",
			wantEffort:     "high",
			wantNormalized: true,
		},
		{
			name:           "ignores unsupported suffix",
			model:          "gpt-5.4-ultra",
			wantNormalized: false,
		},
		{
			name:           "ignores missing base model",
			model:          "-high",
			wantNormalized: false,
		},
		{
			name:           "ignores model without suffix",
			model:          "gpt-5.4",
			wantNormalized: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseModel, effort, ok := splitAutoReasoningEffortModel(tt.model)
			assert.Equal(t, tt.wantNormalized, ok)
			assert.Equal(t, tt.wantBaseModel, baseModel)
			assert.Equal(t, tt.wantEffort, effort)
		})
	}
}

func TestAutoReasoningEffortMiddleware_OnInboundLlmRequest(t *testing.T) {
	tests := []struct {
		name       string
		settings   biz.SystemModelSettings
		request    *llm.Request
		wantModel  string
		wantEffort string
	}{
		{
			name: "disabled setting leaves request unchanged",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: false,
			},
			request: &llm.Request{
				Model: "gpt-5.4-xhigh",
			},
			wantModel:  "gpt-5.4-xhigh",
			wantEffort: "",
		},
		{
			name: "enabled setting applies xhigh suffix",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: true,
			},
			request: &llm.Request{
				Model: "gpt-5.4-xhigh",
			},
			wantModel:  "gpt-5.4",
			wantEffort: "xhigh",
		},
		{
			name: "enabled setting applies max suffix",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: true,
			},
			request: &llm.Request{
				Model: "gpt-5.4-max",
			},
			wantModel:  "gpt-5.4",
			wantEffort: "max",
		},
		{
			name: "enabled setting keeps qwen max model unchanged",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: true,
			},
			request: &llm.Request{
				Model: "qwen3.7-max",
			},
			wantModel:  "qwen3.7-max",
			wantEffort: "",
		},
		{
			name: "suffix reasoning effort overrides explicit request value",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: true,
			},
			request: &llm.Request{
				Model:           "gpt-5.4-xhigh",
				ReasoningEffort: "high",
			},
			wantModel:  "gpt-5.4",
			wantEffort: "xhigh",
		},
		{
			name: "unsupported suffix is ignored",
			settings: biz.SystemModelSettings{
				AutoReasoningEffort: true,
			},
			request: &llm.Request{
				Model: "gpt-5.4-ultra",
			},
			wantModel:  "gpt-5.4-ultra",
			wantEffort: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			middleware := applyAutoReasoningEffort(&RequestSettings{ModelSettings: tt.settings}, nil)
			result, err := middleware.OnInboundLlmRequest(context.Background(), tt.request)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantModel, result.Model)
			assert.Equal(t, tt.wantEffort, result.ReasoningEffort)
		})
	}
}

type stubModelNameServer struct {
	CandidateSelector

	served map[string]bool
}

func (s stubModelNameServer) ServesModelName(_ context.Context, model string) bool {
	return s.served[model]
}

// A channel test or playground probe pinned to a channel sends the channel's own
// model names; splitting them would ask the channel for a base name it lacks.
func TestAutoReasoningEffortMiddleware_PinnedChannelKeepsServedName(t *testing.T) {
	settings := &RequestSettings{ModelSettings: biz.SystemModelSettings{AutoReasoningEffort: true}}
	selector := stubModelNameServer{served: map[string]bool{"gemini-4-hydrogen-xhigh": true}}
	middleware := applyAutoReasoningEffort(settings, selector)

	kept, err := middleware.OnInboundLlmRequest(context.Background(), &llm.Request{Model: "gemini-4-hydrogen-xhigh"})
	require.NoError(t, err)
	assert.Equal(t, "gemini-4-hydrogen-xhigh", kept.Model)
	assert.Empty(t, kept.ReasoningEffort)

	split, err := middleware.OnInboundLlmRequest(context.Background(), &llm.Request{Model: "gpt-5.4-high"})
	require.NoError(t, err)
	assert.Equal(t, "gpt-5.4", split.Model)
	assert.Equal(t, "high", split.ReasoningEffort)
}
