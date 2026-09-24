package orchestrator

import (
	"context"

	"github.com/looplj/axonhub/internal/server/biz"
)

// WithHealthGate enables per-channel, per-upstream-model routing for this balancer.
func (lb *LoadBalancer) WithHealthGate(gate *biz.HealthGate) *LoadBalancer {
	lb.healthGate = gate
	return lb
}

// Resolve against current settings, not the candidate snapshot: a delayed
// request must never roll back a newer health-gate configuration cycle.
func currentHealthGateConfig(
	ctx context.Context,
	provider RetryPolicyProvider,
	service *biz.ChannelService,
	channel *biz.Channel,
	fallback biz.HealthGatePolicy,
) biz.HealthGateConfigResolver {
	return func() (biz.HealthGateConfig, bool) {
		currentChannel := channel
		if service != nil && channel != nil {
			currentChannel = service.GetEnabledChannel(channel.ID)
			if currentChannel == nil {
				return biz.HealthGateConfig{}, false
			}
		}
		currentPolicy := fallback
		if provider != nil {
			currentPolicy = provider.RetryPolicyOrDefault(context.WithoutCancel(ctx)).HealthGateOrDefault()
		}
		return biz.ResolveHealthGateConfig(currentPolicy, currentChannel), true
	}
}
