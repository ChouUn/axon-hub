package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/pipeline"
)

// modelNameServer is implemented by selectors pinned to one channel. When that
// channel accepts the requested name as is (for example a channel test of
// "gemini-4-hydrogen-xhigh"), the name is not split into base model and effort.
type modelNameServer interface {
	ServesModelName(ctx context.Context, model string) bool
}

func applyAutoReasoningEffort(settings *RequestSettings, selector CandidateSelector) pipeline.Middleware {
	return &autoReasoningEffortMiddleware{
		settings: settings,
		selector: selector,
	}
}

type autoReasoningEffortMiddleware struct {
	pipeline.DummyMiddleware

	settings *RequestSettings
	selector CandidateSelector
}

func (m *autoReasoningEffortMiddleware) Name() string {
	return "auto-reasoning-effort"
}

func (m *autoReasoningEffortMiddleware) OnInboundLlmRequest(ctx context.Context, llmRequest *llm.Request) (*llm.Request, error) {
	if llmRequest == nil || llmRequest.Model == "" {
		return llmRequest, nil
	}

	if !m.settings.ModelSettings.AutoReasoningEffort {
		return llmRequest, nil
	}

	baseModel, reasoningEffort, ok := splitAutoReasoningEffortModel(llmRequest.Model)
	if !ok {
		return llmRequest, nil
	}

	if pinned, isPinned := m.selector.(modelNameServer); isPinned && pinned.ServesModelName(ctx, llmRequest.Model) {
		return llmRequest, nil
	}

	originalModel := llmRequest.Model
	llmRequest.Model = baseModel
	llmRequest.ReasoningEffort = reasoningEffort

	log.Debug(ctx, "applied auto reasoning effort",
		log.String("original_model", originalModel),
		log.String("normalized_model", baseModel),
		log.String("reasoning_effort", reasoningEffort),
	)

	return llmRequest, nil
}

func splitAutoReasoningEffortModel(model string) (baseModel string, reasoningEffort string, ok bool) {
	return llm.SplitReasoningEffortModelSuffix(model)
}
