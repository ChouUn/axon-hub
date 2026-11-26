// 职责: 验证「识别档位后缀」的别名生成、选档、默认档位与未关联渠道模型判定。
// 关联文件:
//
//	internal/server/biz/channel_model_effort.go
//	internal/server/biz/channel_llm.go
//	internal/server/biz/model.go

package biz

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/llm"
)

func effortSuffixChannel(models []string, settings objects.ChannelSettings) *Channel {
	settings.ReasoningEffortModelSuffix = true

	return &Channel{Channel: &ent.Channel{ID: 7, SupportedModels: models, Settings: &settings}}
}

func TestGetModelEntries_ReasoningEffortSuffixAliases(t *testing.T) {
	ch := effortSuffixChannel([]string{
		"gemini-4-hydrogen-xhigh",
		"gemini-4-argon-high",
		"gemini-4-argon-low",
		"gemini-4-carbon",
		"gemini-4-carbon-high",
		"qwen3-max",
	}, objects.ChannelSettings{})

	entries := ch.GetModelEntries()

	hydrogen := entries["gemini-4-hydrogen"]
	require.Equal(t, ChannelModelSourceEffortSuffix, hydrogen.Source)
	require.Equal(t, "gemini-4-hydrogen-xhigh", hydrogen.ActualModel)

	argon := entries["gemini-4-argon"]
	require.Equal(t, ChannelModelSourceEffortSuffix, argon.Source)
	require.Equal(t, []ReasoningEffortVariant{
		{Effort: "low", RequestModel: "gemini-4-argon-low", ActualModel: "gemini-4-argon-low"},
		{Effort: "high", RequestModel: "gemini-4-argon-high", ActualModel: "gemini-4-argon-high"},
	}, argon.EffortVariants.Variants())
	// low and high are equally far from medium; the lower effort wins.
	require.Equal(t, "gemini-4-argon-low", argon.ActualModel)

	// A native base model keeps its own entry.
	require.Equal(t, ChannelModelEntry{RequestModel: "gemini-4-carbon", ActualModel: "gemini-4-carbon", Source: "direct"}, entries["gemini-4-carbon"])
	require.Equal(t, "direct", entries["gemini-4-hydrogen-xhigh"].Source)
	require.NotContains(t, entries, "qwen3")

	disabled := &Channel{Channel: &ent.Channel{SupportedModels: []string{"gemini-4-hydrogen-xhigh"}, Settings: &objects.ChannelSettings{}}}
	require.NotContains(t, disabled.GetModelEntries(), "gemini-4-hydrogen")
}

func TestGetModelEntries_ReasoningEffortSuffixWithOtherSettings(t *testing.T) {
	ch := effortSuffixChannel([]string{"google/Gemini-4-Hydrogen-XHIGH"}, objects.ChannelSettings{
		AutoTrimedModelPrefixes: []string{"google"},
		HideOriginalModels:      true,
		LowercaseModelID:        true,
	})

	entries := ch.GetModelEntries()

	alias, ok := entries["gemini-4-hydrogen"]
	require.True(t, ok, "auto-trimmed variants produce a lowercased base alias")
	require.Equal(t, "google/Gemini-4-Hydrogen-XHIGH", alias.ActualModel)
	require.Equal(t, "gemini-4-hydrogen-xhigh", alias.EffortVariants.Variants()[0].RequestModel)
	require.NotContains(t, entries, "google/gemini-4-hydrogen-xhigh", "hideOriginalModels hides the direct name")
}

// With lowercase set, bases that differ only in casing form one alias; grouping
// by the raw base would build two aliases and keep only one after lowercasing.
func TestGetModelEntries_ReasoningEffortSuffixMixedCaseBases(t *testing.T) {
	for range 20 {
		ch := effortSuffixChannel([]string{"Gemini-X-high", "gemini-x-low"}, objects.ChannelSettings{LowercaseModelID: true})

		alias := ch.GetModelEntries()["gemini-x"]
		require.Equal(t, []ReasoningEffortVariant{
			{Effort: "low", RequestModel: "gemini-x-low", ActualModel: "gemini-x-low"},
			{Effort: "high", RequestModel: "gemini-x-high", ActualModel: "Gemini-X-high"},
		}, alias.EffortVariants.Variants())
	}

	// A native base that differs only in casing still wins once names are lowercased.
	ch := effortSuffixChannel([]string{"Gemini-X", "gemini-x-high"}, objects.ChannelSettings{LowercaseModelID: true})
	require.Equal(t, "direct", ch.GetModelEntries()["gemini-x"].Source)

	// Without lowercase the casing is significant and the bases stay separate.
	ch = effortSuffixChannel([]string{"Gemini-X-high", "gemini-x-low"}, objects.ChannelSettings{})
	entries := ch.GetModelEntries()
	require.Equal(t, "Gemini-X-high", entries["Gemini-X"].ActualModel)
	require.Equal(t, "gemini-x-low", entries["gemini-x"].ActualModel)
}

// A manual mapping whose upstream model has another effort must not join the
// alias: clearing the request's "high" would then silently send a low model.
func TestGetModelEntries_ReasoningEffortSuffixSkipsCrossEffortMapping(t *testing.T) {
	ch := effortSuffixChannel([]string{"upstream-low", "vendor-x-high"}, objects.ChannelSettings{
		ModelMappings: []objects.ModelMapping{
			{From: "m-high", To: "upstream-low"},
			{From: "x-high", To: "vendor-x-high"},
		},
	})
	entries := ch.GetModelEntries()

	require.Equal(t, "upstream-low", entries["m-high"].ActualModel, "the mapping itself still works")
	require.NotContains(t, entries, "m", "no alias claims high for an upstream low model")
	require.Equal(t, []ReasoningEffortVariant{
		{Effort: "high", RequestModel: "x-high", ActualModel: "vendor-x-high"},
	}, entries["x"].EffortVariants.Variants(), "a mapping to an upstream model of the same effort still joins")
}

func TestResolveReasoningEffortEntry(t *testing.T) {
	ch := effortSuffixChannel([]string{"gemini-4-argon-high", "gemini-4-argon-low"}, objects.ChannelSettings{
		TransformOptions: objects.TransformOptions{
			ReasoningEffortMapping: []llm.ReasoningEffortMapping{{From: "xhigh", To: "high"}, {From: "low", To: "high"}},
		},
	})
	alias := ch.GetModelEntries()["gemini-4-argon"]

	for _, tc := range []struct {
		effort string
		want   string
		ok     bool
	}{
		{effort: "", want: "gemini-4-argon-low", ok: true},
		{effort: "high", want: "gemini-4-argon-high", ok: true},
		{effort: "HIGH", want: "gemini-4-argon-high", ok: true},
		// The exact variant wins over the mapping.
		{effort: "low", want: "gemini-4-argon-low", ok: true},
		// The mapping is used only because no -xhigh variant exists.
		{effort: "xhigh", want: "gemini-4-argon-high", ok: true},
		// No exact variant and no mapping: the channel does not serve the request.
		{effort: "medium", ok: false},
		{effort: "none", ok: false},
	} {
		t.Run(tc.effort, func(t *testing.T) {
			got, ok := ch.ResolveReasoningEffortEntry(alias, tc.effort)
			require.Equal(t, tc.ok, ok)
			if tc.ok {
				require.Equal(t, tc.want, got.ActualModel)
				require.Equal(t, "gemini-4-argon", got.RequestModel, "the alias keeps its request name")
				variantModel, ok := got.SelectedVariantRequestModel()
				require.True(t, ok)
				require.Equal(t, tc.want, variantModel)
			}
		})
	}

	direct := ch.GetModelEntries()["gemini-4-argon-high"]
	got, ok := ch.ResolveReasoningEffortEntry(direct, "medium")
	require.True(t, ok, "entries without variants are never filtered by effort")
	require.Equal(t, direct, got)
}

func TestReasoningEffortDefaultVariant(t *testing.T) {
	for _, tc := range []struct {
		models []string
		want   string
	}{
		{models: []string{"m-low", "m-medium", "m-max"}, want: "m-medium"},
		{models: []string{"m-xhigh", "m-max"}, want: "m-xhigh"},
		{models: []string{"m-high", "m-max"}, want: "m-high"},
	} {
		alias := effortSuffixChannel(tc.models, objects.ChannelSettings{}).GetModelEntries()["m"]
		require.Equal(t, tc.want, alias.ActualModel, tc.models)
	}
}

func TestFindUnassociatedChannels_EffortSuffixAliasCoversVariants(t *testing.T) {
	ch := effortSuffixChannel([]string{"gemini-4-argon-high", "gemini-4-argon-low", "other-model"}, objects.ChannelSettings{})
	associations := []*objects.ModelAssociation{{
		Type:    "model",
		ModelID: &objects.ModelIDAssociation{ModelID: "gemini-4-argon"},
	}}

	result := findUnassociatedChannels([]*ent.Channel{ch.Channel}, associations)

	require.Len(t, result, 1)
	require.Equal(t, []string{"other-model"}, result[0].Models)
}
