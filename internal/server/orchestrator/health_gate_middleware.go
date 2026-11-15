package orchestrator

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/pkg/xcontext"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm"
	"github.com/looplj/axonhub/llm/httpclient"
	"github.com/looplj/axonhub/llm/pipeline"
	"github.com/looplj/axonhub/llm/streams"
)

// healthGateAttemptTracker owns exactly one ticket per consecutive channel/model
// combination throughout a Process call, including its same-model retries.
type healthGateAttemptTracker struct {
	pipeline.DummyMiddleware
	outbound *PersistentOutboundTransformer
	gate     *biz.HealthGate
	policy   biz.HealthGatePolicy

	mu          sync.Mutex
	active      bool
	key         biz.HealthGateKey
	channel     *biz.Channel
	ticket      biz.HealthGateTicket
	lastOutcome biz.HealthGateOutcome
	lastInfo    biz.HealthGateErrorInfo
	lastResort  bool
	decision    *healthGateRequestDecision
	committed   bool
	rawStream   streams.Stream[*httpclient.StreamEvent]
	stream      *healthGateStream
}

func withHealthGate(outbound *PersistentOutboundTransformer, gate *biz.HealthGate, policy biz.HealthGatePolicy) *healthGateAttemptTracker {
	return &healthGateAttemptTracker{outbound: outbound, gate: gate, policy: policy}
}

func (m *healthGateAttemptTracker) Name() string { return "health-gate-attempt" }

func (m *healthGateAttemptTracker) enabled() bool {
	return m.gate != nil && m.outbound != nil && m.outbound.state != nil &&
		m.outbound.state.RoutingPolicy.LoadBalancerStrategy == biz.LoadBalancerStrategyHealthGated
}

func (m *healthGateAttemptTracker) finishActive(ctx context.Context) {
	if !m.active {
		return
	}
	ctx, cancel := xcontext.DetachWithTimeout(ctx, 10*time.Second)
	defer cancel()
	state := m.outbound.state
	resolve := currentHealthGateConfig(ctx, state.RetryPolicyProvider, state.ChannelService, m.channel, m.policy)
	if err := m.gate.Finish(ctx, m.ticket, resolve, m.lastOutcome, m.lastInfo); err != nil {
		log.Warn(ctx, "failed to finalize health gate attempt", log.Cause(err))
	}
	m.active = false
}

func (m *healthGateAttemptTracker) OnOutboundRawRequest(ctx context.Context, request *httpclient.Request) (*httpclient.Request, error) {
	if !m.enabled() {
		return request, nil
	}
	channel := m.outbound.GetCurrentChannel()
	if channel == nil {
		return request, nil
	}
	key := biz.HealthGateKey{ChannelID: channel.ID, ActualModel: m.outbound.GetCurrentModelID()}
	if key.ActualModel == "" {
		return request, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.rawStream = nil
	m.stream = nil
	if m.active && m.key == key {
		m.lastOutcome = biz.HealthGateOutcomeNeutral
		m.lastInfo = biz.HealthGateErrorInfo{}
		return request, nil
	}
	m.finishActive(ctx)
	candidate := m.outbound.state.CurrentCandidate
	info := candidate.healthGate
	if info != nil {
		m.decision = info.decision
		if info.lastResort && m.decision != nil {
			m.decision.record.LastResort = true
		}
	}
	resolve := currentHealthGateConfig(ctx, m.outbound.state.RetryPolicyProvider, m.outbound.state.ChannelService, channel, m.policy)
	allowProbe, lastResort := false, false
	if info != nil {
		allowProbe, lastResort = info.probeEligible, info.lastResort
	}
	m.lastResort = lastResort
	m.lastOutcome = biz.HealthGateOutcomeNeutral
	m.lastInfo = biz.HealthGateErrorInfo{}
	ticket, ok, err := m.gate.Begin(ctx, key, resolve, allowProbe, lastResort)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errSkipCandidateByHealthGate
	}
	m.key, m.ticket, m.active = key, ticket, true
	m.channel = channel
	return request, nil
}

func (m *healthGateAttemptTracker) OnOutboundRawError(ctx context.Context, err error) {
	if !m.enabled() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		outcome := biz.HealthGateOutcomeNeutral
		if !healthGateClientCanceled(ctx) {
			outcome = classifyHealthGateOutcome(ctx, err)
			if m.outbound.state.OutboundStreamTerminal == streamTerminalFailed {
				outcome = biz.HealthGateOutcomeFailure
			}
			if outcome == biz.HealthGateOutcomeFailure && m.stream != nil && m.stream.meaningful {
				outcome = biz.HealthGateOutcomeUnstable
			}
		}
		if outcome == biz.HealthGateOutcomeNeutral &&
			(m.lastOutcome == biz.HealthGateOutcomeFailure || m.lastOutcome == biz.HealthGateOutcomeUnstable) {
			return
		}
		m.lastOutcome = outcome
		m.lastInfo = healthGateErrorInfo(err)
	}
}

func healthGateErrorInfo(err error) biz.HealthGateErrorInfo {
	if err == nil {
		return biz.HealthGateErrorInfo{}
	}
	return biz.HealthGateErrorInfo{Message: ExtractErrorMessage(err), StatusCode: ExtractStatusCodeFromError(err)}
}

func (m *healthGateAttemptTracker) OnOutboundRawStream(_ context.Context, stream streams.Stream[*httpclient.StreamEvent]) (streams.Stream[*httpclient.StreamEvent], error) {
	if m.enabled() {
		m.mu.Lock()
		m.rawStream = stream
		m.mu.Unlock()
	}
	return stream, nil
}

func (m *healthGateAttemptTracker) OnOutboundLlmStream(ctx context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	if !m.enabled() {
		return stream, nil
	}
	observed := &healthGateStream{stream: stream, ctx: ctx}
	m.mu.Lock()
	m.stream = observed
	m.mu.Unlock()
	return observed, nil
}

// finalize is called after Process. A delivered stream is finalized only on Close.
func (m *healthGateAttemptTracker) finalize(ctx context.Context, err error, stream bool) error {
	if !m.enabled() {
		return err
	}
	m.mu.Lock()
	if m.decision == nil {
		m.decision = healthGateDecisionFromContext(ctx)
	}
	defer m.mu.Unlock()
	if err == nil && stream {
		m.committed = true
		return nil
	}
	if err == nil {
		m.lastOutcome = biz.HealthGateOutcomeSuccess
		m.lastInfo = biz.HealthGateErrorInfo{}
		if healthGateClientCanceled(ctx) {
			m.lastOutcome = biz.HealthGateOutcomeNeutral
		}
	} else if m.active {
		// Preserve failure evidence observed before downstream cancellation. A
		// canceled attempt without such evidence remains neutral.
		if healthGateClientCanceled(ctx) {
			if m.lastOutcome != biz.HealthGateOutcomeFailure && m.lastOutcome != biz.HealthGateOutcomeUnstable {
				m.lastOutcome = biz.HealthGateOutcomeNeutral
				m.lastInfo = healthGateErrorInfo(err)
			}
		} else {
			// The final error may add synthesized timeout or empty-response evidence.
			m.lastOutcome = classifyHealthGateOutcome(ctx, err)
			m.lastInfo = healthGateErrorInfo(err)
		}
	}
	failure := m.lastResort && m.lastOutcome == biz.HealthGateOutcomeFailure && !healthGateClientCanceled(ctx)
	m.finishActive(ctx)
	m.finishSession(ctx, err == nil && !healthGateClientCanceled(ctx))
	if err != nil && (failure || errors.Is(err, errSkipCandidateByHealthGate)) {
		return healthGateUnavailableError()
	}
	return err
}

//nolint:containedctx // The execution context retains lifecycle and timeout provenance.
type healthGateStream struct {
	stream     streams.Stream[*llm.Response]
	ctx        context.Context
	meaningful bool
	once       sync.Once
	closeErr   error
}

func (s *healthGateStream) Next() bool { return s.stream.Next() }
func (s *healthGateStream) Err() error { return s.stream.Err() }
func (s *healthGateStream) Current() *llm.Response {
	response := s.stream.Current()
	if hasMeaningfulStreamOutput(response) {
		s.meaningful = true
	}
	return response
}

func (s *healthGateStream) Close() error {
	s.once.Do(func() {
		s.closeErr = s.stream.Close()
	})
	return s.closeErr
}
