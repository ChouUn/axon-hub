package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
)

func TestHealthGateOutcome_Boundaries(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		outcome biz.HealthGateOutcome
	}{
		{"upstream 500", &httpclient.Error{StatusCode: 500}, biz.HealthGateOutcomeFailure},
		{"upstream 503", &httpclient.Error{StatusCode: 503}, biz.HealthGateOutcomeFailure},
		{"unauthorized", &httpclient.Error{StatusCode: 401}, biz.HealthGateOutcomeFailure},
		{"forbidden", &llm.ResponseError{StatusCode: 403}, biz.HealthGateOutcomeFailure},
		{"request timeout", &httpclient.Error{StatusCode: 408}, biz.HealthGateOutcomeFailure},
		{"rate limit", &httpclient.Error{StatusCode: 429}, biz.HealthGateOutcomeNeutral},
		{"bad request", &httpclient.Error{StatusCode: 400}, biz.HealthGateOutcomeNeutral},
		{"not found", &httpclient.Error{StatusCode: 404}, biz.HealthGateOutcomeNeutral},
		{"unprocessable", &httpclient.Error{StatusCode: 422}, biz.HealthGateOutcomeNeutral},
		{"breaker skip", errSkipCandidateByCircuitBreaker, biz.HealthGateOutcomeNeutral},
		{"health skip", errSkipCandidateByHealthGate, biz.HealthGateOutcomeNeutral},
		{"empty response", pipeline.ErrEmptyResponse, biz.HealthGateOutcomeFailure},
		{"empty stream", pipeline.ErrEmptyStreamChunks, biz.HealthGateOutcomeFailure},
		{"queue full", asChannelQueueError(stickyTestCandidate(1, 0).Channel, ErrChannelQueueFull), biz.HealthGateOutcomeNeutral},
		{"local RPM exhausted", newLocalRPMExhaustedError(stickyTestCandidate(1, 0).Channel, 1), biz.HealthGateOutcomeNeutral},
		{"empty aggregated", pipeline.ErrEmptyAggregatedBody, biz.HealthGateOutcomeFailure},
		{"stream timeout", pipeline.ErrStreamFirstEventTimeout, biz.HealthGateOutcomeFailure},
		{"response timeout", pipeline.ErrNonStreamResponseTimeout, biz.HealthGateOutcomeFailure},
		{"deadline", context.DeadlineExceeded, biz.HealthGateOutcomeFailure},
		{"network", &net.DNSError{Err: "failed"}, biz.HealthGateOutcomeFailure},
		{"EOF", io.EOF, biz.HealthGateOutcomeFailure},
		{"unexpected EOF", io.ErrUnexpectedEOF, biz.HealthGateOutcomeFailure},
		{"incomplete stream", ErrStreamIncomplete, biz.HealthGateOutcomeFailure},
		{"incomplete LLM stream", llm.ErrStreamIncomplete, biz.HealthGateOutcomeFailure},
		{"upstream stream idle timeout", httpclient.ErrStreamIdleTimeout, biz.HealthGateOutcomeFailure},
		{"connection reset", syscall.ECONNRESET, biz.HealthGateOutcomeFailure},
		{"broken pipe", syscall.EPIPE, biz.HealthGateOutcomeFailure},
		{"bad request joined with EOF", errors.Join(&httpclient.Error{StatusCode: 400}, io.EOF), biz.HealthGateOutcomeNeutral},
		{"bad request joined with network reset", errors.Join(&llm.ResponseError{StatusCode: 400}, syscall.ECONNRESET), biz.HealthGateOutcomeFailure},
		{"rate limit joined with EOF", errors.Join(&httpclient.Error{StatusCode: 429}, io.EOF), biz.HealthGateOutcomeNeutral},
		{"other client error", &httpclient.Error{StatusCode: 409}, biz.HealthGateOutcomeNeutral},
		{"unknown local", errors.New("local conversion"), biz.HealthGateOutcomeNeutral},
		{"canceled", context.Canceled, biz.HealthGateOutcomeNeutral},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.outcome, classifyHealthGateOutcome(context.Background(), fmt.Errorf("wrapped: %w", tc.err)))
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, biz.HealthGateOutcomeNeutral, classifyHealthGateOutcome(ctx, context.DeadlineExceeded))
	timeoutCtx, endTimeout := context.WithCancelCause(context.Background())
	endTimeout(pipeline.ErrNonStreamResponseTimeout)
	require.Equal(t, biz.HealthGateOutcomeFailure, classifyHealthGateOutcome(timeoutCtx, context.Canceled))
	streamCtx, endStream := context.WithCancelCause(context.Background())
	endStream(pipeline.ErrStreamFirstEventTimeout)
	require.Equal(t, biz.HealthGateOutcomeFailure, classifyHealthGateOutcome(streamCtx, context.Canceled))
	require.Equal(t, biz.HealthGateOutcomeSuccess, classifyHealthGateOutcome(context.Background(), nil))
}
