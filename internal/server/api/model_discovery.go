// 职责: 从注册目标的模型卡生成模型列表的发现字段，OpenAI、Anthropic 与 Gemini 三条线路的列表共用。
// 关联文件:
//   internal/server/api/openai.go
//   internal/server/api/anthropic.go
//   internal/server/api/gemini.go
//   internal/server/biz/model.go
// 注意事项:
//   OMP 的 openai-models-list 发现请求丢弃查询串，带不上 include，所以省略 include 的列表请求总带这些字段，
//   不受 default_model_api_include_all 控制。
//   值为空或为 0 时省略字段，模型卡未标记支持推理时不下发档位：三条线路要么同值，要么同时省略。
//   模型卡缺输出上限时不编造默认值，OMP 在不同协议上的回退值不同，见 docs/routing/feats/model-discovery.md。

package api

import "github.com/looplj/axonhub/internal/ent"

// ModelDiscoveryFields are the model-card fields that clients such as OMP read
// from a model list to discover limits, input types and reasoning levels.
type ModelDiscoveryFields struct {
	ContextLength            int      `json:"context_length,omitempty"`
	MaxOutputTokens          int      `json:"max_output_tokens,omitempty"`
	InputModalities          []string `json:"input_modalities,omitempty"`
	SupportedReasoningLevels []string `json:"supported_reasoning_levels,omitempty"`
}

// modelDiscoveryFields reads the discovery fields from the admitted target's
// model card. Aliases pass their target, so they report the target's card.
func modelDiscoveryFields(m *ent.Model) ModelDiscoveryFields {
	if m == nil || m.ModelCard == nil {
		return ModelDiscoveryFields{}
	}

	card := m.ModelCard
	fields := ModelDiscoveryFields{
		ContextLength:   card.Limit.Context,
		MaxOutputTokens: card.Limit.Output,
		InputModalities: card.Modalities.Input,
	}
	if card.Reasoning.Supported {
		fields.SupportedReasoningLevels = card.ReasoningEfforts
	}

	return fields
}
