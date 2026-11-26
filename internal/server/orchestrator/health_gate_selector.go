package orchestrator

import (
	"context"
	"slices"

	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

type healthGateCandidateInfo struct {
	probeEligible bool
	lastResort    bool
	decision      *healthGateRequestDecision
}

type gatedModel struct {
	candidate *ChannelModelsCandidate
	index     int
	view      biz.HealthGateView
}

// selectHealthGated applies the same health checks to preferred and quota-reserved
// candidates, then appends healthy reserved candidates last to preserve backpressure.
func (s *LoadBalancedSelector) selectHealthGated(
	ctx context.Context,
	req *llm.Request,
	candidates []*ChannelModelsCandidate,
	policy *biz.RetryPolicy,
	stickyMode biz.TraceStickyMode,
	lb *LoadBalancer,
) ([]*ChannelModelsCandidate, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	decision, probeEligible := s.healthGateSessionDecision(ctx, stickyMode, policy)
	stickyID := 0
	if decision.sticky && decision.found {
		stickyID = decision.owner.ChannelID
	}
	var reserved []*ChannelModelsCandidate
	if s.quotaGate != nil {
		candidates, reserved = s.quotaGate.Filter(ctx, candidates, req, stickyID)
	}
	var channelService *biz.ChannelService
	if service, ok := lb.selectionTracker.(*biz.ChannelService); ok {
		channelService = service
	}
	decision.probeEligible = probeEligible
	decision.candidates = slices.Concat(candidates, reserved)
	decision.channelSvc = channelService
	decision.requestSvc, _ = s.previousChannelProvider.(*biz.RequestService)
	decision.policy = s.healthGatePolicyProvider
	decision.fallback = policy.HealthGateOrDefault()
	if decision.found {
		decision.record.Owner = &objects.RoutingDecisionCombo{
			ChannelID:   decision.owner.ChannelID,
			ChannelName: healthGateOwnerName(ctx, decision.owner.ChannelID, decision.candidates, channelService, decision.requestSvc),
		}
	}
	var lastResort *gatedModel
	ownerSeen, ownerAllowed := false, false
	inspect := func(candidates []*ChannelModelsCandidate) ([]*ChannelModelsCandidate, error) {
		healthy := make([]*ChannelModelsCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate == nil || candidate.Channel == nil {
				continue
			}
			ownerCandidate := decision.found && candidate.Channel.ID == decision.owner.ChannelID
			if ownerCandidate {
				ownerSeen = true
				decision.record.Owner.ChannelName = candidate.Channel.Name
			}
			cfg := currentHealthGateConfig(ctx, s.healthGatePolicyProvider, channelService, candidate.Channel, policy.HealthGateOrDefault())
			indices := make([]int, 0, len(candidate.Models))
			for index, entry := range candidate.Models {
				view, err := lb.healthGate.Inspect(ctx, biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: entry.ActualModel}, cfg)
				if err != nil {
					return nil, err
				}
				combo := healthGateCombo(candidate, entry.ActualModel, view.State)
				if ownerCandidate && decision.record.Owner.ActualModel == "" {
					decision.record.Owner.ActualModel = combo.ActualModel
					decision.record.Owner.State = combo.State
				}
				if view.State == biz.HealthGateStateHealthy || view.State == biz.HealthGateStateUnstable ||
					(view.State == biz.HealthGateStateProbing && probeEligible && !view.ProbeBusy) {
					indices = append(indices, index)
					if ownerCandidate {
						ownerAllowed = true
					}
					continue
				}
				decision.record.Skipped = append(decision.record.Skipped, combo)
				if lastResort == nil || healthGateLastResortBefore(view, lastResort.view) {
					lastResort = &gatedModel{candidate: candidate, index: index, view: view}
				}
			}
			if len(indices) != 0 {
				healthy = append(healthy, cloneHealthGateCandidate(ctx, req, candidate, indices, &healthGateCandidateInfo{probeEligible: probeEligible, decision: decision}))
			}
		}
		return healthy, nil
	}
	preferred, err := inspect(candidates)
	if err != nil {
		return nil, err
	}
	fallbackHealthy, err := inspect(reserved)
	if err != nil {
		return nil, err
	}
	decision.ownerOpen = decision.found && ownerSeen && !ownerAllowed
	requiredCount := 1
	if policy.Enabled {
		requiredCount += policy.MaxChannelRetries
	}
	if len(preferred) == 0 {
		if len(fallbackHealthy) != 0 {
			result := quotaFallbackCandidates(fallbackHealthy, requiredCount)
			for _, candidate := range result {
				candidate.TraceSticky = false
			}
			lb.TrackSelection(result[0])
			return result, nil
		}
		if lastResort == nil {
			return preferred, nil
		}
		selected := healthGateCombo(lastResort.candidate, lastResort.candidate.Models[lastResort.index].ActualModel, lastResort.view.State)
		for i, skipped := range decision.record.Skipped {
			if skipped.ChannelID == selected.ChannelID && skipped.ActualModel == selected.ActualModel {
				decision.record.Skipped = append(decision.record.Skipped[:i], decision.record.Skipped[i+1:]...)
				break
			}
		}
		decision.record.LastResort = true
		clone := cloneHealthGateCandidate(ctx, req, lastResort.candidate, []int{lastResort.index}, &healthGateCandidateInfo{lastResort: true, decision: decision})
		clone.TraceSticky = false
		return []*ChannelModelsCandidate{clone}, nil
	}
	quotaFallback := quotaFallbackCandidates(fallbackHealthy, quotaFallbackLimit(policy))
	// A Guardian review prefers its parent's channel when that channel passed the gate,
	// matching the upstream selector, which pins it after quota admission.
	if pinned, remaining := extractStickyCandidate(preferred, guardianReviewParentChannelID(ctx)); pinned != nil {
		pinned.TraceSticky = false
		fallbacks := s.sortCandidates(ctx, lb, remaining, req, max(requiredCount-1, 0), requiredCount, false)
		lb.TrackSelection(pinned)
		result := make([]*ChannelModelsCandidate, 0, 1+len(fallbacks)+len(quotaFallback))
		result = append(result, pinned)
		result = append(result, fallbacks...)
		return append(result, quotaFallback...), nil
	}
	if decision.sticky && decision.found && ownerAllowed {
		if sticky, remaining := extractStickyCandidate(preferred, decision.owner.ChannelID); sticky != nil {
			sticky.TraceSticky = true
			fallbacks := s.sortCandidates(ctx, lb, remaining, req, max(requiredCount-1, 0), requiredCount, false)
			lb.TrackSelection(sticky)
			result := make([]*ChannelModelsCandidate, 0, 1+len(fallbacks)+len(quotaFallback))
			result = append(result, sticky)
			result = append(result, fallbacks...)
			return append(result, quotaFallback...), nil
		}
	}
	result := s.sortCandidates(ctx, lb, preferred, req, requiredCount, requiredCount, true)
	return append(result, quotaFallback...), nil
}

func healthGateLastResortBefore(a, b biz.HealthGateView) bool {
	if a.State == biz.HealthGateStateProbing && b.State != biz.HealthGateStateProbing {
		return true
	}
	if b.State == biz.HealthGateStateProbing && a.State != biz.HealthGateStateProbing {
		return false
	}
	return a.OpenUntil.Before(b.OpenUntil)
}

func cloneHealthGateCandidate(ctx context.Context, req *llm.Request, source *ChannelModelsCandidate, indices []int, info *healthGateCandidateInfo) *ChannelModelsCandidate {
	clone := *source
	clone.Models = make([]biz.ChannelModelEntry, 0, len(indices))
	if len(source.modelAPIFormats) == len(source.Models) {
		clone.modelAPIFormats = make([]string, 0, len(indices))
	} else {
		clone.modelAPIFormats = nil
	}
	for _, index := range indices {
		clone.Models = append(clone.Models, source.Models[index])
		if clone.modelAPIFormats != nil {
			clone.modelAPIFormats = append(clone.modelAPIFormats, source.modelAPIFormats[index])
		}
	}
	if len(clone.modelAPIFormats) != 0 {
		clone.APIFormat = clone.modelAPIFormats[0]
	} else if indices[0] != 0 {
		endpoints, forced := applyForcedAPIFormats(ctx, clone.Channel, clone.Models[:1], req.Model, clone.Channel.ResolveEndpoints())
		clone.APIFormat = selectEndpointAPIFormat(ctx, endpoints, req, forced)
	}
	clone.healthGate = info
	return &clone
}
