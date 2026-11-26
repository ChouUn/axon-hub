// 职责: 验证档位后缀别名在候选筛选、指定渠道、协议覆盖、出站请求与请求体透传中的行为。
// 关联文件:
//
//	internal/server/orchestrator/candidates_effort.go
//	internal/server/orchestrator/candidates_condition.go
//	internal/server/orchestrator/candidates.go
//	internal/server/orchestrator/outbound.go
//	internal/server/orchestrator/pass_through.go

package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
)

func effortSuffixTestChannel(id int, models []string, mapping []llm.ReasoningEffortMapping) *biz.Channel {
	return &biz.Channel{Channel: &ent.Channel{
		ID:              id,
		Name:            "effort-suffix",
		SupportedModels: models,
		Settings: &objects.ChannelSettings{
			ReasoningEffortModelSuffix: true,
			TransformOptions:           objects.TransformOptions{ReasoningEffortMapping: mapping},
		},
	}}
}

func TestFilterResolvedCandidates_ResolvesEffortSuffixVariants(t *testing.T) {
	suffixChannel := effortSuffixTestChannel(1, []string{"gemini-4-argon-high", "gemini-4-argon-low"}, nil)
	nativeChannel := &biz.Channel{Channel: &ent.Channel{ID: 2, Name: "native", SupportedModels: []string{"gemini-4-argon"}}}
	resolved := []*resolvedAssociationCandidate{
		{channel: suffixChannel, models: []biz.ChannelModelEntry{suffixChannel.GetModelEntries()["gemini-4-argon"]}},
		{channel: nativeChannel, priority: 1, models: []biz.ChannelModelEntry{nativeChannel.GetModelEntries()["gemini-4-argon"]}},
	}
	cachedAlias := resolved[0].models[0]

	got, unavailable := filterResolvedCandidatesForRequest(context.Background(), &llm.Request{Model: "gemini-4-argon", ReasoningEffort: "high"}, resolved)
	require.False(t, unavailable)
	require.Len(t, got, 2)
	require.Equal(t, "gemini-4-argon-high", got[0].Models[0].ActualModel)
	require.Equal(t, "gemini-4-argon", got[1].Models[0].ActualModel)
	require.Equal(t, cachedAlias, resolved[0].models[0], "cached association results are not mutated")

	// The suffix channel lacks -medium; only the native channel remains, so the
	// missing effort is not why the request would fail.
	got, unavailable = filterResolvedCandidatesForRequest(context.Background(), &llm.Request{Model: "gemini-4-argon", ReasoningEffort: "medium"}, resolved)
	require.False(t, unavailable)
	require.Len(t, got, 1)
	require.Equal(t, nativeChannel.ID, got[0].Channel.ID)

	got, unavailable = filterResolvedCandidatesForRequest(context.Background(), &llm.Request{Model: "gemini-4-argon", ReasoningEffort: "medium"}, resolved[:1])
	require.True(t, unavailable, "the missing effort removed every candidate")
	require.Empty(t, got)
}

// A Decisions request needs an explicit endpoint. When the channel offering the
// effort lacks that endpoint, the request fails for the endpoint, not the effort.
func TestFilterResolvedCandidates_EndpointRejectionIsNotEffortUnavailable(t *testing.T) {
	lacksEffort := effortSuffixTestChannel(1, []string{"m-low"}, nil)
	lacksEndpoint := effortSuffixTestChannel(2, []string{"m-high"}, nil)
	resolved := []*resolvedAssociationCandidate{
		{channel: lacksEffort, models: []biz.ChannelModelEntry{lacksEffort.GetModelEntries()["m"]}},
		{channel: lacksEndpoint, models: []biz.ChannelModelEntry{lacksEndpoint.GetModelEntries()["m"]}},
	}

	got, unavailable := filterResolvedCandidatesForRequest(context.Background(), &llm.Request{
		Model:           "m",
		ReasoningEffort: "high",
		RequestType:     llm.RequestTypeDecisions,
	}, resolved)
	require.Empty(t, got)
	require.False(t, unavailable)
}

// A regex such as ^m(-high)?$ matches both the alias and its variant. The alias
// must win regardless of match order, so the upstream body does not depend on
// map iteration.
func TestFilterResolvedCandidates_AliasWinsOverDirectVariant(t *testing.T) {
	ch := effortSuffixTestChannel(1, []string{"m-high"}, nil)
	entries := ch.GetModelEntries()

	for _, order := range [][]string{{"m", "m-high"}, {"m-high", "m"}} {
		models := lo.Map(order, func(name string, _ int) biz.ChannelModelEntry { return entries[name] })
		got, _ := filterResolvedCandidatesForRequest(context.Background(), &llm.Request{Model: "m", ReasoningEffort: "high"}, []*resolvedAssociationCandidate{
			{channel: ch, models: models},
		})
		require.Len(t, got, 1, order)
		require.Len(t, got[0].Models, 1, order)
		require.Equal(t, "m-high", got[0].Models[0].ActualModel, order)
		require.NotNil(t, got[0].Models[0].EffortVariants, order)
	}
}

// A protocol override on the variant name applies when the base alias selects it.
func TestForcedAPIFormatsForCandidate_EffortSuffixVariantName(t *testing.T) {
	ch := effortSuffixTestChannel(1, []string{"google/m-high", "google/m-low"}, nil)
	ch.Settings.AutoTrimedModelPrefixes = []string{"google"}
	ch.Settings.ModelProtocols = []objects.ModelProtocol{{Model: "m-high", APIFormats: []string{llm.APIFormatAnthropicMessage.String()}}}
	alias := ch.GetModelEntries()["m"]

	high, ok := ch.ResolveReasoningEffortEntry(alias, "high")
	require.True(t, ok)
	require.Equal(t, "google/m-high", high.ActualModel)
	require.Equal(t, []string{llm.APIFormatAnthropicMessage.String()}, forcedAPIFormatsForCandidate(ch, []biz.ChannelModelEntry{high}, "m"))

	low, ok := ch.ResolveReasoningEffortEntry(alias, "")
	require.True(t, ok)
	require.Equal(t, "google/m-low", low.ActualModel)
	require.Nil(t, forcedAPIFormatsForCandidate(ch, []biz.ChannelModelEntry{low}, "m"))
}

func TestFilterResolvedCandidates_EffortMappingOnlyWhenVariantMissing(t *testing.T) {
	// A channel-wide high -> xhigh mapping must not move argon off its -high model.
	mapping := []llm.ReasoningEffortMapping{{From: "high", To: "xhigh"}}
	hydrogen := effortSuffixTestChannel(1, []string{"gemini-4-hydrogen-xhigh", "gemini-4-argon-high"}, mapping)
	entries := hydrogen.GetModelEntries()

	for _, tc := range []struct {
		model string
		want  string
	}{
		{model: "gemini-4-hydrogen", want: "gemini-4-hydrogen-xhigh"},
		{model: "gemini-4-argon", want: "gemini-4-argon-high"},
	} {
		got, unavailable := filterResolvedCandidatesForRequest(context.Background(), &llm.Request{Model: tc.model, ReasoningEffort: "high"}, []*resolvedAssociationCandidate{
			{channel: hydrogen, models: []biz.ChannelModelEntry{entries[tc.model]}},
		})
		require.False(t, unavailable)
		require.Len(t, got, 1)
		require.Equal(t, tc.want, got[0].Models[0].ActualModel)
	}
}

func TestSpecifiedChannelSelector_EffortSuffix(t *testing.T) {
	ctx, client := setupTest(t)

	ch, err := client.Channel.Create().
		SetType(channel.TypeOpenai).
		SetName("Effort Suffix Channel").
		SetBaseURL("https://api.openai.com/v1").
		SetCredentials(objects.ChannelCredentials{APIKey: "test-key"}).
		SetSupportedModels([]string{"gemini-4-hydrogen-xhigh"}).
		SetSettings(&objects.ChannelSettings{ReasoningEffortModelSuffix: true}).
		SetDefaultTestModel("gemini-4-hydrogen-xhigh").
		SetStatus(channel.StatusDisabled).
		Save(ctx)
	require.NoError(t, err)

	selector := NewSpecifiedChannelSelector(newTestChannelServiceForChannels(client), objects.GUID{ID: ch.ID})

	require.True(t, selector.ServesModelName(ctx, "gemini-4-hydrogen-xhigh"))
	require.False(t, selector.ServesModelName(ctx, "gpt-5.4-high"))

	result, err := selector.Select(ctx, &llm.Request{Model: "gemini-4-hydrogen"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "gemini-4-hydrogen-xhigh", result[0].Models[0].ActualModel)

	_, err = selector.Select(ctx, &llm.Request{Model: "gemini-4-hydrogen", ReasoningEffort: "low"})
	var unavailable *ReasoningEffortUnavailableError
	require.ErrorAs(t, err, &unavailable)
	require.True(t, errors.Is(err, biz.ErrInvalidModel))
	require.Contains(t, err.Error(), `"low"`)
}

func TestClearReasoningForEffortSuffix(t *testing.T) {
	metadata := map[string]any{
		anthropic.TransformerMetadataKeyThinkingType:       "enabled",
		anthropic.TransformerMetadataKeyOutputConfigEffort: "high",
		anthropic.TransformerMetadataKeyThinkingDisplay:    "summarized",
	}
	extraBody := json.RawMessage(`{"google":{"thinking_config":{"thinking_level":"high","thinking_budget":2048,"include_thoughts":true}},"other":1}`)
	req := &llm.Request{
		Model:               "gemini-4-hydrogen-xhigh",
		ReasoningEffort:     "xhigh",
		ReasoningBudget:     lo.ToPtr(int64(30000)),
		TransformerMetadata: metadata,
		ExtraBody:           extraBody,
	}

	cleared := clearReasoningForEffortSuffix(req)

	require.Empty(t, cleared.ReasoningEffort)
	require.Nil(t, cleared.ReasoningBudget)
	require.NotContains(t, cleared.TransformerMetadata, anthropic.TransformerMetadataKeyThinkingType)
	require.NotContains(t, cleared.TransformerMetadata, anthropic.TransformerMetadataKeyOutputConfigEffort)
	require.Equal(t, "summarized", cleared.TransformerMetadata[anthropic.TransformerMetadataKeyThinkingDisplay])
	require.False(t, gjson.GetBytes(cleared.ExtraBody, "google.thinking_config.thinking_level").Exists())
	require.False(t, gjson.GetBytes(cleared.ExtraBody, "google.thinking_config.thinking_budget").Exists())
	require.True(t, gjson.GetBytes(cleared.ExtraBody, "google.thinking_config.include_thoughts").Bool())
	require.Equal(t, int64(1), gjson.GetBytes(cleared.ExtraBody, "other").Int())

	// The original request keeps its parameters for retries on other candidates.
	require.Equal(t, "xhigh", req.ReasoningEffort)
	require.Equal(t, "enabled", metadata[anthropic.TransformerMetadataKeyThinkingType])
	require.Equal(t, extraBody, req.ExtraBody)
}

func TestPersistentOutboundTransformer_TransformRequest_EffortSuffixVariant(t *testing.T) {
	outbound := &mockTransformer{
		apiFormat:        llm.APIFormatOpenAIChatCompletion,
		requestAPIFormat: llm.APIFormatOpenAIChatCompletion,
		includeEffort:    true,
	}
	selected := effortSuffixTestChannel(1, []string{"gemini-4-hydrogen-xhigh"}, []llm.ReasoningEffortMapping{{From: "xhigh", To: "max"}})
	selected.Outbound = outbound
	entry, ok := selected.ResolveReasoningEffortEntry(selected.GetModelEntries()["gemini-4-hydrogen"], "xhigh")
	require.True(t, ok)

	processor := &PersistentOutboundTransformer{
		wrapped: outbound,
		state: &PersistenceState{
			ChannelModelsCandidates: []*ChannelModelsCandidate{{
				Channel:   selected,
				Models:    []biz.ChannelModelEntry{entry},
				APIFormat: llm.APIFormatOpenAIChatCompletion.String(),
			}},
		},
	}
	request := &llm.Request{
		Model:           "gemini-4-hydrogen",
		ReasoningEffort: "xhigh",
		Messages:        []llm.Message{{Role: "user", Content: llm.MessageContent{Content: lo.ToPtr("hello")}}},
	}

	httpRequest, err := processor.TransformRequest(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "gemini-4-hydrogen-xhigh", gjson.GetBytes(httpRequest.Body, "model").String())
	require.Empty(t, gjson.GetBytes(httpRequest.Body, "reasoning_effort").String(), "the model name is the only effort signal")
	require.Equal(t, "xhigh", request.ReasoningEffort)
}

func TestApplyPassThroughBodySkipsEffortSuffixVariant(t *testing.T) {
	selected := effortSuffixTestChannel(1, []string{"gemini-4-hydrogen-xhigh"}, nil)
	selected.Settings.PassThroughBody = lo.ToPtr(true)
	entry := selected.GetModelEntries()["gemini-4-hydrogen"]

	outbound := &PersistentOutboundTransformer{
		wrapped: &passthroughOutbound{format: llm.APIFormatOpenAIChatCompletion},
		state: &PersistenceState{
			CurrentCandidate: &ChannelModelsCandidate{Channel: selected, Models: []biz.ChannelModelEntry{entry}},
			LlmRequest: &llm.Request{
				Model:     "gemini-4-hydrogen-xhigh",
				APIFormat: llm.APIFormatOpenAIChatCompletion,
				RawRequest: &httpclient.Request{
					APIFormat: string(llm.APIFormatOpenAIChatCompletion),
					Body:      []byte(`{"model":"gemini-4-hydrogen","reasoning_effort":"xhigh","messages":[]}`),
				},
			},
		},
	}
	request := &httpclient.Request{
		APIFormat: string(llm.APIFormatOpenAIChatCompletion),
		Body:      []byte(`{"model":"gemini-4-hydrogen-xhigh","messages":[]}`),
	}

	processed, err := applyPassThroughRequestBody(outbound, nil).OnOutboundRawRequest(context.Background(), request)
	require.NoError(t, err)
	require.False(t, outbound.state.PassThroughApplied)
	require.False(t, gjson.GetBytes(processed.Body, "reasoning_effort").Exists())
	require.Equal(t, "gemini-4-hydrogen-xhigh", gjson.GetBytes(processed.Body, "model").String())
}
