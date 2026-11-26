// 职责: 按请求的推理强度把「识别档位后缀」别名解析为具体的上游模型，并在发往上游前清除重复的推理参数。
// 关联文件:
//   internal/server/biz/channel_model_effort.go
//   internal/server/orchestrator/candidates_condition.go
//   internal/server/orchestrator/outbound.go
//   internal/server/orchestrator/auto_reasoning_effort.go
// 注意事项:
//   resolvedAssociationCandidate 来自关联解析缓存，只能复制，不能原地修改其 models。
//   解析后的 ActualModel 决定去重、健康门控、配额、计价和 Responses 续接来源，必须在聚合前完成。
//   清除推理参数只作用于本次尝试的请求副本，切换到其他候选时仍使用原始参数。

package orchestrator

import (
	"bytes"
	"fmt"
	"maps"

	"github.com/samber/lo"
	"github.com/tidwall/sjson"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
)

// ReasoningEffortUnavailableError reports that the request named a reasoning
// effort that none of the matching effort-suffix channels offers. It wraps
// biz.ErrInvalidModel so inbound transformers keep rendering it as an invalid
// model error.
type ReasoningEffortUnavailableError struct {
	ModelName string
	Effort    string
}

func (e *ReasoningEffortUnavailableError) Error() string {
	return fmt.Sprintf("%s: no channel offers reasoning effort %q for model %s", biz.ErrInvalidModel, e.Effort, e.ModelName)
}

func (e *ReasoningEffortUnavailableError) Unwrap() error {
	return biz.ErrInvalidModel
}

// resolveReasoningEffortCandidates replaces effort-suffix aliases with the
// variant for the request's reasoning effort and drops aliases that cannot
// serve it. emptied reports that dropping those aliases left no candidates, so
// the missing effort is the reason the request has nowhere to go.
func resolveReasoningEffortCandidates(
	req *llm.Request,
	candidates []*resolvedAssociationCandidate,
) (resolved []*resolvedAssociationCandidate, emptied bool) {
	effort := reqReasoningEffort(req)
	resolved = candidates
	copied := false
	dropped := false

	for index, candidate := range candidates {
		if candidate == nil || !hasEffortSuffixEntry(candidate.models) {
			if copied {
				resolved = append(resolved, candidate)
			}

			continue
		}

		if !copied {
			resolved = make([]*resolvedAssociationCandidate, index, len(candidates))
			copy(resolved, candidates[:index])
			copied = true
		}

		// An alias and another entry of the same association can resolve to the
		// same upstream model, e.g. a regex matching both "m" and "m-high".
		// Aggregation keeps the first entry per upstream model, so aliases go
		// first and such requests always take the alias path.
		models := make([]biz.ChannelModelEntry, 0, len(candidate.models))
		for _, aliasPass := range []bool{true, false} {
			for _, entry := range candidate.models {
				if (entry.EffortVariants != nil) != aliasPass {
					continue
				}

				resolvedEntry, ok := candidate.channel.ResolveReasoningEffortEntry(entry, effort)
				if !ok {
					dropped = true
					continue
				}

				models = append(models, resolvedEntry)
			}
		}

		if len(models) == 0 {
			continue
		}

		clone := *candidate
		clone.models = models
		resolved = append(resolved, &clone)
	}

	return resolved, dropped && !lo.ContainsBy(resolved, func(candidate *resolvedAssociationCandidate) bool {
		return candidate != nil && candidate.channel != nil && len(candidate.models) > 0
	})
}

func hasEffortSuffixEntry(entries []biz.ChannelModelEntry) bool {
	for _, entry := range entries {
		if entry.EffortVariants != nil {
			return true
		}
	}

	return false
}

// clearReasoningForEffortSuffix returns a copy of req without reasoning
// parameters, so the effort carried by the variant's model name is the only
// effort signal the upstream receives. Thought visibility settings are kept.
func clearReasoningForEffortSuffix(req *llm.Request) *llm.Request {
	newReq := *req
	newReq.ReasoningEffort = ""
	newReq.ReasoningBudget = nil

	_, hasThinkingType := req.TransformerMetadata[anthropic.TransformerMetadataKeyThinkingType]
	_, hasOutputEffort := req.TransformerMetadata[anthropic.TransformerMetadataKeyOutputConfigEffort]
	if hasThinkingType || hasOutputEffort {
		metadata := maps.Clone(req.TransformerMetadata)
		delete(metadata, anthropic.TransformerMetadataKeyThinkingType)
		delete(metadata, anthropic.TransformerMetadataKeyOutputConfigEffort)
		newReq.TransformerMetadata = metadata
	}

	if len(req.ExtraBody) > 0 {
		extraBody := bytes.Clone(req.ExtraBody)
		for _, path := range []string{"google.thinking_config.thinking_level", "google.thinking_config.thinking_budget"} {
			if updated, err := sjson.DeleteBytes(extraBody, path); err == nil {
				extraBody = updated
			}
		}

		newReq.ExtraBody = extraBody
	}

	return &newReq
}
