package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
	"github.com/looplj/axonhub/llm/transformer/openai"
	"github.com/looplj/axonhub/llm/transformer/openai/responses"
	"github.com/looplj/axonhub/llm/transformer/shared"
)

func healthGateLifecycleHTTPFixture(t *testing.T, format llm.APIFormat, passThrough bool, events []*httpclient.StreamEvent) (context.Context, *ent.Client, *ChatCompletionOrchestrator, *biz.HealthGate, *httpclient.Request) {
	t.Helper()
	ctx, client := setupTest(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data); err != nil {
				return
			}
		}
		w.(http.Flusher).Flush()
	}))
	t.Cleanup(upstream.Close)
	ch := createTestChannel(t, ctx, client)
	ch, err := ch.Update().SetBaseURL(upstream.URL).SetSettings(&objects.ChannelSettings{PassThroughBody: &passThrough}).Save(ctx)
	require.NoError(t, err)
	ctx, processor, gate := healthGateOrchestratorFixture(t, ctx, client, httpclient.NewHttpClientWithClient(upstream.Client()), ch)
	candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
	request := buildTestRequest("gpt-4", "hello", true)
	switch format {
	case llm.APIFormatOpenAIResponse:
		candidate.Channel.Outbound, err = responses.NewOutboundTransformer(upstream.URL, "test-key")
		processor.Inbound = responses.NewInboundTransformer()
		request = &httpclient.Request{Method: http.MethodPost, URL: "/v1/responses", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"model":"gpt-4","input":"hello","stream":true}`)}
	case llm.APIFormatAnthropicMessage:
		candidate.Channel.Outbound, err = anthropic.NewOutboundTransformer(upstream.URL, "test-key")
		processor.Inbound = anthropic.NewInboundTransformer()
		request = anthropicPreReadRequest()
		request.Body = []byte(strings.ReplaceAll(string(request.Body), "claude-test", "gpt-4"))
	case llm.APIFormatOpenAIChatCompletion:
	}
	require.NoError(t, err)
	candidate.APIFormat = string(format)
	processor.ChannelService.SetEnabledChannelsForTest([]*biz.Channel{candidate.Channel})
	policy := processor.SystemService.RetryPolicyOrDefault(ctx)
	policy.TraceStickyMode = biz.TraceStickyPreferPreviousChannel
	require.NoError(t, processor.SystemService.SetRetryPolicy(ctx, policy))
	ctx = contexts.WithTrace(ctx, &ent.Trace{ID: 10, ThreadID: 20})
	return ctx, client, processor, gate, request
}

func deliverHealthGateStream(t *testing.T, ctx context.Context, stream streams.Stream[*httpclient.StreamEvent]) {
	t.Helper()
	boundary, ok := stream.(streams.DeliveryBoundary)
	require.True(t, ok)
	terminal := NewStreamTerminalTracker(1)
	var output strings.Builder
	for stream.Next() {
		event := stream.Current()
		require.NoError(t, boundary.Deliver(ctx, func() (bool, error) {
			_, err := output.Write(event.Data)
			return terminal.Observe(event), err
		}))
	}
	require.NoError(t, stream.Err())
}

func healthGateResponsesEvents(terminal, status, details string) []*httpclient.StreamEvent {
	return []*httpclient.StreamEvent{
		{Type: "response.created", Data: []byte(`{"type":"response.created","response":{"id":"resp_health","model":"gpt-4","status":"in_progress","output":[]}}`)},
		{Type: "response.output_text.delta", Data: []byte(`{"type":"response.output_text.delta","item_id":"msg_health","output_index":0,"content_index":0,"delta":"hello"}`)},
		{Type: terminal, Data: []byte(`{"type":"` + terminal + `","response":{"id":"resp_health","model":"gpt-4","status":"` + status + `","output":[],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}` + details + `}}`)},
	}
}

func TestHealthGateOrchestrator_StreamIncompletePreservesOwner(t *testing.T) {
	for _, reason := range []string{"max_output_tokens", "content_filter"} {
		t.Run(reason, func(t *testing.T) {
			ctx, client, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatOpenAIResponse, false,
				healthGateResponsesEvents("response.incomplete", "incomplete", `,"incomplete_details":{"reason":"`+reason+`"}`))
			candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
			expected := setHealthGateSessionOwner(processor.RequestService, ctx, 20, 10, biz.SessionOwner{ChannelID: candidate.Channel.ID, ConsecutiveFailovers: 1})
			result, err := processor.Process(ctx, request)
			require.NoError(t, err)
			deliverHealthGateStream(t, ctx, result.ChatCompletionStream)
			require.NoError(t, result.ChatCompletionStream.Close())
			rows, err := client.Request.Query().All(ctx)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.EqualValues(t, "failed", rows[0].Status)
			cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), candidate.Channel)
			require.Equal(t, biz.HealthGateStateHealthy, healthGateInspect(t, gate, biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg)).State)
			owner, found, err := processor.RequestService.GetSessionOwner(ctx, 20, 10)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, expected, owner)
		})
	}
}

func TestHealthGateOrchestrator_CompletedBeforeDeliveryCancellation(t *testing.T) {
	ctx, client, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatOpenAIResponse, false,
		healthGateResponsesEvents("response.completed", "completed", ""))
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result, err := processor.Process(requestCtx, request)
	require.NoError(t, err)
	boundary := result.ChatCompletionStream.(streams.DeliveryBoundary)
	terminal := NewStreamTerminalTracker(1)
	sawTerminal := false
	for result.ChatCompletionStream.Next() {
		event := result.ChatCompletionStream.Current()
		if terminal.Observe(event) {
			sawTerminal = true
			cancel()
			require.ErrorIs(t, boundary.Deliver(requestCtx, func() (bool, error) {
				t.Error("canceled delivery must not invoke the writer")
				return true, nil
			}), context.Canceled)
			break
		}
		require.NoError(t, boundary.Deliver(requestCtx, func() (bool, error) { return false, nil }))
	}
	require.True(t, sawTerminal)
	require.NoError(t, result.ChatCompletionStream.Close())
	rows, err := client.Request.Query().All(ctx)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotEqualValues(t, "completed", rows[0].Status)
	candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
	cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), candidate.Channel)
	require.Equal(t, biz.HealthGateStateHealthy, healthGateInspect(t, gate, biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg)).State)
	_, found, err := processor.RequestService.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.False(t, found)
}

type healthGatePauseAfterOutput struct {
	streams.Stream[*llm.Response]
	paused  chan struct{}
	release chan struct{}
	once    sync.Once
	next    bool
}

func (s *healthGatePauseAfterOutput) Next() bool {
	if s.next {
		close(s.paused)
		<-s.release
		s.next = false
	}
	return s.Stream.Next()
}

func (s *healthGatePauseAfterOutput) Current() *llm.Response {
	response := s.Stream.Current()
	if hasMeaningfulStreamOutput(response) {
		s.next = true
	}
	return response
}

func (s *healthGatePauseAfterOutput) Close() error {
	s.once.Do(func() { close(s.release) })
	return s.Stream.Close()
}

type healthGatePauseOutputMiddleware struct {
	pipeline.DummyMiddleware
	paused  chan struct{}
	release chan struct{}
}

func (m *healthGatePauseOutputMiddleware) OnOutboundLlmStream(_ context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	return &healthGatePauseAfterOutput{Stream: stream, paused: m.paused, release: m.release}, nil
}

func TestHealthGateStream_PassThroughFailedTerminalAfterClientCancellation(t *testing.T) {
	ctx, _, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatOpenAIResponse, true,
		healthGateResponsesEvents("response.failed", "failed", `,"error":{"message":"provider failed"}`))
	paused, release := make(chan struct{}), make(chan struct{})
	processor.Middlewares = append(processor.Middlewares, &healthGatePauseOutputMiddleware{paused: paused, release: release})
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result, err := processor.Process(requestCtx, request)
	require.NoError(t, err)
	defer result.ChatCompletionStream.Close()
	boundary := result.ChatCompletionStream.(streams.DeliveryBoundary)
	sawFailure := false
	for result.ChatCompletionStream.Next() {
		event := result.ChatCompletionStream.Current()
		if event.Type == "response.failed" {
			sawFailure = true
			break
		}
		require.NoError(t, boundary.Deliver(requestCtx, func() (bool, error) { return false, nil }))
	}
	require.True(t, sawFailure)
	select {
	case <-paused:
	case <-time.After(5 * time.Second):
		t.Fatal("normalized drain did not pause")
	}
	response := result.ChatCompletionStream.(*healthGateResponseStream)
	require.Equal(t, streamTerminalNone, response.tracker.outbound.state.OutboundStreamTerminal)
	cancel()
	require.NoError(t, result.ChatCompletionStream.Close())
	require.Contains(t, []streamTerminalState{streamTerminalNone, streamTerminalFailed}, response.tracker.outbound.state.OutboundStreamTerminal)
	require.Equal(t, streamTerminalFailed, response.providerTerminal)
	candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
	cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), candidate.Channel)
	snapshot := healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(cfg))
	require.Len(t, snapshot, 1)
	require.Equal(t, biz.HealthGateStateUnstable, snapshot[0].State)
	require.Zero(t, snapshot[0].ConsecutiveFailures)
	_, found, err := processor.RequestService.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.False(t, found)
}

func TestHealthGateOrchestrator_AggregatedGenerationIsHealthyWithoutWireTerminal(t *testing.T) {
	events := anthropicPreReadEvents(0, false)
	ctx, _, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatAnthropicMessage, false, events[:len(events)-1])
	result, err := processor.Process(ctx, request)
	require.NoError(t, err)
	deliverHealthGateStream(t, ctx, result.ChatCompletionStream)
	require.NoError(t, result.ChatCompletionStream.Close())
	response := result.ChatCompletionStream.(*healthGateResponseStream)
	state := response.tracker.outbound.state
	require.Equal(t, streamTerminalNone, state.OutboundStreamTerminal)
	require.False(t, state.CleanEOFCompletionEvidence)
	require.True(t, state.Perf.RequestCompleted)
	require.True(t, state.Perf.Success)
	candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
	cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), candidate.Channel)
	require.Equal(t, biz.HealthGateStateHealthy, healthGateInspect(t, gate, biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg)).State)
}

func TestHealthGateOutcome_LifecycleCloseDoesNotCancelClient(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("downstream_canceled=%t", canceled), func(t *testing.T) {
			downstream, cancel := context.WithCancel(t.Context())
			defer cancel()
			owner, ctx := newStreamLifecycle(downstream)
			if canceled {
				cancel()
			}
			owner.Close()
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			expected := biz.HealthGateOutcomeFailure
			if canceled {
				expected = biz.HealthGateOutcomeNeutral
			}
			require.Equal(t, expected, classifyHealthGateOutcome(ctx, healthGateServerError()))
			require.Equal(t, expected, classifyHealthGateOutcome(ctx, pipeline.ErrNonStreamResponseTimeout))
		})
	}
}

func TestHealthGateOrchestrator_NonStreamClientCancellationIsNeutral(t *testing.T) {
	ctx, client := setupTest(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer upstream.Close()
	defer close(release)
	ch := createTestChannel(t, ctx, client)
	ch, err := ch.Update().SetBaseURL(upstream.URL).Save(ctx)
	require.NoError(t, err)
	ctx, processor, gate := healthGateOrchestratorFixture(t, ctx, client, httpclient.NewHttpClientWithClient(upstream.Client()), ch)
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := processor.Process(requestCtx, buildTestRequest("gpt-4", "hello", false))
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("upstream request did not begin")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("canceled request did not return")
	}
	cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), &biz.Channel{Channel: ch})
	snapshot := healthGateSnapshot(t, gate, ch.ID, healthGateResolver(cfg))
	require.Len(t, snapshot, 1)
	require.Equal(t, biz.HealthGateStateHealthy, snapshot[0].State)
	require.Zero(t, snapshot[0].ConsecutiveFailures)
	require.Empty(t, snapshot[0].LastError)
}

func TestHealthGateOutcome_ResponsesRecoveryRetainsClassification(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want biz.HealthGateOutcome
	}{
		{"local", shared.ErrResponsesContinuationUnavailable, biz.HealthGateOutcomeNeutral},
		{"constructed", shared.ResponsesRecoveryError(), biz.HealthGateOutcomeNeutral},
		{"wire400", &httpclient.Error{StatusCode: 400, Body: []byte(`{"error":{"code":"previous_response_not_found"}}`)}, biz.HealthGateOutcomeNeutral},
		{"wire404", &httpclient.Error{StatusCode: 404, Body: []byte(`{"error":{"code":"previous_response_not_found"}}`)}, biz.HealthGateOutcomeNeutral},
		{"normalized400", &llm.ResponseError{StatusCode: 400, Detail: llm.ErrorDetail{Code: "previous_response_not_found"}}, biz.HealthGateOutcomeNeutral},
		{"wire500", &httpclient.Error{StatusCode: 500, Body: []byte(`{"error":{"code":"previous_response_not_found"}}`)}, biz.HealthGateOutcomeFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.True(t, shared.IsResponsesRecoveryError(test.err))
			require.Equal(t, test.want, classifyHealthGateOutcome(t.Context(), test.err))
		})
	}
}

type healthGateRawErrorStream struct {
	streams.Stream[*httpclient.StreamEvent]
	err error
}

func (s *healthGateRawErrorStream) Err() error { return s.err }

func TestHealthGateStream_FinalizationObservesTailEvidence(t *testing.T) {
	for _, test := range []struct {
		name  string
		event *httpclient.StreamEvent
		err   error
		want  biz.HealthGateOutcome
	}{
		{"completed", healthGateResponsesEvents("response.completed", "completed", "")[2], nil, biz.HealthGateOutcomeSuccess},
		{"incomplete", healthGateResponsesEvents("response.incomplete", "incomplete", "")[2], nil, biz.HealthGateOutcomeSuccess},
		{"failed", healthGateResponsesEvents("response.failed", "failed", "")[2], nil, biz.HealthGateOutcomeFailure},
		{"transport", nil, io.ErrUnexpectedEOF, biz.HealthGateOutcomeFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := healthGateSelectorCandidate(1, 0, "model")
			state := &PersistenceState{CurrentCandidate: candidate, RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}}
			tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, biz.NewHealthGate(nil), biz.DefaultHealthGatePolicy())
			owner, ctx := newStreamLifecycle(t.Context())
			defer owner.Close()
			_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
			require.NoError(t, err)
			var events []*httpclient.StreamEvent
			if test.event != nil {
				events = append(events, test.event)
			}
			source := &healthGateRawErrorStream{Stream: streams.SliceStream(events), err: test.err}
			_, err = tracker.OnOutboundRawStream(ctx, source)
			require.NoError(t, err)
			provider, err := responses.NewOutboundTransformer("https://example.test", "test-key")
			require.NoError(t, err)
			raw := NewOutboundPersistentStream(ctx, source, nil, nil, nil, nil, provider, nil, state)
			normalized, err := provider.TransformStream(ctx, nil, raw)
			require.NoError(t, err)
			normalized, err = tracker.OnOutboundLlmStream(ctx, normalized)
			require.NoError(t, err)
			inbound := openai.NewInboundTransformer()
			converted, err := inbound.TransformStream(ctx, normalized)
			require.NoError(t, err)
			persistent := NewInboundPersistentStream(ctx, converted, nil, nil, nil, inbound, nil, state)
			require.NoError(t, tracker.finalize(ctx, nil, true))
			response := tracker.wrapResponseStream(ctx, persistent)
			owner.AcknowledgeDelivery()
			require.Equal(t, streamTerminalNone, state.OutboundStreamTerminal)
			require.NoError(t, response.Close())
			require.Equal(t, test.want, tracker.lastOutcome)
			require.False(t, tracker.active)
		})
	}
}

func TestHealthGateStream_DeliveredTimeoutWaitsForConvertedDrain(t *testing.T) {
	candidate := healthGateSelectorCandidate(1, 0, "model")
	state := &PersistenceState{CurrentCandidate: candidate, RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}}
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, biz.NewHealthGate(nil), biz.DefaultHealthGatePolicy())
	owner, ctx := newStreamLifecycle(t.Context())
	defer owner.Close()
	_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	_, err = tracker.OnOutboundLlmStream(ctx, streams.SliceStream([]*llm.Response{}))
	require.NoError(t, err)
	require.NoError(t, tracker.finalize(ctx, nil, true))
	drain := make(chan struct{})
	state.rawStreamDrainDone = drain
	closed := make(chan struct{})
	inner := withPassThroughDeliveryGateAndDrain(streams.SliceStream([]*httpclient.StreamEvent{}), owner, func() error {
		owner.Close()
		close(closed)
		return nil
	}, drain)
	owner.AcknowledgeDelivery()
	response := tracker.wrapResponseStream(ctx, inner)
	done := make(chan error, 1)
	go func() { done <- response.Close() }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("delivery gate did not take its timeout close path")
	}
	tracker.mu.Lock()
	require.True(t, tracker.active)
	tracker.mu.Unlock()
	select {
	case err := <-done:
		t.Fatalf("health finalization ran before converted drain exited: %v", err)
	default:
	}
	state.OutboundStreamTerminal = streamTerminalCompleted
	close(drain)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("health finalization did not finish after drain exit")
	}
	require.Equal(t, biz.HealthGateOutcomeSuccess, tracker.lastOutcome)
	require.NoError(t, response.Close())
}

func TestHealthGateStream_DownstreamWriteFailureDoesNotBecomeProviderFailure(t *testing.T) {
	ctx := t.Context()
	candidate := healthGateSelectorCandidate(1, 0, "model")
	state := &PersistenceState{CurrentCandidate: candidate, RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}}
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, biz.NewHealthGate(nil), biz.DefaultHealthGatePolicy())
	_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	require.NoError(t, tracker.finalize(ctx, nil, true))
	response := tracker.wrapResponseStream(ctx, streams.SliceStream([]*httpclient.StreamEvent{}))
	writeErr := errors.New("downstream write failed")
	require.ErrorIs(t, response.(streams.DeliveryBoundary).Deliver(ctx, func() (bool, error) { return false, writeErr }), writeErr)
	require.NoError(t, ctx.Err())
	require.NoError(t, response.Close())
	require.Equal(t, biz.HealthGateOutcomeNeutral, tracker.lastOutcome)
}

func TestHealthGateStream_RawTerminalOnlyFillsMissingUpstreamTerminal(t *testing.T) {
	for _, test := range []struct {
		name     string
		upstream streamTerminalState
		raw      *httpclient.StreamEvent
		want     biz.HealthGateOutcome
	}{
		{"missing upstream", streamTerminalNone, healthGateResponsesEvents("response.failed", "failed", "")[2], biz.HealthGateOutcomeFailure},
		{"upstream completed", streamTerminalCompleted, healthGateResponsesEvents("response.failed", "failed", "")[2], biz.HealthGateOutcomeSuccess},
		{"upstream failed", streamTerminalFailed, healthGateResponsesEvents("response.completed", "completed", "")[2], biz.HealthGateOutcomeFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := healthGateSelectorCandidate(1, 0, "model")
			state := &PersistenceState{CurrentCandidate: candidate, RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}, OutboundStreamTerminal: test.upstream}
			drain := make(chan struct{})
			close(drain)
			state.rawStreamDrainDone = drain
			tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, biz.NewHealthGate(nil), biz.DefaultHealthGatePolicy())
			_, err := tracker.OnOutboundRawRequest(t.Context(), &httpclient.Request{})
			require.NoError(t, err)
			require.NoError(t, tracker.finalize(t.Context(), nil, true))
			response := tracker.wrapResponseStream(t.Context(), streams.SliceStream([]*httpclient.StreamEvent{test.raw}))
			require.True(t, response.Next())
			response.Current()
			require.NoError(t, response.Close())
			require.Equal(t, test.want, tracker.lastOutcome)
			require.Equal(t, test.upstream, state.OutboundStreamTerminal)
		})
	}
}

func TestHealthGateStream_LaterCancellationDoesNotOverwriteObservedTransportError(t *testing.T) {
	for _, meaningful := range []bool{false, true} {
		t.Run(fmt.Sprintf("meaningful=%t", meaningful), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			candidate := healthGateSelectorCandidate(1, 0, "model")
			state := &PersistenceState{CurrentCandidate: candidate, RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}}
			tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, biz.NewHealthGate(nil), biz.DefaultHealthGatePolicy())
			_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
			require.NoError(t, err)
			_, err = tracker.OnOutboundRawStream(ctx, streams.SliceStream([]*httpclient.StreamEvent{}))
			require.NoError(t, err)
			var events []*llm.Response
			if meaningful {
				text := "hello"
				events = append(events, &llm.Response{Choices: []llm.Choice{{Delta: &llm.Message{Content: llm.MessageContent{Content: &text}}}}})
			}
			observed, err := tracker.OnOutboundLlmStream(ctx, &healthGateErrorStream{Stream: streams.SliceStream(events), err: context.Canceled})
			require.NoError(t, err)
			for observed.Next() {
				observed.Current()
			}
			require.NoError(t, tracker.finalize(ctx, nil, true))
			response := tracker.wrapResponseStream(ctx, &healthGateRawErrorStream{Stream: streams.SliceStream([]*httpclient.StreamEvent{}), err: io.ErrUnexpectedEOF})
			cancel()
			require.NoError(t, response.Close())
			expected := biz.HealthGateOutcomeFailure
			if meaningful {
				expected = biz.HealthGateOutcomeUnstable
			}
			require.Equal(t, expected, tracker.lastOutcome)
			require.Contains(t, tracker.lastInfo.Message, io.ErrUnexpectedEOF.Error())
		})
	}
}

type healthGateCancelAfterRawError struct {
	pipeline.DummyMiddleware
	cancel context.CancelFunc
}

func (m *healthGateCancelAfterRawError) OnOutboundRawError(context.Context, error) {
	m.cancel()
}

func TestHealthGateOrchestrator_PreCommitPassThroughFailureSurvivesCancellation(t *testing.T) {
	for _, failedTerminal := range []bool{false, true} {
		t.Run(fmt.Sprintf("failed_terminal=%t", failedTerminal), func(t *testing.T) {
			ctx, client := setupTest(t)
			ch := createTestChannel(t, ctx, client)
			passThrough := true
			ch.Settings = &objects.ChannelSettings{PassThroughBody: &passThrough}
			events := []*httpclient.StreamEvent{{Data: []byte(`{"id":"chatcmpl-health","object":"chat.completion.chunk","model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"}}]}`)}}
			var sourceErr error = io.ErrUnexpectedEOF
			if failedTerminal {
				terminalEvents := healthGateResponsesEvents("response.failed", "failed", `,"error":{"message":"provider failed"}`)
				events = []*httpclient.StreamEvent{terminalEvents[0], terminalEvents[2]}
				sourceErr = nil
			}
			ctx, processor, gate := healthGateOrchestratorFixture(t, ctx, client, &mockExecutorWithErrorStream{events: events, streamErr: sourceErr}, ch)
			policy := processor.SystemService.RetryPolicyOrDefault(ctx)
			policy.StreamFirstEventTimeoutSeconds = 2
			policy.EmptyResponseDetection = true
			require.NoError(t, processor.SystemService.SetRetryPolicy(ctx, policy))
			request := buildTestRequest("gpt-4", "hello", true)
			if failedTerminal {
				candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
				var err error
				candidate.Channel.Outbound, err = responses.NewOutboundTransformer("https://example.test", "test-key")
				require.NoError(t, err)
				candidate.APIFormat = string(llm.APIFormatOpenAIResponse)
				processor.Inbound = responses.NewInboundTransformer()
				request = &httpclient.Request{Method: http.MethodPost, URL: "/v1/responses", Headers: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"model":"gpt-4","input":"hello","stream":true}`)}
			}
			requestCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			processor.Middlewares = append(processor.Middlewares, &healthGateCancelAfterRawError{cancel: cancel})
			result, err := processor.Process(requestCtx, request)
			require.Error(t, err)
			require.Nil(t, result.ChatCompletionStream)
			require.ErrorIs(t, requestCtx.Err(), context.Canceled)
			cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), &biz.Channel{Channel: ch})
			snapshot := healthGateSnapshot(t, gate, ch.ID, healthGateResolver(cfg))
			require.Len(t, snapshot, 1)
			if failedTerminal {
				require.Equal(t, 0, snapshot[0].ConsecutiveFailures)
				require.Equal(t, biz.HealthGateStateHealthy, snapshot[0].State)
			} else {
				require.Equal(t, 1, snapshot[0].ConsecutiveFailures)
				require.Equal(t, biz.HealthGateStateUnstable, snapshot[0].State)
			}
		})
	}
}

func TestHealthGateOrchestrator_EmptyResponseDetectionPrecedesProviderTerminal(t *testing.T) {
	for _, status := range []string{"completed", "incomplete", "canceled"} {
		t.Run(status, func(t *testing.T) {
			kind := "response." + status
			if status == "canceled" {
				kind = "response.cancelled"
			}
			events := healthGateResponsesEvents(kind, status, "")
			ctx, _, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatOpenAIResponse, false, []*httpclient.StreamEvent{events[0], events[2]})
			policy := processor.SystemService.RetryPolicyOrDefault(ctx)
			policy.EmptyResponseDetection = true
			require.NoError(t, processor.SystemService.SetRetryPolicy(ctx, policy))
			result, err := processor.Process(ctx, request)
			require.ErrorIs(t, err, pipeline.ErrEmptyResponse)
			require.Nil(t, result.ChatCompletionStream)
			candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
			cfg := biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), candidate.Channel)
			snapshot := healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(cfg))
			require.Len(t, snapshot, 1)
			require.Equal(t, 1, snapshot[0].ConsecutiveFailures)
			require.Equal(t, biz.HealthGateStateUnstable, snapshot[0].State)
			if status == "completed" {
				require.Eventually(t, func() bool {
					metrics, err := processor.ChannelService.GetChannelMetrics(ctx, candidate.Channel.ID)
					return err == nil && metrics.SuccessCount+metrics.FailureCount == 2
				}, 5*time.Second, time.Millisecond)
				metrics, err := processor.ChannelService.GetChannelMetrics(ctx, candidate.Channel.ID)
				require.NoError(t, err)
				require.EqualValues(t, 1, metrics.SuccessCount, "queued completion must retain its success snapshot")
				require.EqualValues(t, 1, metrics.FailureCount, "the later empty-response callback must enqueue a separate failure")
			}
		})
	}
}

func TestHealthGateStream_SameModelRetryClearsPreviousTransportEvidence(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	streaming := true
	candidate := healthGateSelectorCandidate(1, 0, "model")
	state := &PersistenceState{
		CurrentCandidate: candidate, PassThroughApplied: true,
		RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
		LlmRequest:    &llm.Request{Stream: &streaming},
	}
	gate := biz.NewHealthGate(nil)
	policy := biz.DefaultHealthGatePolicy()
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, gate, policy)
	_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	_, err = tracker.OnOutboundRawStream(ctx, &healthGateRawErrorStream{Stream: streams.SliceStream([]*httpclient.StreamEvent{}), err: io.ErrUnexpectedEOF})
	require.NoError(t, err)
	tracker.OnOutboundRawError(ctx, io.ErrUnexpectedEOF)
	_, err = tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	cancel()
	tracker.OnOutboundRawError(ctx, context.Canceled)
	require.ErrorIs(t, tracker.finalize(ctx, context.Canceled, false), context.Canceled)
	require.Equal(t, biz.HealthGateOutcomeNeutral, tracker.lastOutcome)
	cfg := biz.ResolveHealthGateConfig(policy, candidate.Channel)
	snapshot := healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(cfg))
	require.Len(t, snapshot, 1)
	require.Zero(t, snapshot[0].ConsecutiveFailures)
	require.Equal(t, biz.HealthGateStateHealthy, snapshot[0].State)
}

func TestHealthGateOrchestrator_AggregatedGenerationWithoutDeliveryIsHealthy(t *testing.T) {
	events := anthropicPreReadEvents(0, false)
	ctx, _, processor, gate, request := healthGateLifecycleHTTPFixture(t, llm.APIFormatAnthropicMessage, false, events[:len(events)-1])
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	result, err := processor.Process(requestCtx, request)
	require.NoError(t, err)
	for result.ChatCompletionStream.Next() {
		result.ChatCompletionStream.Current()
	}
	require.NoError(t, result.ChatCompletionStream.Err())
	cancel()
	require.NoError(t, result.ChatCompletionStream.Close())
	response := result.ChatCompletionStream.(*healthGateResponseStream)
	state := response.tracker.outbound.state
	require.Equal(t, streamTerminalNone, state.OutboundStreamTerminal)
	require.False(t, state.CleanEOFCompletionEvidence)
	require.True(t, state.Perf.RequestCompleted)
	require.True(t, state.Perf.Success)
	require.False(t, state.StreamCompleted)
	candidate := processor.channelSelector.(*staticChannelSelector).candidates[0]
	cfg := biz.ResolveHealthGateConfig(processor.SystemService.RetryPolicyOrDefault(ctx).HealthGateOrDefault(), candidate.Channel)
	require.Equal(t, biz.HealthGateStateHealthy, healthGateInspect(t, gate, biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: "gpt-4"}, healthGateResolver(cfg)).State)
	_, found, err := processor.RequestService.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.False(t, found)
}
