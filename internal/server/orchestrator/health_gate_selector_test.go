package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/providerquotastatus"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
)

type healthGateLegacySessionProvider struct {
	PreviousChannelProvider
}

func (p healthGateLegacySessionProvider) GetSessionOwner(ctx context.Context, threadID, traceID int) (biz.SessionOwner, bool, error) {
	id := 0
	var err error
	if threadID != 0 {
		id, err = p.GetPreviousChannelIDByThread(ctx, threadID)
	} else if traceID != 0 {
		id, err = p.GetPreviousChannelID(ctx, traceID)
	}
	return biz.SessionOwner{ChannelID: id}, id != 0, err
}

func healthGateResolver(cfg biz.HealthGateConfig) biz.HealthGateConfigResolver {
	return func() (biz.HealthGateConfig, bool) { return cfg, true }
}

func healthGateSelectorFixture(now *time.Time, candidates []*ChannelModelsCandidate, stickyMode biz.TraceStickyMode, previous PreviousChannelProvider) (*LoadBalancedSelector, *biz.HealthGate, *biz.RetryPolicy) {
	gate := biz.NewHealthGate(func() time.Time { return *now })
	policy := &biz.RetryPolicy{
		Enabled: true, MaxChannelRetries: 2, LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated,
		TraceStickyMode: stickyMode, HealthGate: &biz.HealthGatePolicy{FailureThreshold: 1, OpenDurationSeconds: 10, MaxOpenDurationSeconds: 60, ProbeSuccessThreshold: 2, UnstableWindowSeconds: 30},
	}
	provider := &mockRetryPolicyProvider{policy: policy}
	if previous != nil {
		previous = healthGateLegacySessionProvider{previous}
	}
	selector := WithTraceStickyLoadBalancedSelector(&staticChannelSelector{candidates: candidates}, NewLoadBalancer(nil, nil).WithHealthGate(gate), policy, previous)
	selector.healthGatePolicyProvider = provider
	return selector, gate, policy
}

func openHealthGateModel(t *testing.T, gate *biz.HealthGate, policy *biz.RetryPolicy, candidate *ChannelModelsCandidate, model string) {
	t.Helper()
	key := biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: model}
	cfg := biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), candidate.Channel)
	ticket, ok := healthGateBegin(t, gate, key, healthGateResolver(cfg), true, false)
	require.True(t, ok)
	healthGateFinish(t, gate, ticket, healthGateResolver(cfg), biz.HealthGateOutcomeFailure, biz.HealthGateErrorInfo{StatusCode: 500})
}

func healthGateSelectorCandidate(channelID, priority int, models ...string) *ChannelModelsCandidate {
	candidate := stickyTestCandidate(channelID, priority)
	candidate.Models = make([]biz.ChannelModelEntry, 0, len(models))
	candidate.modelAPIFormats = make([]string, 0, len(models))
	for _, model := range models {
		candidate.Models = append(candidate.Models, biz.ChannelModelEntry{RequestModel: "alias", ActualModel: model})
		candidate.modelAPIFormats = append(candidate.modelAPIFormats, "format-"+model)
	}
	candidate.APIFormat = candidate.modelAPIFormats[0]
	return candidate
}

func TestHealthGateSelector_FiltersModelsWithoutMutatingCandidates(t *testing.T) {
	now := time.Unix(1000, 0)
	first := healthGateSelectorCandidate(1, 0, "broken", "working")
	second := healthGateSelectorCandidate(2, 1, "broken")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{first, second}, biz.TraceStickyDisabled, nil)
	openHealthGateModel(t, gate, policy, first, "broken")
	openHealthGateModel(t, gate, policy, second, "broken")

	result, err := selector.Select(context.Background(), &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, "working", result[0].Models[0].ActualModel)
	require.Equal(t, []string{"format-working"}, result[0].modelAPIFormats)
	require.Equal(t, "format-working", result[0].APIFormat)
	require.Len(t, first.Models, 2)
	require.Equal(t, []string{"format-broken", "format-working"}, first.modelAPIFormats)
	require.Equal(t, "format-broken", first.APIFormat)
	require.NotSame(t, first, result[0])
}

// A Guardian review prefers its parent's channel only when that channel passes
// the gate. An open parent falls back to the healthy candidate.
func TestHealthGateSelector_GuardianReviewPrefersHealthyParent(t *testing.T) {
	for _, parentOpen := range []bool{false, true} {
		t.Run(fmt.Sprintf("parentOpen=%v", parentOpen), func(t *testing.T) {
			now := time.Unix(1000, 0)
			other := healthGateSelectorCandidate(1, 0, "other")
			parent := healthGateSelectorCandidate(2, 1, "parent")
			selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{other, parent}, biz.TraceStickyDisabled, nil)
			if parentOpen {
				openHealthGateModel(t, gate, policy, parent, "parent")
			}
			ctx := WithSafeguardsParent(context.Background())
			link := ctx.Value(guardianLinkageKey{}).(guardianLinkage)
			link.parent.attempt = &guardianParentAttempt{channelID: parent.Channel.ID, responseID: "resp_parent"}
			ctx = WithSafeguardsReview(ctx)

			result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
			require.NoError(t, err)
			require.NotEmpty(t, result)
			if parentOpen {
				require.Equal(t, []int{other.Channel.ID}, quotaRoutingIDs(result))
				return
			}
			require.Equal(t, []int{parent.Channel.ID, other.Channel.ID}, quotaRoutingIDs(result))
			require.False(t, result[0].TraceSticky)
		})
	}
}

func TestHealthGateSelector_QuotaFilteringPrecedesHealthGate(t *testing.T) {
	now := time.Unix(1000, 0)
	exhausted := healthGateSelectorCandidate(1, 0, "exhausted")
	available := healthGateSelectorCandidate(2, 1, "available")
	previous := &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: exhausted.Channel.ID}}
	selector, gate, policy := healthGateSelectorFixture(&now,
		[]*ChannelModelsCandidate{exhausted, available}, biz.TraceStickyPreferPreviousChannel, previous)
	openHealthGateModel(t, gate, policy, exhausted, "exhausted")
	openHealthGateModel(t, gate, policy, available, "available")
	selector.quotaGate = NewQuotaRoutingGate(&mockQuotaStatusProvider{statuses: map[int]*biz.QuotaChannelStatus{
		exhausted.Channel.ID: quotaExhaustedStatus(),
		available.Channel.ID: quotaRoutingStatus(providerquotastatus.StatusAvailable),
	}}, biz.QuotaRoutingSettings{DefaultMode: objects.QuotaRoutingModeRemoveOnExhausted})
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10})

	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, available.Channel.ID, result[0].Channel.ID)
	require.True(t, result[0].healthGate.lastResort)
	require.Equal(t, 1, selector.quotaGate.DroppedCount())
	decision := result[0].healthGate.decision
	require.Equal(t, []*ChannelModelsCandidate{available}, decision.candidates)
	require.Empty(t, decision.record.Skipped, "quota-exhausted candidates never reach health inspection")
	require.False(t, decision.ownerOpen, "a quota-filtered owner is absent rather than health-gated")
}

func TestHealthGateSelector_AllQuotaExhaustedReturnsEmpty(t *testing.T) {
	now := time.Unix(1000, 0)
	candidate := healthGateSelectorCandidate(1, 0, "model")
	selector, _, _ := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{candidate}, biz.TraceStickyDisabled, nil)
	selector.quotaGate = NewQuotaRoutingGate(&mockQuotaStatusProvider{statuses: map[int]*biz.QuotaChannelStatus{
		candidate.Channel.ID: quotaExhaustedStatus(),
	}}, biz.QuotaRoutingSettings{DefaultMode: objects.QuotaRoutingModeRemoveOnExhausted})

	result, err := selector.Select(context.Background(), &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Empty(t, result)
	require.Equal(t, 1, selector.quotaGate.DroppedCount())
}

func TestHealthGateSelector_QuotaBackpressureFallbacks(t *testing.T) {
	tests := []struct {
		name           string
		retriesEnabled bool
		tripOrder      []int
		wantIDs        []int
		wantSkipped    []int
		wantLastResort bool
	}{
		{name: "healthy_preferred_then_reserved", retriesEnabled: true, wantIDs: []int{1, 2}},
		{name: "tripped_reserved_is_skipped", retriesEnabled: true, tripOrder: []int{2}, wantIDs: []int{1}, wantSkipped: []int{2}},
		{name: "healthy_reserved_is_initial_attempt", retriesEnabled: true, tripOrder: []int{1}, wantIDs: []int{2}, wantSkipped: []int{1}},
		{name: "disabled_retries_exclude_reserved_fallback", wantIDs: []int{1}},
		{name: "disabled_retries_allow_reserved_initial_attempt", tripOrder: []int{1}, wantIDs: []int{2}, wantSkipped: []int{1}},
		{name: "earliest_last_trial_is_reserved", retriesEnabled: true, tripOrder: []int{2, 1}, wantIDs: []int{2}, wantSkipped: []int{1}, wantLastResort: true},
		{name: "earliest_last_trial_is_preferred", retriesEnabled: true, tripOrder: []int{1, 2}, wantIDs: []int{1}, wantSkipped: []int{2}, wantLastResort: true},
	}
	for _, mode := range []biz.TraceStickyMode{biz.TraceStickyDisabled, biz.TraceStickyPreferPreviousChannel} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s/%s", mode, tt.name), func(t *testing.T) {
				now := time.Unix(1000, 0)
				open := healthGateSelectorCandidate(1, 1, "open")
				pressured := healthGateSelectorCandidate(2, 0, "pressured")
				open.Channel.Name, pressured.Channel.Name = "open-channel", "pressured-channel"
				byID := map[int]*ChannelModelsCandidate{1: open, 2: pressured}
				previous := &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: open.Channel.ID}}
				selector, gate, policy := healthGateSelectorFixture(&now,
					[]*ChannelModelsCandidate{pressured, open}, mode, previous)
				policy.Enabled, policy.MaxChannelRetries = tt.retriesEnabled, 1
				tracker := &mockSelectionTracker{}
				selector.loadBalancer.selectionTracker = tracker
				selector.quotaGate = NewQuotaRoutingGate(&mockQuotaStatusProvider{statuses: map[int]*biz.QuotaChannelStatus{
					open.Channel.ID:      quotaRoutingStatus(providerquotastatus.StatusAvailable),
					pressured.Channel.ID: quotaStickyOnlyStatus(),
				}}, biz.QuotaRoutingSettings{DefaultMode: objects.QuotaRoutingModeBackpressure})
				tripped := make(map[int]bool)
				for _, id := range tt.tripOrder {
					candidate := byID[id]
					openHealthGateModel(t, gate, policy, candidate, candidate.Models[0].ActualModel)
					tripped[id] = true
					now = now.Add(time.Second)
				}
				if tt.wantIDs[0] == pressured.Channel.ID {
					pressured.TraceSticky = true
				}
				ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10})

				result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
				require.NoError(t, err)
				require.Equal(t, tt.wantIDs, quotaRoutingIDs(result))
				require.Equal(t, mode == biz.TraceStickyPreferPreviousChannel && tt.wantIDs[0] == open.Channel.ID && !tt.wantLastResort, result[0].TraceSticky)
				decision := result[0].healthGate.decision
				require.Equal(t, tt.wantLastResort, decision.record.LastResort)
				require.Equal(t, []*ChannelModelsCandidate{open, pressured}, decision.candidates)
				require.Equal(t, "pressured-channel", healthGateOwnerName(ctx, pressured.Channel.ID, decision.candidates, nil, nil))
				require.Equal(t, tripped[pressured.Channel.ID], decision.ownerGatedAtCompletion(ctx, gate, pressured.Channel.ID))
				var skippedIDs []int
				for _, skipped := range decision.record.Skipped {
					skippedIDs = append(skippedIDs, skipped.ChannelID)
					require.Equal(t, string(biz.HealthGateStateOpen), skipped.State)
				}
				require.Equal(t, tt.wantSkipped, skippedIDs)
				for _, candidate := range result {
					require.NotSame(t, byID[candidate.Channel.ID], candidate)
					require.Same(t, decision, candidate.healthGate.decision)
					require.Equal(t, tt.wantLastResort, candidate.healthGate.lastResort)
				}
				for id := range byID {
					wantSelections := 0
					if !tt.wantLastResort && id == tt.wantIDs[0] {
						wantSelections = 1
					}
					require.Equal(t, wantSelections, tracker.selections[id])
				}
				if mode == biz.TraceStickyPreferPreviousChannel {
					require.Equal(t, open.Channel.ID, decision.record.Owner.ChannelID)
					require.Equal(t, open.Channel.Name, decision.record.Owner.ChannelName)
					require.Equal(t, tripped[open.Channel.ID], decision.ownerOpen)
				}
			})
		}
	}
}

func TestHealthGateSelector_StickyQuotaFallbackFollowsAllPreferred(t *testing.T) {
	now := time.Unix(1000, 0)
	owner := healthGateSelectorCandidate(1, 2, "owner")
	pressured := healthGateSelectorCandidate(2, 0, "pressured")
	other := healthGateSelectorCandidate(3, 1, "other")
	previous := &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: owner.Channel.ID}}
	selector, _, _ := healthGateSelectorFixture(&now,
		[]*ChannelModelsCandidate{pressured, other, owner}, biz.TraceStickyPreferPreviousChannel, previous)
	selector.quotaGate = NewQuotaRoutingGate(&mockQuotaStatusProvider{statuses: map[int]*biz.QuotaChannelStatus{
		owner.Channel.ID:     quotaRoutingStatus(providerquotastatus.StatusAvailable),
		pressured.Channel.ID: quotaStickyOnlyStatus(),
		other.Channel.ID:     quotaRoutingStatus(providerquotastatus.StatusAvailable),
	}}, biz.QuotaRoutingSettings{DefaultMode: objects.QuotaRoutingModeBackpressure})
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10})

	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Equal(t, []int{owner.Channel.ID, other.Channel.ID, pressured.Channel.ID}, quotaRoutingIDs(result))
	require.True(t, result[0].TraceSticky)
}

func TestHealthGateSelector_StickyGateLeavesCacheUnchanged(t *testing.T) {
	now := time.Unix(1000, 0)
	first := healthGateSelectorCandidate(1, 0, "broken")
	second := healthGateSelectorCandidate(2, 1, "working")
	previous := &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: 1}, threadChannelIDs: map[int]int{20: 1}}
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{first, second}, biz.TraceStickyPreferPreviousChannel, previous)
	openHealthGateModel(t, gate, policy, first, "broken")
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10, ThreadID: 20})

	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, 2, result[0].Channel.ID)
	require.False(t, result[0].TraceSticky)
	require.Equal(t, 1, previous.traceChannelIDs[10])
	require.Equal(t, 1, previous.threadChannelIDs[20])
}

func TestHealthGateSelector_ProbeEligibilityAndBusyFallback(t *testing.T) {
	now := time.Unix(1000, 0)
	candidate := healthGateSelectorCandidate(1, 0, "broken")
	previous := &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: 0}, threadChannelIDs: map[int]int{20: 0}}
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{candidate}, biz.TraceStickyPreferPreviousChannel, previous)
	openHealthGateModel(t, gate, policy, candidate, "broken")
	now = now.Add(11 * time.Second)
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10, ThreadID: 20})
	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.False(t, result[0].healthGate.lastResort)
	require.True(t, result[0].healthGate.probeEligible)

	previous.threadChannelIDs[20] = 9
	result, err = selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.True(t, result[0].healthGate.lastResort)
	previous.threadChannelIDs[20] = 0
	cfg := biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), candidate.Channel)
	key := biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: "broken"}
	ticket, ok := healthGateBegin(t, gate, key, healthGateResolver(cfg), true, false)
	require.True(t, ok)
	result, err = selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.True(t, result[0].healthGate.lastResort)
	_, ok = healthGateBegin(t, gate, key, healthGateResolver(cfg), true, true)
	require.False(t, ok)
	healthGateFinish(t, gate, ticket, healthGateResolver(cfg), biz.HealthGateOutcomeNeutral, biz.HealthGateErrorInfo{})
}

type healthGateUnavailableStickyCache struct{}

func (healthGateUnavailableStickyCache) GetPreviousChannelID(context.Context, int) (int, error) {
	return 0, context.DeadlineExceeded
}

func (healthGateUnavailableStickyCache) GetPreviousChannelIDByThread(context.Context, int) (int, error) {
	return 0, context.DeadlineExceeded
}

func TestHealthGateSelector_StickyReadErrorPreventsNormalProbe(t *testing.T) {
	now := time.Unix(1000, 0)
	candidate := healthGateSelectorCandidate(1, 0, "model")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{candidate}, biz.TraceStickyPreferPreviousChannel, healthGateUnavailableStickyCache{})
	openHealthGateModel(t, gate, policy, candidate, "model")
	now = now.Add(11 * time.Second)
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10})
	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.True(t, result[0].healthGate.lastResort)
}

func TestHealthGateSelector_BusyProbeUsesHealthyCandidate(t *testing.T) {
	now := time.Unix(1000, 0)
	probing := healthGateSelectorCandidate(1, 0, "probing")
	healthy := healthGateSelectorCandidate(2, 1, "healthy")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{probing, healthy}, biz.TraceStickyDisabled, nil)
	openHealthGateModel(t, gate, policy, probing, "probing")
	now = now.Add(11 * time.Second)
	cfg := biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), probing.Channel)
	ticket, ok := healthGateBegin(t, gate, biz.HealthGateKey{ChannelID: probing.Channel.ID, ActualModel: "probing"}, healthGateResolver(cfg), true, false)
	require.True(t, ok)
	result, err := selector.Select(context.Background(), &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, healthy.Channel.ID, result[0].Channel.ID)
	require.False(t, result[0].healthGate.lastResort)
	healthGateFinish(t, gate, ticket, healthGateResolver(cfg), biz.HealthGateOutcomeNeutral, biz.HealthGateErrorInfo{})
}

func TestHealthGateSelector_LastResortPrefersProbingThenEarliestExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	first := healthGateSelectorCandidate(1, 0, "a")
	second := healthGateSelectorCandidate(2, 0, "b")
	third := healthGateSelectorCandidate(3, 0, "c")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{first, second, third}, biz.TraceStickyPreferPreviousChannel, &fakePreviousChannelProvider{traceChannelIDs: map[int]int{10: 99}})
	openHealthGateModel(t, gate, policy, first, "a")
	now = now.Add(2 * time.Second)
	openHealthGateModel(t, gate, policy, second, "b")
	now = now.Add(2 * time.Second)
	openHealthGateModel(t, gate, policy, third, "c")
	ctx := contexts.WithTrace(context.Background(), &ent.Trace{ID: 10})
	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, 1, result[0].Channel.ID)
	require.True(t, result[0].healthGate.lastResort)
	require.False(t, result[0].TraceSticky)

	now = now.Add(7 * time.Second) // a has expired, b and c have not.
	result, err = selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Equal(t, 1, result[0].Channel.ID)
	require.True(t, result[0].healthGate.lastResort)
}

func TestHealthGateSelector_ProbeTakenAfterSelectionReturnsGeneric503(t *testing.T) {
	now := time.Unix(1000, 0)
	candidate := healthGateSelectorCandidate(1, 0, "model")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{candidate}, biz.TraceStickyDisabled, nil)
	openHealthGateModel(t, gate, policy, candidate, "model")
	now = now.Add(11 * time.Second)
	ctx := context.Background()
	first, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	second, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.False(t, first[0].healthGate.lastResort)
	require.False(t, second[0].healthGate.lastResort)
	newTracker := func(candidates []*ChannelModelsCandidate) *healthGateAttemptTracker {
		state := &PersistenceState{
			RoutingPolicy:    EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
			CurrentCandidate: candidates[0], ChannelModelsCandidates: candidates,
		}
		return withHealthGate(&PersistentOutboundTransformer{state: state}, gate, policy.HealthGateOrDefault())
	}
	owner := newTracker(first)
	_, err = owner.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	contender := newTracker(second)
	_, err = contender.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.ErrorIs(t, err, errSkipCandidateByHealthGate)
	var unavailable *llm.ResponseError
	require.ErrorAs(t, contender.finalize(ctx, fmt.Errorf("pipeline: %w", err), false), &unavailable)
	require.Equal(t, 503, unavailable.StatusCode)
	require.Equal(t, "service_unavailable", unavailable.Detail.Code)
	view := healthGateInspect(t, gate, biz.HealthGateKey{ChannelID: 1, ActualModel: "model"}, healthGateResolver(biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), candidate.Channel)))
	require.True(t, view.ProbeBusy)
	require.NoError(t, owner.finalize(ctx, nil, false))
}

func TestHealthGateSelector_ProbeRaceSwitchesToHealthyFallback(t *testing.T) {
	now := time.Unix(1000, 0)
	probe := healthGateSelectorCandidate(1, 0, "probe")
	fallback := healthGateSelectorCandidate(2, 1, "healthy")
	selector, gate, policy := healthGateSelectorFixture(&now, []*ChannelModelsCandidate{probe, fallback}, biz.TraceStickyDisabled, nil)
	openHealthGateModel(t, gate, policy, probe, "probe")
	now = now.Add(11 * time.Second)
	ctx := context.Background()
	first, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	second, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.Equal(t, probe.Channel.ID, second[0].Channel.ID)
	firstState := &PersistenceState{RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}, CurrentCandidate: first[0]}
	owner := withHealthGate(&PersistentOutboundTransformer{state: firstState}, gate, policy.HealthGateOrDefault())
	_, err = owner.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	state := &PersistenceState{RoutingPolicy: EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated}, ChannelModelsCandidates: second, CurrentCandidate: second[0]}
	outbound := &PersistentOutboundTransformer{state: state}
	contender := withHealthGate(outbound, gate, policy.HealthGateOrDefault())
	_, err = contender.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.ErrorIs(t, err, errSkipCandidateByHealthGate)
	require.False(t, outbound.CanRetry(err))
	require.True(t, outbound.HasMoreChannels())
	require.NoError(t, outbound.NextChannel(ctx))
	_, err = contender.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	require.NoError(t, contender.finalize(ctx, nil, false))
	require.NoError(t, owner.finalize(ctx, nil, false))
}

func TestHealthGateSelector_UsesCurrentChannelThreshold(t *testing.T) {
	_, client := setupTest(t)
	service := newTestChannelServiceForChannels(client)
	t.Cleanup(service.Stop)
	candidate := healthGateSelectorCandidate(1, 0, "model")
	policy := &biz.RetryPolicy{HealthGate: &biz.HealthGatePolicy{
		FailureThreshold: 1, OpenDurationSeconds: 300, MaxOpenDurationSeconds: 600,
		ProbeSuccessThreshold: 2, UnstableWindowSeconds: 300,
	}}
	provider := &mockRetryPolicyProvider{policy: policy}
	gate := biz.NewHealthGate(nil)
	openHealthGateModel(t, gate, policy, candidate, "model")
	disabled := 0
	updatedChannel := *candidate.Channel.Channel
	updatedChannel.Settings = &objects.ChannelSettings{HealthGateFailureThreshold: &disabled}
	service.SetEnabledChannelsForTest([]*biz.Channel{{Channel: &updatedChannel}})
	selector := WithTraceStickyLoadBalancedSelector(&staticChannelSelector{candidates: []*ChannelModelsCandidate{candidate}}, NewLoadBalancer(service, service).WithHealthGate(gate), policy, nil)
	selector.healthGatePolicyProvider = provider
	ctx := context.Background()
	result, err := selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	require.False(t, result[0].healthGate.lastResort)
	state := &PersistenceState{
		RoutingPolicy:    EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
		CurrentCandidate: result[0], ChannelService: service, HealthGatePolicyProvider: provider,
	}
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, gate, policy.HealthGateOrDefault())
	_, err = tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	tracker.OnOutboundRawError(ctx, healthGateServerError())
	require.Error(t, tracker.finalize(ctx, healthGateServerError(), false))
	require.Empty(t, healthGateSnapshot(t, gate, candidate.Channel.ID, healthGateResolver(biz.ResolveHealthGateConfig(policy.HealthGateOrDefault(), &biz.Channel{Channel: &updatedChannel}))))
}
