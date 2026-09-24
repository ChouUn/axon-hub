package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
)

type healthGateCandidateInfo struct {
	probeEligible bool
	lastResort    bool
}

type gatedModel struct {
	candidate *ChannelModelsCandidate
	index     int
	view      biz.HealthGateView
}

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

	probeEligible := s.healthGateProbeEligible(ctx, stickyMode)
	var channelService *biz.ChannelService
	if service, ok := lb.selectionTracker.(*biz.ChannelService); ok {
		channelService = service
	}
	filtered := make([]*ChannelModelsCandidate, 0, len(candidates))
	var lastResort *gatedModel
	for _, candidate := range candidates {
		if candidate == nil || candidate.Channel == nil {
			continue
		}
		resolve := currentHealthGateConfig(ctx, s.policy, channelService, candidate.Channel, policy.HealthGateOrDefault())
		indices := make([]int, 0, len(candidate.Models))
		for index, entry := range candidate.Models {
			view := lb.healthGate.Inspect(biz.HealthGateKey{ChannelID: candidate.Channel.ID, ActualModel: entry.ActualModel}, resolve)
			if view.State == biz.HealthGateStateHealthy || view.State == biz.HealthGateStateUnstable ||
				(view.State == biz.HealthGateStateProbing && probeEligible && !view.ProbeBusy) {
				indices = append(indices, index)
				continue
			}
			if lastResort == nil || healthGateLastResortBefore(view, lastResort.view) {
				lastResort = &gatedModel{candidate: candidate, index: index, view: view}
			}
		}
		if len(indices) != 0 {
			filtered = append(filtered, cloneHealthGateCandidate(ctx, req, candidate, indices, &healthGateCandidateInfo{probeEligible: probeEligible}))
		}
	}

	if len(filtered) == 0 {
		if lastResort == nil {
			return filtered, nil
		}
		clone := cloneHealthGateCandidate(ctx, req, lastResort.candidate, []int{lastResort.index}, &healthGateCandidateInfo{lastResort: true})
		clone.TraceSticky = false
		return []*ChannelModelsCandidate{clone}, nil
	}

	requiredCount := 1
	if policy.Enabled {
		requiredCount += policy.MaxChannelRetries
	}
	if stickyMode == biz.TraceStickyPreferPreviousChannel {
		if sticky, remaining := s.selectTraceStickyCandidate(ctx, filtered); sticky != nil {
			sticky.TraceSticky = true
			fallbacks := s.sortCandidates(ctx, lb, remaining, req, max(requiredCount-1, 0), false)
			if lb != nil {
				lb.TrackSelection(sticky)
			}
			return append([]*ChannelModelsCandidate{sticky}, fallbacks...), nil
		}
	}
	return s.sortCandidates(ctx, lb, filtered, req, requiredCount, true), nil
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

// The original sticky cache value, not the presence of a valid sticky candidate,
// determines whether this request may take a normal probe.
func (s *LoadBalancedSelector) healthGateProbeEligible(ctx context.Context, mode biz.TraceStickyMode) bool {
	if mode != biz.TraceStickyPreferPreviousChannel {
		return true
	}
	if s.previousChannelProvider == nil {
		return true
	}
	trace, hasTrace := contexts.GetTrace(ctx)
	if hasTrace && trace != nil {
		channelID, err := s.previousChannelProvider.GetPreviousChannelID(ctx, trace.ID)
		if err != nil {
			log.Warn(ctx, "failed to read trace stickiness for health probe", log.Cause(err))
			return false
		}
		if channelID != 0 {
			return false
		}
	}
	threadID := 0
	if thread, ok := contexts.GetThread(ctx); ok && thread != nil {
		threadID = thread.ID
	} else if hasTrace && trace != nil {
		threadID = trace.ThreadID
	}
	if threadID == 0 {
		return true
	}
	channelID, err := s.previousChannelProvider.GetPreviousChannelIDByThread(ctx, threadID)
	if err != nil {
		log.Warn(ctx, "failed to read thread stickiness for health probe", log.Cause(err))
		return false
	}
	return channelID == 0
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
		endpoints := applyForcedAPIFormats(ctx, clone.Channel, clone.Models[:1], req.Model, clone.Channel.ResolveEndpoints())
		clone.APIFormat = SelectAPIFormat(endpoints, req)
	}
	clone.healthGate = info
	return &clone
}
