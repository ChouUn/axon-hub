// 职责: 为开启「识别档位后缀」的渠道生成基础模型别名，并按请求的推理强度解析实际发送的上游模型。
// 关联文件:
//   internal/server/biz/channel_llm.go
//   internal/server/biz/model.go
//   internal/server/orchestrator/candidates_effort.go
//   internal/server/orchestrator/outbound.go
//   llm/reasoning.go
// 注意事项:
//   别名条目及其档位表在 GetModelEntries 中构建后只读，调用方不得修改。
//   显式推理强度只做精确匹配；渠道推理强度映射仅在基础模型缺少该档位时使用一次，不就近升降档。
//   渠道已有与基础模型同名的条目时以已有条目为准，不生成别名。

package biz

import (
	"strings"

	"github.com/looplj/axonhub/llm"
)

// ChannelModelSourceEffortSuffix marks an alias entry that serves "<base>" through
// the channel's "<base>-<effort>" models.
const ChannelModelSourceEffortSuffix = "effort_suffix"

// ReasoningEffortVariant is one upstream model behind an effort-suffix alias.
type ReasoningEffortVariant struct {
	Effort string
	// RequestModel is the channel request name of the variant, e.g. "gemini-4-hydrogen-xhigh".
	RequestModel string
	ActualModel  string
}

// ReasoningEffortVariants lists the variants of an effort-suffix alias in
// ascending effort order.
type ReasoningEffortVariants struct {
	variants []ReasoningEffortVariant
}

// Variants returns the variants in ascending effort order. Do not modify the result.
func (v *ReasoningEffortVariants) Variants() []ReasoningEffortVariant {
	return v.variants
}

func (v *ReasoningEffortVariants) find(effort string) (ReasoningEffortVariant, bool) {
	effort = strings.ToLower(effort)
	for _, variant := range v.variants {
		if variant.Effort == effort {
			return variant, true
		}
	}

	return ReasoningEffortVariant{}, false
}

// defaultVariant serves requests without a reasoning effort: medium, otherwise
// the variant nearest to medium, preferring the lower effort on a tie.
func (v *ReasoningEffortVariants) defaultVariant() ReasoningEffortVariant {
	mediumRank := reasoningEffortRank(llm.ReasoningEffortMedium)
	best := v.variants[0]
	bestDistance := abs(reasoningEffortRank(best.Effort) - mediumRank)

	for _, variant := range v.variants[1:] {
		if distance := abs(reasoningEffortRank(variant.Effort) - mediumRank); distance < bestDistance {
			best, bestDistance = variant, distance
		}
	}

	return best
}

func reasoningEffortRank(effort string) int {
	for rank, candidate := range llm.ModelSuffixReasoningEfforts {
		if candidate == effort {
			return rank
		}
	}

	return -1
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

// addReasoningEffortAliases adds a "<base>" alias for every group of request
// names "<base>-<effort>". A base that is already a request name keeps its entry.
// With lowercase set, bases are grouped and compared case-insensitively, so
// "Gemini-X-high" and "gemini-x-low" form one alias with both variants instead
// of two aliases that collide once request names are lowercased.
func addReasoningEffortAliases(entries map[string]ChannelModelEntry, lowercase bool) {
	normalize := func(name string) string { return name }
	if lowercase {
		normalize = strings.ToLower
	}

	existing := make(map[string]struct{}, len(entries))
	for requestModel := range entries {
		existing[normalize(requestModel)] = struct{}{}
	}

	grouped := make(map[string]map[string]ChannelModelEntry)

	for requestModel, entry := range entries {
		base, effort, ok := llm.SplitReasoningEffortModelSuffix(requestModel)
		if !ok {
			continue
		}

		base = normalize(base)
		if _, exists := existing[base]; exists {
			continue
		}

		// A manual mapping may point "m-high" at an upstream model of another
		// effort. Only names whose upstream model carries the same effort join
		// the alias, so clearing the request effort never changes the effort sent.
		if entry.Source == "mapping" {
			if _, actualEffort, ok := llm.SplitReasoningEffortModelSuffix(entry.ActualModel); !ok || actualEffort != effort {
				continue
			}
		}

		byEffort, ok := grouped[base]
		if !ok {
			byEffort = make(map[string]ChannelModelEntry)
			grouped[base] = byEffort
		}

		if current, duplicate := byEffort[effort]; duplicate && !preferReasoningEffortVariant(entry, current) {
			continue
		}

		byEffort[effort] = entry
	}

	for base, byEffort := range grouped {
		variants := make([]ReasoningEffortVariant, 0, len(byEffort))
		for _, effort := range llm.ModelSuffixReasoningEfforts {
			if entry, ok := byEffort[effort]; ok {
				variants = append(variants, ReasoningEffortVariant{
					Effort:       effort,
					RequestModel: entry.RequestModel,
					ActualModel:  entry.ActualModel,
				})
			}
		}

		effortVariants := &ReasoningEffortVariants{variants: variants}
		entries[base] = ChannelModelEntry{
			RequestModel:   base,
			ActualModel:    effortVariants.defaultVariant().ActualModel,
			Source:         ChannelModelSourceEffortSuffix,
			EffortVariants: effortVariants,
		}
	}
}

// preferReasoningEffortVariant breaks ties between request names that differ
// only in casing, e.g. "m-high" and "m-HIGH".
func preferReasoningEffortVariant(candidate, existing ChannelModelEntry) bool {
	if sourcePriority[candidate.Source] != sourcePriority[existing.Source] {
		return sourcePriority[candidate.Source] > sourcePriority[existing.Source]
	}

	return candidate.RequestModel < existing.RequestModel
}

// lowercaseRequestModels follows LowercaseModelID for the variants' request names.
func (v *ReasoningEffortVariants) lowercaseRequestModels() *ReasoningEffortVariants {
	variants := make([]ReasoningEffortVariant, len(v.variants))
	for i, variant := range v.variants {
		variant.RequestModel = strings.ToLower(variant.RequestModel)
		variants[i] = variant
	}

	return &ReasoningEffortVariants{variants: variants}
}

// ResolveReasoningEffortEntry returns the entry to send for a request with the
// given reasoning effort. Entries other than effort-suffix aliases are returned
// unchanged.
//
// An empty effort selects the default variant. An explicit effort selects the
// variant with exactly that effort; only when the alias has no such variant is
// the channel's reasoning effort mapping applied once. ok is false when neither
// exists, so the channel does not serve the request.
//
// The resolved entry keeps the alias request name and points ActualModel at
// the variant; SelectedVariantRequestModel recovers the variant's own name.
func (ch *Channel) ResolveReasoningEffortEntry(entry ChannelModelEntry, effort string) (ChannelModelEntry, bool) {
	if entry.EffortVariants == nil {
		return entry, true
	}

	if effort == "" {
		entry.ActualModel = entry.EffortVariants.defaultVariant().ActualModel
		return entry, true
	}

	if variant, ok := entry.EffortVariants.find(effort); ok {
		entry.ActualModel = variant.ActualModel
		return entry, true
	}

	if ch != nil && ch.Settings != nil {
		mapped := llm.ApplyReasoningEffortMapping(effort, ch.Settings.TransformOptions.ReasoningEffortMapping)
		if mapped != effort {
			if variant, ok := entry.EffortVariants.find(mapped); ok {
				entry.ActualModel = variant.ActualModel
				return entry, true
			}
		}
	}

	return ChannelModelEntry{}, false
}

// SelectedVariantRequestModel returns the channel request name of the variant
// the entry currently targets, e.g. "gemini-4-hydrogen-xhigh" for the resolved
// alias "gemini-4-hydrogen". Per-model settings keyed by that name, such as
// protocol overrides, use it. ok is false for entries without variants.
func (e ChannelModelEntry) SelectedVariantRequestModel() (string, bool) {
	if e.EffortVariants == nil {
		return "", false
	}

	for _, variant := range e.EffortVariants.variants {
		if variant.ActualModel == e.ActualModel {
			return variant.RequestModel, true
		}
	}

	return "", false
}
