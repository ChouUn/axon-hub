package biz

// HealthGatePolicy controls health-gated routing and its recovery probes.
type HealthGatePolicy struct {
	FailureThreshold       int `json:"failure_threshold"`
	OpenDurationSeconds    int `json:"open_duration_seconds"`
	MaxOpenDurationSeconds int `json:"max_open_duration_seconds"`
	ProbeSuccessThreshold  int `json:"probe_success_threshold"`
	UnstableWindowSeconds  int `json:"unstable_window_seconds"`
}

func DefaultHealthGatePolicy() HealthGatePolicy {
	return HealthGatePolicy{
		FailureThreshold:       5,
		OpenDurationSeconds:    300,
		MaxOpenDurationSeconds: 3600,
		ProbeSuccessThreshold:  2,
		UnstableWindowSeconds:  300,
	}
}

func (p *RetryPolicy) HealthGateOrDefault() HealthGatePolicy {
	if p == nil || p.HealthGate == nil {
		return DefaultHealthGatePolicy()
	}

	return normalizedHealthGatePolicy(*p.HealthGate)
}

func normalizeHealthGatePolicy(p *RetryPolicy) {
	if p == nil {
		return
	}
	if p.HealthGate == nil {
		policy := DefaultHealthGatePolicy()
		p.HealthGate = &policy
		return
	}
	*p.HealthGate = normalizedHealthGatePolicy(*p.HealthGate)
}

func normalizedHealthGatePolicy(policy HealthGatePolicy) HealthGatePolicy {
	defaults := DefaultHealthGatePolicy()
	if policy.FailureThreshold < 0 {
		policy.FailureThreshold = 0
	}
	if policy.OpenDurationSeconds <= 0 {
		policy.OpenDurationSeconds = defaults.OpenDurationSeconds
	}
	if policy.MaxOpenDurationSeconds < policy.OpenDurationSeconds {
		policy.MaxOpenDurationSeconds = policy.OpenDurationSeconds
	}
	if policy.ProbeSuccessThreshold <= 0 {
		policy.ProbeSuccessThreshold = defaults.ProbeSuccessThreshold
	}
	if policy.UnstableWindowSeconds <= 0 {
		policy.UnstableWindowSeconds = defaults.UnstableWindowSeconds
	}
	return policy
}
