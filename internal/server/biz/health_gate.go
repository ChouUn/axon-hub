package biz

import (
	"math"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

type HealthGateConfig struct {
	FailureThreshold      int
	OpenDuration          time.Duration
	MaxOpenDuration       time.Duration
	ProbeSuccessThreshold int
	UnstableWindow        time.Duration
}

func (c HealthGateConfig) Disabled() bool { return c.FailureThreshold <= 0 }

// HealthGateConfigResolver fetches the current channel configuration; false means unavailable.
type HealthGateConfigResolver func() (cfg HealthGateConfig, current bool)

func ResolveHealthGateConfig(policy HealthGatePolicy, ch *Channel) HealthGateConfig {
	policy = normalizedHealthGatePolicy(policy)
	cfg := HealthGateConfig{
		FailureThreshold:      policy.FailureThreshold,
		OpenDuration:          healthGateSeconds(policy.OpenDurationSeconds),
		MaxOpenDuration:       healthGateSeconds(policy.MaxOpenDurationSeconds),
		ProbeSuccessThreshold: policy.ProbeSuccessThreshold,
		UnstableWindow:        healthGateSeconds(policy.UnstableWindowSeconds),
	}
	if ch != nil && ch.Channel != nil && ch.Settings != nil &&
		ch.Settings.HealthGateFailureThreshold != nil && *ch.Settings.HealthGateFailureThreshold >= 0 {
		cfg.FailureThreshold = *ch.Settings.HealthGateFailureThreshold
	}
	return cfg
}

func healthGateSeconds(seconds int) time.Duration {
	if seconds > math.MaxInt64/int(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(seconds) * time.Second
}

type HealthGateKey struct {
	ChannelID   int
	ActualModel string
}

type HealthGateState string

const (
	HealthGateStateHealthy  HealthGateState = "healthy"
	HealthGateStateUnstable HealthGateState = "unstable"
	HealthGateStateOpen     HealthGateState = "open"
	HealthGateStateProbing  HealthGateState = "probing"
)

type HealthGateOutcome int

const (
	HealthGateOutcomeNeutral HealthGateOutcome = iota
	HealthGateOutcomeSuccess
	HealthGateOutcomeFailure
	HealthGateOutcomeUnstable
)

type HealthGateErrorInfo struct {
	Message    string
	StatusCode int
}

type HealthGateView struct {
	State     HealthGateState
	OpenUntil time.Time
	ProbeBusy bool
}

type HealthGateTicket struct {
	key     HealthGateKey
	gen     uint64
	probeID uint64
	noop    bool
	// cfg is the configuration accepted at Begin; Finish falls back to it when
	// the current configuration is unavailable.
	cfg HealthGateConfig
}

func (t HealthGateTicket) IsProbe() bool { return t.probeID != 0 && !t.noop }

type HealthGateModelSnapshot struct {
	ActualModel         string
	State               HealthGateState
	ConsecutiveFailures int
	ProbeSuccesses      int
	BackoffLevel        int
	LastError           string
	LastStatusCode      int
	LastErrorAt         *time.Time
	OpenUntil           *time.Time
}

type healthGateEntry struct {
	gen                  uint64
	disabled             bool
	open                 bool
	openUntil            time.Time
	consecutiveFailures  int
	probeSuccesses       int
	probeHolder          uint64
	backoffLevel         int
	everOpened           bool
	recoveredAt          time.Time
	lastCountedFailureAt time.Time
	lastUnstableAt       time.Time
	lastError            string
	lastStatusCode       int
	lastErrorAt          time.Time
	// cfg is the last configuration observed for this entry.
	cfg HealthGateConfig
}

type HealthGate struct {
	// Lock order: channel observation lock, then mu. Resolvers never run under mu.
	obsMu       sync.Mutex
	obsLocks    map[int]*sync.Mutex
	mu          sync.Mutex
	entries     map[HealthGateKey]*healthGateEntry
	nextProbeID uint64
	now         func() time.Time
}

func NewHealthGate(now func() time.Time) *HealthGate {
	if now == nil {
		now = time.Now
	}
	return &HealthGate{entries: make(map[HealthGateKey]*healthGateEntry), now: now}
}

func (svc *ChannelService) HealthGate() *HealthGate {
	svc.healthGateOnce.Do(func() { svc.healthGate = NewHealthGate(nil) })
	return svc.healthGate
}

func (g *HealthGate) observationLock(channelID int) *sync.Mutex {
	g.obsMu.Lock()
	defer g.obsMu.Unlock()
	if g.obsLocks == nil {
		g.obsLocks = make(map[int]*sync.Mutex)
	}
	lock := g.obsLocks[channelID]
	if lock == nil {
		lock = &sync.Mutex{}
		g.obsLocks[channelID] = lock
	}
	return lock
}

// observe applies configuration transitions before any state is read or changed. Call with mu held.
func (g *HealthGate) observe(key HealthGateKey, cfg HealthGateConfig, now time.Time, create bool) *healthGateEntry {
	entry := g.entries[key]
	if entry == nil {
		if cfg.Disabled() || !create {
			return nil
		}
		entry = &healthGateEntry{}
		g.entries[key] = entry
	}
	if entry.disabled != cfg.Disabled() {
		gen := entry.gen + 1
		*entry = healthGateEntry{gen: gen, disabled: cfg.Disabled()}
	}
	// Remember the accepted configuration so reads can still derive windows
	// when the current configuration is temporarily unavailable.
	entry.cfg = cfg
	if !entry.disabled && !entry.open && entry.everOpened {
		since := entry.recoveredAt
		if entry.lastCountedFailureAt.After(since) {
			since = entry.lastCountedFailureAt
		}
		if now.Sub(since) > cfg.MaxOpenDuration {
			entry.everOpened = false
			entry.backoffLevel = 0
		}
	}
	return entry
}

func healthGateState(entry *healthGateEntry, cfg HealthGateConfig, now time.Time) HealthGateState {
	if entry == nil || entry.disabled {
		return HealthGateStateHealthy
	}
	if entry.open {
		if now.Before(entry.openUntil) {
			return HealthGateStateOpen
		}
		return HealthGateStateProbing
	}
	if !entry.lastUnstableAt.IsZero() && !now.Before(entry.lastUnstableAt) &&
		now.Sub(entry.lastUnstableAt) <= cfg.UnstableWindow {
		return HealthGateStateUnstable
	}
	return HealthGateStateHealthy
}

func (g *HealthGate) Inspect(key HealthGateKey, resolve HealthGateConfigResolver) HealthGateView {
	observation := g.observationLock(key.ChannelID)
	observation.Lock()
	defer observation.Unlock()
	cfg, current := resolve()
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	entry := g.entries[key]
	if current {
		entry = g.observe(key, cfg, now, false)
	}
	if entry == nil || entry.disabled {
		return HealthGateView{State: HealthGateStateHealthy}
	}
	stateCfg := cfg
	if !current {
		stateCfg = entry.cfg
	}
	view := HealthGateView{State: healthGateState(entry, stateCfg, now)}
	if entry.open {
		view.OpenUntil = entry.openUntil
		view.ProbeBusy = entry.probeHolder != 0
	}
	return view
}

func (g *HealthGate) Begin(key HealthGateKey, resolve HealthGateConfigResolver, allowProbe, lastResort bool) (HealthGateTicket, bool) {
	observation := g.observationLock(key.ChannelID)
	observation.Lock()
	defer observation.Unlock()
	cfg, current := resolve()
	if !current {
		return HealthGateTicket{}, false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	entry := g.observe(key, cfg, now, true)
	if cfg.Disabled() {
		return HealthGateTicket{key: key, noop: true}, true
	}
	ticket := HealthGateTicket{key: key, gen: entry.gen, cfg: cfg}
	switch healthGateState(entry, cfg, now) {
	case HealthGateStateOpen:
		if !lastResort || entry.probeHolder != 0 {
			return HealthGateTicket{}, false
		}
		entry.openUntil = now
	case HealthGateStateProbing:
		if (!allowProbe && !lastResort) || entry.probeHolder != 0 {
			return HealthGateTicket{}, false
		}
	default:
		return ticket, true
	}
	g.nextProbeID++
	if g.nextProbeID == 0 {
		g.nextProbeID++
	}
	entry.probeHolder = g.nextProbeID
	ticket.probeID = g.nextProbeID
	return ticket, true
}

func healthGateOpenDuration(cfg HealthGateConfig, level int) time.Duration {
	limit := cfg.MaxOpenDuration
	if cfg.OpenDuration >= limit {
		return limit
	}
	duration := cfg.OpenDuration
	for range level {
		if duration >= limit {
			break
		}
		if duration > limit/2 {
			return limit
		}
		duration *= 2
	}
	return duration
}

func healthGateRecordError(entry *healthGateEntry, now time.Time, info HealthGateErrorInfo) {
	if utf8.RuneCountInString(info.Message) > 512 {
		info.Message = string([]rune(info.Message)[:512])
	}
	entry.lastError = info.Message
	entry.lastStatusCode = info.StatusCode
	entry.lastErrorAt = now
}

func (g *HealthGate) Finish(t HealthGateTicket, resolve HealthGateConfigResolver, outcome HealthGateOutcome, info HealthGateErrorInfo) {
	observation := g.observationLock(t.key.ChannelID)
	observation.Lock()
	defer observation.Unlock()
	cfg, current := resolve()
	g.mu.Lock()
	defer g.mu.Unlock()
	entry := g.entries[t.key]
	if t.noop || entry == nil || entry.gen != t.gen {
		return
	}
	now := g.now()
	if current {
		entry = g.observe(t.key, cfg, now, false)
	} else {
		cfg = t.cfg
	}
	if entry.disabled || entry.gen != t.gen {
		return
	}
	if t.IsProbe() {
		if entry.probeHolder != t.probeID {
			return
		}
		entry.probeHolder = 0
		switch outcome {
		case HealthGateOutcomeSuccess:
			entry.probeSuccesses++
			if entry.probeSuccesses >= cfg.ProbeSuccessThreshold {
				entry.open = false
				entry.consecutiveFailures = 0
				entry.probeSuccesses = 0
				entry.recoveredAt = now
			}
		case HealthGateOutcomeFailure:
			if healthGateOpenDuration(cfg, entry.backoffLevel) < cfg.MaxOpenDuration {
				entry.backoffLevel++
			}
			entry.openUntil = now.Add(healthGateOpenDuration(cfg, entry.backoffLevel))
			entry.probeSuccesses = 0
			entry.lastCountedFailureAt = now
			entry.lastUnstableAt = now
			healthGateRecordError(entry, now, info)
		case HealthGateOutcomeUnstable:
			entry.lastUnstableAt = now
			healthGateRecordError(entry, now, info)
		case HealthGateOutcomeNeutral:
			if info.Message != "" || info.StatusCode != 0 {
				healthGateRecordError(entry, now, info)
			}
		}
		return
	}
	if entry.open {
		if outcome == HealthGateOutcomeFailure || outcome == HealthGateOutcomeUnstable {
			healthGateRecordError(entry, now, info)
		}
		return
	}
	switch outcome {
	case HealthGateOutcomeSuccess:
		entry.consecutiveFailures = 0
	case HealthGateOutcomeFailure:
		entry.consecutiveFailures++
		entry.lastCountedFailureAt = now
		entry.lastUnstableAt = now
		healthGateRecordError(entry, now, info)
		if entry.consecutiveFailures >= cfg.FailureThreshold {
			entry.open = true
			if entry.everOpened && healthGateOpenDuration(cfg, entry.backoffLevel) < cfg.MaxOpenDuration {
				entry.backoffLevel++
			} else if !entry.everOpened {
				entry.backoffLevel = 0
			}
			entry.everOpened = true
			entry.openUntil = now.Add(healthGateOpenDuration(cfg, entry.backoffLevel))
			entry.probeSuccesses = 0
		}
	case HealthGateOutcomeUnstable:
		entry.lastUnstableAt = now
		healthGateRecordError(entry, now, info)
	}
}

func (g *HealthGate) Snapshot(channelID int, resolve HealthGateConfigResolver) []HealthGateModelSnapshot {
	observation := g.observationLock(channelID)
	observation.Lock()
	defer observation.Unlock()
	cfg, current := resolve()
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.now()
	var snapshots []HealthGateModelSnapshot
	for key := range g.entries {
		if key.ChannelID != channelID {
			continue
		}
		entry := g.entries[key]
		if current {
			entry = g.observe(key, cfg, now, false)
		}
		stateCfg := cfg
		if !current {
			stateCfg = entry.cfg
		}
		if entry.disabled || (current && cfg.Disabled()) {
			continue
		}
		snapshot := HealthGateModelSnapshot{
			ActualModel:         key.ActualModel,
			State:               healthGateState(entry, stateCfg, now),
			ConsecutiveFailures: entry.consecutiveFailures,
			ProbeSuccesses:      entry.probeSuccesses,
			BackoffLevel:        entry.backoffLevel,
			LastError:           entry.lastError,
			LastStatusCode:      entry.lastStatusCode,
		}
		if !entry.lastErrorAt.IsZero() {
			at := entry.lastErrorAt
			snapshot.LastErrorAt = &at
		}
		if entry.open {
			until := entry.openUntil
			snapshot.OpenUntil = &until
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].ActualModel < snapshots[j].ActualModel })
	return snapshots
}

func (g *HealthGate) Reset(channelID int, actualModel string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, entry := range g.entries {
		if key.ChannelID == channelID && (actualModel == "" || key.ActualModel == actualModel) {
			gen := entry.gen + 1
			*entry = healthGateEntry{gen: gen, disabled: entry.disabled}
		}
	}
}

func (g *HealthGate) ChannelIDsWithEntries() []int {
	g.mu.Lock()
	defer g.mu.Unlock()
	ids := make(map[int]struct{})
	for key, entry := range g.entries {
		if !entry.disabled {
			ids[key.ChannelID] = struct{}{}
		}
	}
	result := make([]int, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}
