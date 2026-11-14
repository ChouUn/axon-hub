package orchestrator

import (
	"context"
	"errors"
	"net"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/pipeline"
)

func healthGatePipelineTimeout(ctx context.Context) bool {
	if ctx == nil || ctx.Err() == nil {
		return false
	}
	cause := context.Cause(ctx)
	return errors.Is(cause, pipeline.ErrNonStreamResponseTimeout) || errors.Is(cause, pipeline.ErrStreamFirstEventTimeout)
}

func healthGateClientCanceled(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if owner := streamLifecycleFromContext(ctx); owner != nil {
		return owner.downstream.Err() != nil || (owner.shutdown != nil && owner.shutdown.Err() != nil)
	}
	return ctx != nil && ctx.Err() != nil && !healthGatePipelineTimeout(ctx)
}

func classifyHealthGateOutcome(ctx context.Context, err error) biz.HealthGateOutcome {
	if err == nil {
		return biz.HealthGateOutcomeSuccess
	}
	if errors.Is(err, errSkipCandidateByCircuitBreaker) || errors.Is(err, errSkipCandidateByHealthGate) ||
		isChannelQueueError(err) || isLocalRPMExhaustedError(err) {
		return biz.HealthGateOutcomeNeutral
	}
	if healthGatePipelineTimeout(ctx) {
		return biz.HealthGateOutcomeFailure
	}
	if healthGateClientCanceled(ctx) || errors.Is(err, context.Canceled) {
		return biz.HealthGateOutcomeNeutral
	}
	if errors.Is(err, pipeline.ErrEmptyResponse) || errors.Is(err, pipeline.ErrEmptyStreamChunks) ||
		errors.Is(err, pipeline.ErrEmptyAggregatedBody) || errors.Is(err, pipeline.ErrStreamFirstEventTimeout) ||
		errors.Is(err, pipeline.ErrNonStreamResponseTimeout) || errors.Is(err, context.DeadlineExceeded) ||
		IsUpstreamTransportError(err) {
		return biz.HealthGateOutcomeFailure
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return biz.HealthGateOutcomeFailure
	}
	status := ExtractStatusCodeFromError(err)
	if status >= 500 || status == 401 || status == 403 || status == 408 {
		return biz.HealthGateOutcomeFailure
	}
	return biz.HealthGateOutcomeNeutral
}

func classifyHealthGateStreamOutcome(ctx context.Context, state *PersistenceState, terminal streamTerminalState, meaningful bool, streamErr, deliveryErr error) biz.HealthGateOutcome {
	interrupted := func() biz.HealthGateOutcome {
		if meaningful {
			return biz.HealthGateOutcomeUnstable
		}
		return biz.HealthGateOutcomeFailure
	}
	switch terminal {
	case streamTerminalCompleted, streamTerminalIncomplete:
		return biz.HealthGateOutcomeSuccess
	case streamTerminalCanceled:
		return biz.HealthGateOutcomeNeutral
	case streamTerminalFailed:
		return interrupted()
	case streamTerminalNone:
	}
	// Performance completion is upstream generation evidence, including validated
	// aggregation without a wire terminal. It is independent of downstream delivery.
	if state.CleanEOFCompletionEvidence || (state.Perf != nil && state.Perf.RequestCompleted && state.Perf.Success) {
		return biz.HealthGateOutcomeSuccess
	}
	if IsUpstreamTransportError(streamErr) || healthGatePipelineTimeout(ctx) {
		return interrupted()
	}
	if healthGateClientCanceled(ctx) || deliveryErr != nil ||
		(streamLifecycleFromContext(ctx) == nil && errors.Is(streamErr, context.Canceled)) {
		return biz.HealthGateOutcomeNeutral
	}
	return interrupted()
}
