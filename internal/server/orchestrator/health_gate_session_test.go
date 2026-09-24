package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
)

type healthGateSessionFixture struct {
	ctx        context.Context
	client     *ent.Client
	projectID  int
	request    *biz.RequestService
	selector   *LoadBalancedSelector
	gate       *biz.HealthGate
	policy     *biz.RetryPolicy
	candidates []*ChannelModelsCandidate
}

func newHealthGateSessionFixture(t *testing.T) *healthGateSessionFixture {
	t.Helper()
	ctx, client := setupTest(t)
	project := createTestProject(t, ctx, client)
	_, requests, system, _ := setupTestServices(t, client)
	one := healthGateSelectorCandidate(1, 0, "first")
	one.Channel.Name = "primary"
	two := healthGateSelectorCandidate(2, 1, "second")
	two.Channel.Name = "backup"
	candidates := []*ChannelModelsCandidate{one, two}
	policy := &biz.RetryPolicy{
		Enabled: true, MaxChannelRetries: 1,
		LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated,
		TraceStickyMode:      biz.TraceStickyPreferPreviousChannel,
		HealthGate: &biz.HealthGatePolicy{FailureThreshold: 1, OpenDurationSeconds: 300, MaxOpenDurationSeconds: 600,
			ProbeSuccessThreshold: 2, UnstableWindowSeconds: 300, OwnerFailoverThreshold: 2},
	}
	require.NoError(t, system.SetRetryPolicy(ctx, policy))
	gate := biz.NewHealthGate(nil)
	selector := WithTraceStickyLoadBalancedSelector(&staticChannelSelector{candidates: candidates},
		NewLoadBalancer(system, nil).WithHealthGate(gate), system, requests)
	return &healthGateSessionFixture{ctx: ctx, client: client, projectID: project.ID, request: requests, selector: selector, gate: gate, policy: policy, candidates: candidates}
}

func (f *healthGateSessionFixture) selectCandidates(t *testing.T, ctx context.Context) []*ChannelModelsCandidate {
	t.Helper()
	candidates, err := f.selector.Select(ctx, &llm.Request{Model: "alias"})
	require.NoError(t, err)
	return candidates
}

func (f *healthGateSessionFixture) complete(t *testing.T, ctx context.Context, candidates []*ChannelModelsCandidate, index int, success bool, stream bool) objects.RequestRoutingDecision {
	t.Helper()
	row, err := f.client.Request.Create().SetProjectID(f.projectID).SetSource("api").SetStatus("pending").SetModelID("alias").SetRequestBody([]byte(`{"model":"alias"}`)).Save(ctx)
	require.NoError(t, err)
	state := &PersistenceState{
		Request: row, RequestService: f.request,
		RoutingPolicy:           EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
		ChannelModelsCandidates: candidates, CurrentCandidate: candidates[index],
	}
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, f.gate, f.policy.HealthGateOrDefault())
	_, err = tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	if stream {
		streamWrapper, err := tracker.OnOutboundLlmStream(ctx, streams.SliceStream([]*llm.Response{}))
		require.NoError(t, err)
		require.NoError(t, tracker.finalize(ctx, nil, true))
		if success {
			state.StreamCompleted = true
		}
		require.NoError(t, streamWrapper.Close())
	} else if success {
		require.NoError(t, tracker.finalize(ctx, nil, false))
	} else {
		tracker.OnOutboundRawError(ctx, errors.New("local conversion error"))
		require.Error(t, tracker.finalize(ctx, errors.New("local conversion error"), false))
	}
	persisted, err := f.client.Request.Get(ctx, row.ID)
	require.NoError(t, err)
	require.NotNil(t, persisted.RoutingDecision)
	return *persisted.RoutingDecision
}

func setHealthGateSessionOwner(svc *biz.RequestService, ctx context.Context, threadID, traceID int, owner biz.SessionOwner) {
	svc.UpdateSessionOwner(ctx, threadID, traceID, func(biz.SessionOwner, bool) (biz.SessionOwner, bool) {
		return owner, true
	})
}

func TestHealthGateSession_ThreadOwnerPrecedenceAndD2Seed(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 11, ThreadID: 22})
	setHealthGateSessionOwner(f.request, ctx, 0, 11, biz.SessionOwner{ChannelID: 1})
	setHealthGateSessionOwner(f.request, ctx, 22, 0, biz.SessionOwner{ChannelID: 2})
	selected := f.selectCandidates(t, ctx)
	require.Equal(t, 2, selected[0].Channel.ID)
	require.True(t, selected[0].TraceSticky)
	require.Equal(t, 2, selected[0].healthGate.decision.record.Owner.ChannelID)

	fresh := newHealthGateSessionFixture(t)
	ctx = fresh.ctx
	trace, err := fresh.client.Trace.Create().SetProjectID(fresh.projectID).SetTraceID("legacy-seed-trace").Save(ctx)
	require.NoError(t, err)
	row, err := fresh.client.Request.Create().SetProjectID(fresh.projectID).SetTraceID(trace.ID).
		SetSource("api").SetStatus("completed").SetModelID("alias").SetRequestBody([]byte(`{"model":"alias"}`)).Save(ctx)
	require.NoError(t, err)
	ctx = contexts.WithTrace(ctx, trace)
	require.NoError(t, fresh.request.UpdateRequestChannelID(ctx, row.ID, 2))
	selected = fresh.selectCandidates(t, ctx)
	require.Equal(t, 2, selected[0].Channel.ID)
	require.Zero(t, selected[0].healthGate.decision.record.ConsecutiveFailovers)
}

func TestHealthGateSession_TemporaryThenConsecutiveMigration(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1})
	selected := f.selectCandidates(t, ctx)
	require.Equal(t, 1, selected[0].Channel.ID)
	first := f.complete(t, ctx, selected, 1, true, false)
	require.True(t, first.TemporaryFailover)
	require.Equal(t, 1, first.ConsecutiveFailovers)
	require.Nil(t, first.Migration)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1}, owner)
	selected = f.selectCandidates(t, ctx)
	require.Equal(t, 1, selected[0].Channel.ID, "the next request returns to the original owner")
	second := f.complete(t, ctx, selected, 1, true, false)
	require.False(t, second.TemporaryFailover)
	require.Equal(t, 0, second.ConsecutiveFailovers)
	require.Equal(t, objects.RoutingMigrationReasonConsecutiveFailovers, second.Migration.Reason)
	require.Equal(t, "primary", second.Migration.FromChannelName)
	require.Equal(t, "backup", second.Migration.ToChannelName)
	owner, _, err = f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.Equal(t, biz.SessionOwner{ChannelID: 2}, owner)
	require.Equal(t, 2, f.selectCandidates(t, ctx)[0].Channel.ID, "the migrated owner stays selected")
}

func TestHealthGateSession_OwnerOpenMigratesAndRecordsSkipped(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1})
	openHealthGateModel(t, f.gate, f.policy, f.candidates[0], "first")
	selected := f.selectCandidates(t, ctx)
	require.Equal(t, 2, selected[0].Channel.ID)
	require.True(t, selected[0].healthGate.decision.ownerOpen)
	require.Equal(t, []objects.RoutingDecisionCombo{{ChannelID: 1, ChannelName: "primary", ActualModel: "first", State: "open"}}, selected[0].healthGate.decision.record.Skipped)
	decision := f.complete(t, ctx, selected, 0, true, false)
	require.Equal(t, objects.RoutingMigrationReasonOwnerOpen, decision.Migration.Reason)
	require.Equal(t, "first", decision.Owner.ActualModel)
	require.Equal(t, "open", decision.Owner.State)
	owner, _, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.Equal(t, biz.SessionOwner{ChannelID: 2}, owner)
}

func TestHealthGateSession_FailureAndStreamCompletionOnly(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1})
	decision := f.complete(t, ctx, f.selectCandidates(t, ctx), 1, false, false)
	require.Equal(t, 1, decision.ConsecutiveFailovers)
	require.False(t, decision.TemporaryFailover)
	owner, _, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.Equal(t, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1}, owner)
	selected := f.selectCandidates(t, ctx)
	decision = f.complete(t, ctx, selected, 0, true, false)
	require.Equal(t, 0, decision.ConsecutiveFailovers)
	owner, _, err = f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.Equal(t, biz.SessionOwner{ChannelID: 1}, owner)
	selected = f.selectCandidates(t, ctx)
	decision = f.complete(t, ctx, selected, 1, false, true)
	require.Equal(t, 0, decision.ConsecutiveFailovers)
	owner, _, err = f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.Equal(t, biz.SessionOwner{ChannelID: 1}, owner)
	f.gate.Reset(f.candidates[1].Channel.ID, "second")
	selected = f.selectCandidates(t, ctx)
	decision = f.complete(t, ctx, selected, 1, true, true)
	require.True(t, decision.TemporaryFailover)
}

func TestHealthGateSession_LastResortExcludedFromSkippedAndStickyDisabled(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1})
	openHealthGateModel(t, f.gate, f.policy, f.candidates[0], "first")
	openHealthGateModel(t, f.gate, f.policy, f.candidates[1], "second")
	selected := f.selectCandidates(t, ctx)
	require.Len(t, selected, 1)
	require.True(t, selected[0].healthGate.decision.record.LastResort)
	require.Len(t, selected[0].healthGate.decision.record.Skipped, 1)
	require.NotEqual(t, selected[0].Channel.ID, selected[0].healthGate.decision.record.Skipped[0].ChannelID)
	f.policy.TraceStickyMode = biz.TraceStickyDisabled
	require.NoError(t, f.selector.policy.(*biz.SystemService).SetRetryPolicy(ctx, f.policy))
	selected = f.selectCandidates(t, ctx)
	require.Nil(t, selected[0].healthGate.decision.record.Owner)
	require.False(t, selected[0].healthGate.decision.sticky)
}

func TestHealthGateSession_StickyDisabledDoesNotChangeOwner(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1})
	f.policy.TraceStickyMode = biz.TraceStickyDisabled
	require.NoError(t, f.selector.policy.(*biz.SystemService).SetRetryPolicy(ctx, f.policy))
	selected := f.selectCandidates(t, ctx)
	require.Nil(t, selected[0].healthGate.decision.record.Owner)
	decision := f.complete(t, ctx, selected, 1, true, false)
	require.Nil(t, decision.Owner)
	require.False(t, decision.TemporaryFailover)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1}, owner)
}

func TestHealthGateSession_StreamCommitsOnlyAfterTerminalClose(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	selected := f.selectCandidates(t, ctx)
	state := &PersistenceState{
		RoutingPolicy:    EffectiveRoutingPolicy{LoadBalancerStrategy: biz.LoadBalancerStrategyHealthGated},
		CurrentCandidate: selected[0], RequestService: f.request,
	}
	tracker := withHealthGate(&PersistentOutboundTransformer{state: state}, f.gate, f.policy.HealthGateOrDefault())
	_, err := tracker.OnOutboundRawRequest(ctx, &httpclient.Request{})
	require.NoError(t, err)
	require.NotNil(t, tracker.decision)
	require.True(t, tracker.decision.sticky)
	require.Equal(t, 20, tracker.decision.threadID)
	require.True(t, tracker.active)
	wrapper, err := tracker.OnOutboundLlmStream(ctx, streams.SliceStream([]*llm.Response{}))
	require.NoError(t, err)
	require.NoError(t, tracker.finalize(ctx, nil, true))
	require.True(t, tracker.committed)
	_, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.False(t, found)
	state.StreamCompleted = true
	require.NoError(t, wrapper.Close())
	require.False(t, tracker.active)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, selected[0].Channel.ID, owner.ChannelID)
}

func TestHealthGateSession_LateFormerOwnerCannotUndoMigration(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	third := healthGateSelectorCandidate(3, 2, "third")
	third.Channel.Name = "new owner"
	f.candidates = append(f.candidates, third)
	f.selector.wrapped.(*staticChannelSelector).candidates = f.candidates
	f.policy.MaxChannelRetries = 2
	require.NoError(t, f.selector.policy.(*biz.SystemService).SetRetryPolicy(ctx, f.policy))
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1})
	fast := f.selectCandidates(t, ctx)
	slow := f.selectCandidates(t, ctx)
	require.Equal(t, 1, fast[0].Channel.ID)
	require.Equal(t, 1, slow[0].Channel.ID)
	decision := f.complete(t, ctx, fast, 2, true, false)
	require.Equal(t, objects.RoutingMigrationReasonConsecutiveFailovers, decision.Migration.Reason)
	decision = f.complete(t, ctx, slow, 0, true, false)
	require.Equal(t, 1, decision.Owner.ChannelID)
	require.Nil(t, decision.Migration)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, biz.SessionOwner{ChannelID: 3}, owner)
	require.Equal(t, 3, f.selectCandidates(t, ctx)[0].Channel.ID)
}

func TestHealthGateSession_TwoSelectedFailoversCountBoth(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1})
	first, second := f.selectCandidates(t, ctx), f.selectCandidates(t, ctx)
	firstDecision := f.complete(t, ctx, first, 1, true, false)
	require.True(t, firstDecision.TemporaryFailover)
	require.Equal(t, 1, firstDecision.ConsecutiveFailovers)
	secondDecision := f.complete(t, ctx, second, 1, true, false)
	require.Equal(t, objects.RoutingMigrationReasonConsecutiveFailovers, secondDecision.Migration.Reason)
	require.Zero(t, secondDecision.ConsecutiveFailovers)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, biz.SessionOwner{ChannelID: 2}, owner)
}

func TestHealthGateSession_PartiallyGatedOwnerIsNotOwnerOpen(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ctx := contexts.WithTrace(f.ctx, &ent.Trace{ID: 10, ThreadID: 20})
	f.candidates[0].Models = append(f.candidates[0].Models, biz.ChannelModelEntry{RequestModel: "alias", ActualModel: "other"})
	f.candidates[0].modelAPIFormats = append(f.candidates[0].modelAPIFormats, "format-other")
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1})
	openHealthGateModel(t, f.gate, f.policy, f.candidates[0], "first")
	selected := f.selectCandidates(t, ctx)
	require.Equal(t, 1, selected[0].Channel.ID)
	require.Equal(t, "other", selected[0].Models[0].ActualModel)
	require.False(t, selected[0].healthGate.decision.ownerOpen)
	decision := f.complete(t, ctx, selected, 1, true, false)
	require.True(t, decision.TemporaryFailover)
	require.Nil(t, decision.Migration)
	owner, found, err := f.request.GetSessionOwner(ctx, 20, 10)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, biz.SessionOwner{ChannelID: 1, ConsecutiveFailovers: 1}, owner)
}

func TestHealthGateSession_MissingOwnerHasAuditableMigration(t *testing.T) {
	f := newHealthGateSessionFixture(t)
	ownerRow := createTestChannel(t, f.ctx, f.client)
	require.Equal(t, 1, ownerRow.ID)
	ctx := contexts.WithTrace(authz.WithTestBypass(context.Background()), &ent.Trace{ID: 10, ThreadID: 20})
	setHealthGateSessionOwner(f.request, ctx, 20, 10, biz.SessionOwner{ChannelID: 1})
	f.selector.wrapped.(*staticChannelSelector).candidates = f.candidates[1:]
	first := f.selectCandidates(t, ctx)
	require.Len(t, first, 1)
	require.NotNil(t, first[0].healthGate.decision.record.Owner)
	require.Equal(t, ownerRow.Name, first[0].healthGate.decision.record.Owner.ChannelName)
	require.Empty(t, first[0].healthGate.decision.record.Owner.ActualModel)
	firstDecision := f.complete(t, ctx, first, 0, true, false)
	require.True(t, firstDecision.TemporaryFailover)
	require.Equal(t, 1, firstDecision.ConsecutiveFailovers)
	require.Equal(t, ownerRow.Name, firstDecision.Owner.ChannelName)
	secondDecision := f.complete(t, ctx, f.selectCandidates(t, ctx), 0, true, false)
	require.Equal(t, objects.RoutingMigrationReasonConsecutiveFailovers, secondDecision.Migration.Reason)
	require.Equal(t, 1, secondDecision.Migration.FromChannelID)
	require.Equal(t, ownerRow.Name, secondDecision.Migration.FromChannelName)
	require.Equal(t, 2, secondDecision.Migration.ToChannelID)
}
