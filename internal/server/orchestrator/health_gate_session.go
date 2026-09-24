package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
)

type healthGateDecisionContextKey struct{}

func healthGateDecisionFromContext(ctx context.Context) *healthGateRequestDecision {
	if slot, ok := ctx.Value(healthGateDecisionContextKey{}).(**healthGateRequestDecision); ok && slot != nil {
		return *slot
	}
	return nil
}

// Older selector callers need only the previous-channel interface. The
// production RequestService implements this additional session-owner method.
type healthGateSessionOwnerProvider interface {
	GetSessionOwner(ctx context.Context, threadID, traceID int) (biz.SessionOwner, bool, error)
}

type healthGateRequestDecision struct {
	threadID      int
	traceID       int
	sticky        bool
	owner         biz.SessionOwner
	found         bool
	ownerOpen     bool
	probeEligible bool
	// candidates is the set after ordinary request admission and before health
	// gating. It is needed to recheck every owner model at completion.
	candidates []*ChannelModelsCandidate
	channelSvc *biz.ChannelService
	requestSvc *biz.RequestService
	policy     RetryPolicyProvider
	fallback   biz.HealthGatePolicy
	record     objects.RequestRoutingDecision
	threshold  int
}

func (s *LoadBalancedSelector) healthGateSessionDecision(ctx context.Context, mode biz.TraceStickyMode, policy *biz.RetryPolicy) (*healthGateRequestDecision, bool) {
	d := &healthGateRequestDecision{
		sticky:    mode == biz.TraceStickyPreferPreviousChannel,
		threshold: policy.HealthGateOrDefault().OwnerFailoverThreshold,
		record: objects.RequestRoutingDecision{
			Skipped: []objects.RoutingDecisionCombo{},
		},
	}
	if slot, ok := ctx.Value(healthGateDecisionContextKey{}).(**healthGateRequestDecision); ok && slot != nil {
		*slot = d
	}
	if !d.sticky {
		return d, true
	}
	trace, hasTrace := contexts.GetTrace(ctx)
	if hasTrace && trace != nil {
		d.traceID = trace.ID
		d.threadID = trace.ThreadID
	}
	if thread, ok := contexts.GetThread(ctx); ok && thread != nil {
		d.threadID = thread.ID
	}
	provider, ok := s.previousChannelProvider.(healthGateSessionOwnerProvider)
	if !ok || provider == nil {
		return d, s.previousChannelProvider == nil
	}
	owner, found, err := provider.GetSessionOwner(ctx, d.threadID, d.traceID)
	if err != nil {
		log.Warn(ctx, "failed to read health gate session owner", log.Cause(err))
		return d, false
	}
	d.owner, d.found = owner, found && owner.ChannelID != 0
	if d.found {
		d.record.ConsecutiveFailovers = owner.ConsecutiveFailovers
	}
	return d, !d.found
}

func healthGateCombo(candidate *ChannelModelsCandidate, actualModel string, state biz.HealthGateState) objects.RoutingDecisionCombo {
	return objects.RoutingDecisionCombo{
		ChannelID: candidate.Channel.ID, ChannelName: candidate.Channel.Name,
		ActualModel: actualModel, State: string(state),
	}
}

// The owner may have been excluded by model, permission, or quota filtering.
// Capture its name at selection time while keeping its model/state empty.
func healthGateOwnerName(ctx context.Context, channelID int, candidates []*ChannelModelsCandidate, service *biz.ChannelService, requests *biz.RequestService) string {
	for _, candidate := range candidates {
		if candidate != nil && candidate.Channel != nil && candidate.Channel.ID == channelID {
			return candidate.Channel.Name
		}
	}
	if service != nil {
		if current := service.GetEnabledChannel(channelID); current != nil {
			return current.Name
		}
	}
	if requests != nil {
		if name, err := requests.SessionOwnerChannelName(ctx, channelID); err == nil {
			return name
		}
	}
	return ""
}

// ownerGatedAtCompletion considers every admitted model for the CURRENT owner.
// A missing owner is a regular failover; one healthy owner model is sufficient
// to prevent an immediate owner_open migration.
func (d *healthGateRequestDecision) ownerGatedAtCompletion(ctx context.Context, gate *biz.HealthGate, channelID int) bool {
	if gate == nil {
		return false
	}
	seen := false
	for _, candidate := range d.candidates {
		if candidate == nil || candidate.Channel == nil || candidate.Channel.ID != channelID {
			continue
		}
		cfg := currentHealthGateConfig(ctx, d.policy, d.channelSvc, candidate.Channel, d.fallback)
		for _, model := range candidate.Models {
			seen = true
			view := gate.Inspect(biz.HealthGateKey{ChannelID: channelID, ActualModel: model.ActualModel}, cfg)
			if view.State == biz.HealthGateStateHealthy || view.State == biz.HealthGateStateUnstable ||
				(view.State == biz.HealthGateStateProbing && d.probeEligible && !view.ProbeBusy) {
				return false
			}
		}
	}
	return seen
}

// finishSession records actual completion. Read/modify/write of the current
// session owner happens in biz under its per-session lock, so a late completion
// cannot overwrite a newer migration made after this request selected candidates.
func (m *healthGateAttemptTracker) finishSession(ctx context.Context, successful bool) {
	d := m.decision
	if d == nil || m.outbound == nil || m.outbound.state == nil {
		return
	}
	state := m.outbound.state
	record := d.record
	record.LastResort = m.lastResort || record.LastResort
	if successful && m.channel != nil && d.sticky && state.RequestService != nil && (d.threadID != 0 || d.traceID != 0) {
		success := healthGateCombo(state.CurrentCandidate, m.key.ActualModel, "")
		state.RequestService.UpdateSessionOwner(context.WithoutCancel(ctx), d.threadID, d.traceID, func(current biz.SessionOwner, found bool) (biz.SessionOwner, bool) {
			if !found {
				record.ConsecutiveFailovers = 0
				return biz.SessionOwner{ChannelID: success.ChannelID}, true
			}
			record.ConsecutiveFailovers = current.ConsecutiveFailovers
			if current.ChannelID == success.ChannelID {
				record.ConsecutiveFailovers = 0
				return biz.SessionOwner{ChannelID: current.ChannelID}, true
			}
			// This request selected the former owner. If another request already
			// migrated the session, its late success cannot move it back.
			if d.found && current.ChannelID != d.owner.ChannelID && success.ChannelID == d.owner.ChannelID {
				return current, false
			}
			fromName := healthGateOwnerName(ctx, current.ChannelID, d.candidates, d.channelSvc, d.requestSvc)
			if d.found && current.ChannelID == d.owner.ChannelID && d.record.Owner != nil {
				fromName = d.record.Owner.ChannelName
			}
			migration := func(reason string) (biz.SessionOwner, bool) {
				record.ConsecutiveFailovers = 0
				record.Migration = &objects.RoutingDecisionMigration{
					FromChannelID: current.ChannelID, FromChannelName: fromName,
					ToChannelID: success.ChannelID, ToChannelName: success.ChannelName,
					Reason: reason,
				}
				return biz.SessionOwner{ChannelID: success.ChannelID}, true
			}
			if d.ownerGatedAtCompletion(ctx, m.gate, current.ChannelID) {
				return migration(objects.RoutingMigrationReasonOwnerOpen)
			}
			count := current.ConsecutiveFailovers + 1
			if count >= d.threshold {
				return migration(objects.RoutingMigrationReasonConsecutiveFailovers)
			}
			record.ConsecutiveFailovers = count
			record.TemporaryFailover = true
			return biz.SessionOwner{ChannelID: current.ChannelID, ConsecutiveFailovers: count}, true
		})
	}
	if state.Request == nil || state.RequestService == nil {
		return
	}
	if err := state.RequestService.SetRequestRoutingDecision(context.WithoutCancel(ctx), state.Request.ID, &record); err != nil {
		log.Warn(ctx, "failed to persist health gate routing decision", log.Cause(err))
	}
}
