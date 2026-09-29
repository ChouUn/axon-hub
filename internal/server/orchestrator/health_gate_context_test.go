package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
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
