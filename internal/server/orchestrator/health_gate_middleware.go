package orchestrator

import (
	"context"
	"errors"
	"sync"

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
	state := m.outbound.state
	resolve := currentHealthGateConfig(ctx, state.RetryPolicyProvider, state.ChannelService, m.channel, m.policy)
	m.gate.Finish(m.ticket, resolve, m.lastOutcome, m.lastInfo)
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
	if m.active && m.key == key {
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
	ticket, ok := m.gate.Begin(key, resolve, allowProbe, lastResort)
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
		m.lastOutcome = classifyHealthGateOutcome(ctx, err)
		m.lastInfo = healthGateErrorInfo(err)
	}
}

func healthGateErrorInfo(err error) biz.HealthGateErrorInfo {
	if err == nil {
		return biz.HealthGateErrorInfo{}
	}
	return biz.HealthGateErrorInfo{Message: ExtractErrorMessage(err), StatusCode: ExtractStatusCodeFromError(err)}
}

func (m *healthGateAttemptTracker) OnOutboundLlmStream(ctx context.Context, stream streams.Stream[*llm.Response]) (streams.Stream[*llm.Response], error) {
	if !m.enabled() {
		return stream, nil
	}
	return &healthGateStream{stream: stream, ctx: ctx, tracker: m, state: m.outbound.state}, nil
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
		// The final error can be synthesized after the raw error callback (timeouts,
		// empty bodies); use its classification when it carries new information.
		m.lastOutcome = classifyHealthGateOutcome(ctx, err)
		m.lastInfo = healthGateErrorInfo(err)
	}
	failure := m.lastResort && m.lastOutcome == biz.HealthGateOutcomeFailure
	m.finishActive(ctx)
	m.finishSession(ctx, err == nil && !healthGateClientCanceled(ctx))
	if err != nil && (failure || errors.Is(err, errSkipCandidateByHealthGate)) {
		return healthGateUnavailableError()
	}
	return err
}

//nolint:containedctx // The original request context identifies client cancellation.
type healthGateStream struct {
	stream     streams.Stream[*llm.Response]
	ctx        context.Context
	tracker    *healthGateAttemptTracker
	state      *PersistenceState
	meaningful bool
	once       sync.Once
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
	var closeErr error
	s.once.Do(func() {
		closeErr = s.stream.Close()
		m := s.tracker
		m.mu.Lock()
		defer m.mu.Unlock()
		if !m.active {
			if m.committed {
				m.finishSession(s.ctx, false)
			}
			return
		}
		var outcome biz.HealthGateOutcome
		switch {
		case s.state.StreamCompleted:
			outcome = biz.HealthGateOutcomeSuccess
		case healthGateClientCanceled(s.ctx):
			outcome = biz.HealthGateOutcomeNeutral
		case s.meaningful:
			outcome = biz.HealthGateOutcomeUnstable
		default:
			outcome = biz.HealthGateOutcomeFailure
		}
		m.lastOutcome = outcome
		streamErr := s.stream.Err()
		if streamErr == nil {
			streamErr = closeErr
		}
		if streamErr == nil && (outcome == biz.HealthGateOutcomeFailure || outcome == biz.HealthGateOutcomeUnstable) {
			streamErr = ErrStreamIncomplete
		}
		m.lastInfo = healthGateErrorInfo(streamErr)
		if m.committed {
			m.finishActive(s.ctx)
			m.finishSession(s.ctx, outcome == biz.HealthGateOutcomeSuccess && !healthGateClientCanceled(s.ctx))
		}
	})
	return closeErr
}
