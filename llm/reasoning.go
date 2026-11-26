// 职责: 定义统一推理强度档位、渠道推理强度映射，以及模型名推理强度后缀的拆分规则。
// 关联文件:
//   internal/server/orchestrator/auto_reasoning_effort.go
//   internal/server/biz/channel_model_effort.go
//   internal/server/biz/channel_price.go
//   frontend/src/features/models/data/reasoning-efforts.ts
// 注意事项:
//   后缀拆分规则与前端 splitReasoningEffortModelSuffix 保持一致。
//   qwen 系列的 -max 是模型名的一部分，不是推理强度。

package llm

import "strings"

// Unified reasoning effort levels carried by llm.Request.ReasoningEffort.
// These are protocol-independent labels; each outbound transformer maps them to
// its native representation (OpenAI reasoning_effort, Anthropic thinking/output_config,
// etc.). Values not in this list pass through transformers unchanged.
const (
	ReasoningEffortNone    = "none"
	ReasoningEffortMinimal = "minimal"
	ReasoningEffortLow     = "low"
	ReasoningEffortMedium  = "medium"
	ReasoningEffortHigh    = "high"
	ReasoningEffortXHigh   = "xhigh"
	ReasoningEffortMax     = "max"
)

// ApplyReasoningEffortMapping replaces an effort value according to a per-channel
// mapping. The first entry whose From matches wins; values not in the list (or an
// empty/nil list) pass through unchanged. Applied centrally by the orchestrator on
// the unified request before the outbound transformer runs, so it affects every
// outbound protocol uniformly.
func ApplyReasoningEffortMapping(effort string, mappings []ReasoningEffortMapping) string {
	if len(mappings) == 0 || effort == "" {
		return effort
	}

	for _, m := range mappings {
		if m.From == effort {
			return m.To
		}
	}

	return effort
}

// ModelSuffixReasoningEfforts are the effort levels recognized as a model name
// suffix ("<base>-<effort>"), in ascending order. "none" and "minimal" are not
// suffixes: no provider names a model after them.
var ModelSuffixReasoningEfforts = []string{
	ReasoningEffortLow,
	ReasoningEffortMedium,
	ReasoningEffortHigh,
	ReasoningEffortXHigh,
	ReasoningEffortMax,
}

// SplitReasoningEffortModelSuffix splits "<base>-<effort>" at the last dash.
// The effort is matched case-insensitively against ModelSuffixReasoningEfforts
// and returned lowercased; the base keeps its original casing. Qwen "-max"
// models are not split because "max" is part of their model name.
func SplitReasoningEffortModelSuffix(model string) (base string, effort string, ok bool) {
	lastDash := strings.LastIndex(model, "-")
	if lastDash <= 0 || lastDash == len(model)-1 {
		return "", "", false
	}

	effort = strings.ToLower(model[lastDash+1:])
	if !isModelSuffixReasoningEffort(effort) {
		return "", "", false
	}

	if effort == ReasoningEffortMax && isQwenMaxModel(model) {
		return "", "", false
	}

	return model[:lastDash], effort, true
}

func isModelSuffixReasoningEffort(effort string) bool {
	for _, candidate := range ModelSuffixReasoningEfforts {
		if candidate == effort {
			return true
		}
	}

	return false
}

func isQwenMaxModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasSuffix(normalized, "-max") {
		return false
	}

	if lastSlash := strings.LastIndex(normalized, "/"); lastSlash >= 0 {
		normalized = normalized[lastSlash+1:]
	}

	return strings.HasPrefix(normalized, "qwen")
}

// ReasoningEffortMapping is a single reasoning_effort value mapping entry.
// From is the inbound effort value (e.g. "xhigh"); To is the outbound value
// (e.g. "max") sent to the upstream provider.
//
// Defined at the llm root package (rather than under transformer/openai) so that
// both the OpenAI transformer implementation and internal/objects can reference
// the same strong-typed entry without internal/objects depending on a concrete
// transformer package. Mirrors the ModelMapping pattern in internal/objects.
type ReasoningEffortMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}
