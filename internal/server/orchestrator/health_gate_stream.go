package orchestrator

import (
	"context"
	"sync"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/streams"
)

//nolint:containedctx // Finalization retains request metadata after delivery ends.
type healthGateResponseStream struct {
	streams.Stream[*httpclient.StreamEvent]
	ctx      context.Context
	tracker  *healthGateAttemptTracker
	observed *healthGateStream
	raw      streams.Stream[*httpclient.StreamEvent]
	drain    <-chan struct{}
	once     sync.Once
	closeErr error

	terminalTracker  *StreamTerminalTracker
	providerTerminal streamTerminalState

	deliveryMu  sync.Mutex
	deliveryErr error
}

func (m *healthGateAttemptTracker) wrapResponseStream(ctx context.Context, stream streams.Stream[*httpclient.StreamEvent]) streams.Stream[*httpclient.StreamEvent] {
	if !m.enabled() {
		return stream
	}
	m.mu.Lock()
	result := &healthGateResponseStream{
		Stream: stream, ctx: ctx, tracker: m, observed: m.stream,
		raw: m.rawStream, drain: m.outbound.state.rawStreamDrainDone,
	}
	if result.drain != nil {
		result.terminalTracker = NewStreamTerminalTrackerForRequest(m.outbound.state.Request)
	}
	m.mu.Unlock()
	return httpclient.WithResponseHeaders(result, httpclient.GetResponseHeaders(stream))
}

func (s *healthGateResponseStream) Current() *httpclient.StreamEvent {
	event := s.Stream.Current()
	if s.terminalTracker != nil && s.terminalTracker.Observe(event) {
		s.deliveryMu.Lock()
		s.providerTerminal = classifyAcceptedTerminalEvent(event)
		s.deliveryMu.Unlock()
		s.terminalTracker = nil
	}
	return event
}

func (s *healthGateResponseStream) Close() error {
	s.once.Do(func() {
		s.closeErr = s.Stream.Close()
		if s.drain != nil {
			<-s.drain
		}
		s.finish()
	})
	return s.closeErr
}

func (s *healthGateResponseStream) finish() {
	ctx := s.ctx
	streamErr := s.Stream.Err()
	meaningful := false
	if s.observed != nil {
		ctx = s.observed.ctx
		meaningful = s.observed.meaningful
		if err := s.observed.Err(); err != nil && !IsUpstreamTransportError(streamErr) {
			streamErr = err
		}
	}
	if s.raw != nil {
		if err := s.raw.Err(); err != nil && !IsUpstreamTransportError(streamErr) && (streamErr == nil || IsUpstreamTransportError(err)) {
			streamErr = err
		}
	}
	s.deliveryMu.Lock()
	deliveryErr := s.deliveryErr
	providerTerminal := s.providerTerminal
	s.deliveryMu.Unlock()
	m := s.tracker
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.committed {
		return
	}
	m.committed = false
	state := m.outbound.state
	if state.OutboundStreamTerminal != streamTerminalNone {
		providerTerminal = state.OutboundStreamTerminal
	}
	if m.active {
		m.lastOutcome = classifyHealthGateStreamOutcome(ctx, state, providerTerminal, meaningful, streamErr, deliveryErr)
		if m.lastOutcome == biz.HealthGateOutcomeSuccess {
			m.lastInfo = biz.HealthGateErrorInfo{}
		} else {
			if streamErr == nil && (m.lastOutcome == biz.HealthGateOutcomeFailure || m.lastOutcome == biz.HealthGateOutcomeUnstable) {
				streamErr = ErrStreamIncomplete
			}
			m.lastInfo = healthGateErrorInfo(streamErr)
		}
		m.finishActive(s.ctx)
	}
	m.finishSession(s.ctx, state.StreamCompleted)
}

func (s *healthGateResponseStream) AcknowledgeDelivery() {
	if observer, ok := s.Stream.(streams.DeliveryAcknowledger); ok {
		observer.AcknowledgeDelivery()
	}
}

func (s *healthGateResponseStream) DeliveryAcknowledged() bool {
	observer, ok := s.Stream.(streams.DeliveryAcknowledger)
	return ok && observer.DeliveryAcknowledged()
}

func (s *healthGateResponseStream) Deliver(ctx context.Context, write func() (bool, error)) error {
	tracked := func() (bool, error) {
		terminal, err := write()
		if err != nil {
			s.deliveryMu.Lock()
			if s.deliveryErr == nil {
				s.deliveryErr = err
			}
			s.deliveryMu.Unlock()
		}
		return terminal, err
	}
	if boundary, ok := s.Stream.(streams.DeliveryBoundary); ok {
		return boundary.Deliver(ctx, tracked)
	}
	terminal, err := tracked()
	if err == nil && terminal {
		s.AcknowledgeDelivery()
	}
	return err
}

func (s *healthGateResponseStream) WaitForFinalization() {
	if waiter, ok := s.Stream.(streams.FinalizationWaiter); ok {
		waiter.WaitForFinalization()
	}
}

func (s *healthGateResponseStream) ExpectedStreamChoices() int {
	if choices, ok := s.Stream.(interface{ ExpectedStreamChoices() int }); ok {
		return choices.ExpectedStreamChoices()
	}
	return 0
}
