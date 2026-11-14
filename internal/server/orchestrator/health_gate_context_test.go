package orchestrator

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
	"github.com/looplj/axonhub/llm/transformer/anthropic"
)

type contextBlockedRetryPolicy struct {
	entered chan context.Context
	release chan struct{}
}

func (p *contextBlockedRetryPolicy) RetryPolicyOrDefault(ctx context.Context) *biz.RetryPolicy {
	p.entered <- ctx
	select {
	case <-ctx.Done():
	case <-p.release:
	}
	return &biz.RetryPolicy{}
}

func TestHealthGateAdmissionCancelsConfigurationRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider := &contextBlockedRetryPolicy{entered: make(chan context.Context, 1), release: make(chan struct{})}
	defer close(provider.release)
	candidate := healthGateSelectorCandidate(1, 1, "model")
	state := &PersistenceState{
		CurrentCandidate:        candidate,
		ChannelModelsCandidates: []*ChannelModelsCandidate{candidate},
		RetryPolicyProvider:     provider,
		RoutingPolicy:           EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
	}
	gate := biz.NewHealthGate(nil)
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, gate, biz.DefaultHealthGatePolicy())
	done := make(chan error, 1)
	go func() {
		defer func() {
			if cause := recover(); cause != nil {
				t.Errorf("admission panicked: %v", cause)
				done <- context.Canceled
			}
		}()
		_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
		done <- err
	}()
	select {
	case <-provider.entered:
	case <-time.After(time.Second):
		t.Fatal("configuration read did not begin")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled, "local cancellation must not become a channel health failure")
	case <-time.After(time.Second):
		t.Fatal("configuration read ignored request cancellation")
	}
	// Same-channel access must remain usable after the canceled read releases its lock.
	cfg := biz.ResolveHealthGateConfig(biz.DefaultHealthGatePolicy(), candidate.Channel)
	view, err := gate.Inspect(t.Context(), biz.HealthGateKey{ChannelID: 1, ActualModel: "other"}, healthGateResolver(cfg))
	require.NoError(t, err)
	require.Equal(t, biz.HealthGateStateHealthy, view.State)
	require.False(t, tracker.active)
}

type healthGateErrorStream struct {
	streams.Stream[*llm.Response]
	err error
}

func (s *healthGateErrorStream) Err() error { return s.err }

func TestHealthGateStream_CanceledContextPreservesUpstreamFailure(t *testing.T) {
	for _, tt := range []struct {
		name         string
		output       bool
		err          error
		wantOutcome  biz.HealthGateOutcome
		wantState    biz.HealthGateState
		wantFailures int
	}{
		{"upstream interruption after output", true, io.ErrUnexpectedEOF, biz.HealthGateOutcomeUnstable, biz.HealthGateStateUnstable, 0},
		{"upstream interruption before output", false, io.ErrUnexpectedEOF, biz.HealthGateOutcomeFailure, biz.HealthGateStateOpen, 1},
		{"client cancellation after output", true, context.Canceled, biz.HealthGateOutcomeNeutral, biz.HealthGateStateHealthy, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(1000, 0)
			candidate := healthGateSelectorCandidate(1, 0, "model")
			_, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{candidate}, biz.TraceStickyDisabled, nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			state := &PersistenceState{
				RoutingPolicy:    EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
				CurrentCandidate: candidate,
			}
			tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, gate, policy.HealthGateOrDefault())
			_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
			require.NoError(t, err)
			var responses []*llm.Response
			if tt.output {
				text := "hello"
				responses = append(responses, &llm.Response{Choices: []llm.Choice{{
					Delta: &llm.Message{Content: llm.MessageContent{Content: &text}},
				}}})
			}
			wrapper, err := tracker.OnOutboundLlmStream(ctx, &healthGateErrorStream{
				Stream: streams.SliceStream(responses), err: tt.err,
			})
			require.NoError(t, err)
			for wrapper.Next() {
				_ = wrapper.Current()
			}
			require.NoError(t, tracker.finalize(ctx, nil, true))
			require.True(t, tracker.active, "stream results are recorded only on close")
			cancel()
			require.NoError(t, wrapper.Close())
			require.False(t, tracker.active)
			require.Equal(t, tt.wantOutcome, tracker.lastOutcome)
			cfg := biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), candidate.Channel)
			snapshots := healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(cfg))
			require.Len(t, snapshots, 1)
			require.Equal(t, tt.wantState, snapshots[0].State)
			require.Equal(t, tt.wantFailures, snapshots[0].ConsecutiveFailures)
			if tt.wantOutcome == biz.HealthGateOutcomeNeutral {
				require.Empty(t, snapshots[0].LastError)
				require.Nil(t, snapshots[0].LastErrorAt)
			} else {
				require.Contains(t, snapshots[0].LastError, tt.err.Error())
				require.NotNil(t, snapshots[0].LastErrorAt)
			}
		})
	}
}

func TestHealthGateStream_PassThroughHTTPClientCancellationIsNeutral(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []*httpclient.StreamEvent{
			{Type: "message_start", Data: []byte(`{"type":"message_start","message":{"id":"msg_test","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1}}}`)},
			{Type: "content_block_start", Data: []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)},
			{Type: "content_block_delta", Data: []byte(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"OK"}}`)},
		} {
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data); err != nil {
				return
			}
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer upstream.Close()
	provider, err := anthropic.NewOutboundTransformer(upstream.URL, "test-key")
	require.NoError(t, err)
	gate := biz.NewHealthGate(nil)
	policy := biz.DefaultHealthGatePolicy()
	var state *PersistenceState
	var tracker *healthGateAttemptTracker
	process := anthropicPreReadPipeline(t, httpclient.NewHttpClientWithClient(upstream.Client()), func(current *PersistenceState) pipeline.Middleware {
		state = current
		state.ChannelModelsCandidates[0].Channel.Outbound = provider
		state.RoutingPolicy.LoadBalancerStrategy = biz.LoadBalancerStrategyHealthGated
		tracker = withHealthGate(&PersistentOutboundTransformer{state: state}, gate, policy)
		return tracker
	})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	result, err := process(ctx, anthropicPreReadRequest())
	require.NoError(t, err)
	require.NotNil(t, result.EventStream)
	defer result.EventStream.Close()
	require.True(t, state.PassThroughApplied)
	require.NotNil(t, state.RawStreamDone, "the raw pass-through producer must be active")
	require.NoError(t, tracker.finalize(ctx, nil, true))
	sawContent := false
	for result.EventStream.Next() {
		if event := result.EventStream.Current(); event.Type == "content_block_delta" {
			sawContent = true
			break
		}
	}
	require.True(t, sawContent, "the client must receive output before disconnecting")
	require.NoError(t, ctx.Err())
	cancel()
	select {
	case <-state.RawStreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("pass-through upstream did not stop after client cancellation")
	}
	streamErr := *state.RawStreamErrRef
	require.ErrorIs(t, streamErr, context.Canceled, "actual upstream error: %T: %v", streamErr, streamErr)
	require.False(t, IsUpstreamTransportError(streamErr), "actual upstream error: %T: %v", streamErr, streamErr)
	require.Eventually(t, func() bool {
		tracker.mu.Lock()
		defer tracker.mu.Unlock()
		return !tracker.active
	}, 5*time.Second, time.Millisecond, "the background pipeline must finish health recording")
	candidate := state.CurrentCandidate
	cfg := biz.ResolveHealthGateConfig(policy, candidate.Channel)
	snapshots := healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(cfg))
	require.Len(t, snapshots, 1)
	require.Equal(t, biz.HealthGateStateHealthy, snapshots[0].State)
	require.Zero(t, snapshots[0].ConsecutiveFailures)
	require.Empty(t, snapshots[0].LastError)
	require.Nil(t, snapshots[0].LastErrorAt)
}
