package biz

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/objects"
)

type healthGateClock struct{ at time.Time }

func (c *healthGateClock) advance(d time.Duration) { c.at = c.at.Add(d) }

func healthGateTestResolver(cfg HealthGateConfig) HealthGateConfigResolver {
	return func() (HealthGateConfig, bool) { return cfg, true }
}

func testHealthGate(t *testing.T) (*HealthGate, *healthGateClock, HealthGateConfig, HealthGateKey) {
	t.Helper()
	clock := &healthGateClock{at: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	gate := NewHealthGate(func() time.Time { return clock.at })
	cfg := HealthGateConfig{FailureThreshold: 2, OpenDuration: time.Minute, MaxOpenDuration: 4 * time.Minute, ProbeSuccessThreshold: 2, UnstableWindow: 30 * time.Second}
	return gate, clock, cfg, HealthGateKey{ChannelID: 7, ActualModel: "upstream"}
}

func finishHealthGate(t *testing.T, gate *HealthGate, key HealthGateKey, cfg HealthGateConfig, outcome HealthGateOutcome) {
	t.Helper()
	ticket, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	if !ok || ticket.IsProbe() {
		t.Fatalf("normal attempt was unexpectedly gated: ok=%t, probe=%t", ok, ticket.IsProbe())
	}
	gate.Finish(ticket, healthGateTestResolver(cfg), outcome, HealthGateErrorInfo{Message: "provider unavailable", StatusCode: 503})
}

func requireHealthGateState(t *testing.T, gate *HealthGate, key HealthGateKey, cfg HealthGateConfig, state HealthGateState) {
	t.Helper()
	if view := gate.Inspect(key, healthGateTestResolver(cfg)); view.State != state {
		t.Fatalf("gate state = %s, want %s", view.State, state)
	}
}

func TestHealthGateFailuresAndRecovery(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	requireHealthGateState(t, gate, key, cfg, HealthGateStateUnstable)
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeSuccess)
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	if got := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0].ConsecutiveFailures; got != 1 {
		t.Fatalf("success did not clear earlier failures: got %d", got)
	}
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	requireHealthGateState(t, gate, key, cfg, HealthGateStateOpen)
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); ok {
		t.Fatal("open gate admitted normal probe before its deadline")
	}
	probe, ok := gate.Begin(key, healthGateTestResolver(cfg), false, true)
	if !ok || !probe.IsProbe() {
		t.Fatal("last resort should admit exactly one probe before the deadline")
	}
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), false, true); ok {
		t.Fatal("occupied probe slot admitted another last resort")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{})
	clock.advance(time.Minute)
	requireHealthGateState(t, gate, key, cfg, HealthGateStateProbing)
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false); ok {
		t.Fatal("probe without eligibility was admitted")
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok || !probe.IsProbe() {
		t.Fatal("eligible probe was rejected")
	}
	if view := gate.Inspect(key, healthGateTestResolver(cfg)); !view.ProbeBusy {
		t.Fatal("probe slot did not become busy")
	}
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); ok {
		t.Fatal("concurrent probe was admitted")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeUnstable, HealthGateErrorInfo{Message: "stream interrupted"})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ProbeSuccesses != 0 || snap.State != HealthGateStateProbing || snap.LastError != "stream interrupted" {
		t.Fatalf("unstable probe changed progress or lost error: %+v", snap)
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("probe slot was not released")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("probe slot was not released")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{Message: "not counted"})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ProbeSuccesses != 1 || snap.State != HealthGateStateProbing {
		t.Fatalf("neutral probe erased recovery progress: %+v", snap)
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("neutral probe did not release its slot")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	requireHealthGateState(t, gate, key, cfg, HealthGateStateUnstable)
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ConsecutiveFailures != 0 || snap.ProbeSuccesses != 0 {
		t.Fatalf("recovery did not reset failure/probe counters: %+v", snap)
	}
	clock.advance(cfg.UnstableWindow + time.Nanosecond)
	requireHealthGateState(t, gate, key, cfg, HealthGateStateHealthy)
}

func TestHealthGateEarlyLastResortEntersProbing(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	initialUntil := gate.Inspect(key, healthGateTestResolver(cfg)).OpenUntil
	probe, ok := gate.Begin(key, healthGateTestResolver(cfg), false, true)
	if !ok || !probe.IsProbe() {
		t.Fatal("early last resort did not acquire a probe")
	}
	if view := gate.Inspect(key, healthGateTestResolver(cfg)); view.State != HealthGateStateProbing || !view.ProbeBusy {
		t.Fatalf("early probe did not enter probing state: %+v", view)
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{})
	if view := gate.Inspect(key, healthGateTestResolver(cfg)); view.State != HealthGateStateProbing || view.ProbeBusy {
		t.Fatalf("neutral early probe restored the open gate: %+v", view)
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok || !probe.IsProbe() {
		t.Fatal("eligible probe was not available after neutral outcome")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.State != HealthGateStateProbing || snap.ProbeSuccesses != 1 {
		t.Fatalf("first success prematurely closed early probe: %+v", snap)
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok || !probe.IsProbe() {
		t.Fatal("eligible probe was not available after partial success")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{StatusCode: 503})
	view := gate.Inspect(key, healthGateTestResolver(cfg))
	if view.State != HealthGateStateOpen || !view.OpenUntil.Equal(clock.at.Add(2*cfg.OpenDuration)) || !view.OpenUntil.After(initialUntil) {
		t.Fatalf("failed early probe did not reopen with doubled duration: %+v", view)
	}
}

func TestHealthGateBackoffAndQuietReset(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	for level, duration := range []time.Duration{2 * time.Minute, 4 * time.Minute, 4 * time.Minute} {
		clock.advance(duration / 2) // Last resort can fail before the ordinary deadline.
		probe, ok := gate.Begin(key, healthGateTestResolver(cfg), false, true)
		if !ok {
			t.Fatal("last resort was rejected")
		}
		gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{})
		snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]
		if snap.OpenUntil == nil || !snap.OpenUntil.Equal(clock.at.Add(duration)) {
			t.Fatalf("failure #%d reopened until %v, want %v", level+1, snap.OpenUntil, clock.at.Add(duration))
		}
	}
	clock.advance(cfg.MaxOpenDuration)
	for range cfg.ProbeSuccessThreshold {
		probe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("recovery probe was rejected")
		}
		gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	}
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.OpenUntil == nil || !snap.OpenUntil.Equal(clock.at.Add(cfg.MaxOpenDuration)) {
		t.Fatalf("reopened gate forgot capped backoff: %+v", snap)
	}
	clock.advance(cfg.MaxOpenDuration)
	for range cfg.ProbeSuccessThreshold {
		probe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("recovery probe was rejected")
		}
		gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	}
	clock.advance(cfg.MaxOpenDuration + time.Nanosecond)
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.BackoffLevel != 0 {
		t.Fatalf("quiet period did not clear backoff: %+v", snap)
	}
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.OpenUntil == nil || !snap.OpenUntil.Equal(clock.at.Add(cfg.OpenDuration)) {
		t.Fatalf("fresh circuit did not use initial duration: %+v", snap)
	}
}

func TestHealthGateReopenDoublesAfterRecovery(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	clock.advance(cfg.OpenDuration)
	for range cfg.ProbeSuccessThreshold {
		probe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("recovery probe rejected")
		}
		gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	}
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.BackoffLevel != 1 || snap.OpenUntil == nil || !snap.OpenUntil.Equal(clock.at.Add(2*cfg.OpenDuration)) {
		t.Fatalf("reopen discarded retained backoff: %+v", snap)
	}
}

func TestHealthGateLateTicketsCannotMutateOrReleaseProbe(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	late, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	if !ok {
		t.Fatal("initial attempt rejected")
	}
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	gate.Finish(late, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.State != HealthGateStateOpen || snap.ConsecutiveFailures != cfg.FailureThreshold {
		t.Fatalf("late normal success changed open circuit: %+v", snap)
	}
	clock.advance(cfg.OpenDuration)
	oldProbe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("probe rejected")
	}
	gate.Reset(key.ChannelID, key.ActualModel)
	gate.Finish(oldProbe, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale"})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.State != HealthGateStateHealthy || snap.LastError != "" {
		t.Fatalf("reset admitted old result: %+v", snap)
	}
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	clock.advance(cfg.OpenDuration)
	newProbe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("new probe rejected")
	}
	gate.Finish(oldProbe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); ok {
		t.Fatal("old probe released new generation's slot")
	}
	gate.Finish(newProbe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); !ok {
		t.Fatal("valid probe failed to release its slot")
	}
}

func TestHealthGateThresholdTransitionsAndSnapshots(t *testing.T) {
	gate, _, cfg, key := testHealthGate(t)
	other := HealthGateKey{ChannelID: 3, ActualModel: "alpha"}
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	finishHealthGate(t, gate, other, cfg, HealthGateOutcomeFailure)
	if ids := gate.ChannelIDsWithEntries(); len(ids) != 2 || ids[0] != 3 || ids[1] != 7 {
		t.Fatalf("entry channel IDs not sorted: %v", ids)
	}
	stale, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	if !ok {
		t.Fatal("active attempt rejected")
	}
	disabled := cfg
	disabled.FailureThreshold = 0
	if view := gate.Inspect(key, healthGateTestResolver(disabled)); view.State != HealthGateStateHealthy {
		t.Fatalf("disabled gate still abnormal: %+v", view)
	}
	if snapshots := gate.Snapshot(key.ChannelID, healthGateTestResolver(disabled)); snapshots != nil {
		t.Fatalf("disabled gate exposed prior entries: %+v", snapshots)
	}
	if ids := gate.ChannelIDsWithEntries(); len(ids) != 1 || ids[0] != other.ChannelID {
		t.Fatalf("disabled channel retained visible entries: %v", ids)
	}
	noop, ok := gate.Begin(key, healthGateTestResolver(disabled), false, false)
	if !ok || noop.IsProbe() {
		t.Fatal("disabled gate blocked ordinary traffic")
	}
	gate.Finish(noop, healthGateTestResolver(disabled), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "ignored"})
	active, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	if !ok {
		t.Fatal("re-enabled gate rejected a fresh attempt")
	}
	gate.Finish(active, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{})
	requireHealthGateState(t, gate, key, cfg, HealthGateStateHealthy)
	gate.Finish(stale, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale"})
	gate.Finish(noop, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale noop"})
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ConsecutiveFailures != 0 || snap.LastError != "" {
		t.Fatalf("re-enabled gate inherited previous generation: %+v", snap)
	}
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ConsecutiveFailures != 1 {
		t.Fatalf("re-enabled gate did not start afresh: %+v", snap)
	}
}

func TestHealthGateStaleFinishCannotChangeConfigurationCycle(t *testing.T) {
	t.Run("old attempt cannot clear new failure count", func(t *testing.T) {
		gate, _, cfg, key := testHealthGate(t)
		old, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
		if !ok {
			t.Fatal("initial attempt rejected")
		}
		disabled := cfg
		disabled.FailureThreshold = 0
		gate.Snapshot(key.ChannelID, healthGateTestResolver(disabled))
		fresh, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
		if !ok {
			t.Fatal("re-enabled attempt rejected")
		}
		gate.Finish(fresh, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "new failure"})
		gate.Finish(old, healthGateTestResolver(disabled), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale failure"})
		gate.Finish(old, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale failure"})
		if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ConsecutiveFailures != 1 || snap.LastError != "new failure" {
			t.Fatalf("stale result changed new statistics: %+v", snap)
		}
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
		if snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snap.ConsecutiveFailures != 2 {
			t.Fatalf("new cycle stopped counting valid attempts: %+v", snap)
		}
	})

	t.Run("old probe cannot clear new probe slot", func(t *testing.T) {
		gate, clock, cfg, key := testHealthGate(t)
		for range cfg.FailureThreshold {
			finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
		}
		clock.advance(cfg.OpenDuration)
		old, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("initial probe rejected")
		}
		disabled := cfg
		disabled.FailureThreshold = 0
		gate.Snapshot(key.ChannelID, healthGateTestResolver(disabled))
		for range cfg.FailureThreshold {
			finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
		}
		clock.advance(cfg.OpenDuration)
		fresh, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("new probe rejected")
		}
		gate.Finish(old, healthGateTestResolver(disabled), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
		gate.Finish(old, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
		if view := gate.Inspect(key, healthGateTestResolver(cfg)); view.State != HealthGateStateProbing || !view.ProbeBusy {
			t.Fatalf("old probe reset or released new slot: %+v", view)
		}
		if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); ok {
			t.Fatal("old probe let a competing probe through")
		}
		gate.Finish(fresh, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{})
		if _, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false); !ok {
			t.Fatal("new probe could not release its slot")
		}
	})
}

func TestHealthGateInspectObservesConfigurationCycle(t *testing.T) {
	gate, _, cfg, key := testHealthGate(t)
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	old, ok := gate.Begin(key, healthGateTestResolver(cfg), false, true)
	if !ok {
		t.Fatal("last-resort probe rejected")
	}
	disabled := cfg
	disabled.FailureThreshold = 0
	if view := gate.Inspect(key, healthGateTestResolver(disabled)); view.State != HealthGateStateHealthy || !view.OpenUntil.IsZero() || view.ProbeBusy {
		t.Fatalf("disabled inspection exposed open circuit: %+v", view)
	}
	gate.Finish(old, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "stale"})
	if view := gate.Inspect(key, healthGateTestResolver(cfg)); view.State != HealthGateStateHealthy {
		t.Fatalf("old open state revived after re-enabling: %+v", view)
	}
	if snapshots := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg)); len(snapshots) != 1 || snapshots[0].ConsecutiveFailures != 0 || snapshots[0].LastError != "" {
		t.Fatalf("old statistics revived after threshold cycle: %+v", snapshots)
	}
}

func TestHealthGateObservationSerializesConfigurationReads(t *testing.T) {
	gate, _, cfg, key := testHealthGate(t)
	for range cfg.FailureThreshold {
		finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	}
	var threshold atomic.Int64
	threshold.Store(0)
	read := make(chan struct{})
	release := make(chan struct{})
	inspected := make(chan struct{})
	go func() {
		defer close(inspected)
		gate.Inspect(key, func() (HealthGateConfig, bool) {
			observed := cfg
			observed.FailureThreshold = int(threshold.Load())
			close(read)
			<-release
			return observed, true
		})
	}()
	<-read
	threshold.Store(1)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		resolver := func() (HealthGateConfig, bool) {
			fresh := cfg
			fresh.FailureThreshold = int(threshold.Load())
			return fresh, true
		}
		ticket, ok := gate.Begin(key, resolver, false, false)
		if !ok {
			return
		}
		gate.Finish(ticket, resolver, HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "new cycle"})
	}()
	close(release)
	<-inspected
	<-finished
	fresh := cfg
	fresh.FailureThreshold = 1
	if snapshot := gate.Snapshot(key.ChannelID, healthGateTestResolver(fresh))[0]; snapshot.ConsecutiveFailures != 1 || snapshot.LastError != "new cycle" {
		t.Fatalf("stale observation erased newer failure: %+v", snapshot)
	}
}

func TestHealthGateUnavailableConfiguration(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	// Production resolvers return a zero config when the channel is unavailable.
	unknown := func() (HealthGateConfig, bool) { return HealthGateConfig{}, false }
	if _, ok := gate.Begin(key, unknown, false, false); ok {
		t.Fatal("begin admitted an unresolvable channel")
	}
	ticket, ok := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	if !ok {
		t.Fatal("valid attempt rejected")
	}
	gate.Finish(ticket, unknown, HealthGateOutcomeFailure, HealthGateErrorInfo{Message: "provider failed"})
	snapshot := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]
	if snapshot.ConsecutiveFailures != 1 || snapshot.LastError != "provider failed" || snapshot.State != HealthGateStateUnstable {
		t.Fatalf("unavailable config must record with the accepted threshold, not open early: %+v", snapshot)
	}
	clock.at = clock.at.Add(time.Second)
	if view := gate.Inspect(key, unknown); view.State != HealthGateStateUnstable {
		t.Fatalf("unavailable config reported %s inside the unstable window", view.State)
	}
	if snapshot := gate.Snapshot(key.ChannelID, unknown)[0]; snapshot.State != HealthGateStateUnstable {
		t.Fatalf("unavailable config snapshot reported %s inside the unstable window", snapshot.State)
	}

	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	clock.at = clock.at.Add(cfg.OpenDuration)
	probe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok || !probe.IsProbe() {
		t.Fatalf("expired open combination did not admit a probe: ok=%t probe=%t", ok, probe.IsProbe())
	}
	gate.Finish(probe, unknown, HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if snapshot := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]; snapshot.State != HealthGateStateProbing || snapshot.ProbeSuccesses != 1 {
		t.Fatalf("one probe success under unavailable config bypassed M=%d: %+v", cfg.ProbeSuccessThreshold, snapshot)
	}
}

func TestHealthGateErrorTruncationAndModelIsolation(t *testing.T) {
	gate, _, cfg, key := testHealthGate(t)
	other := HealthGateKey{ChannelID: key.ChannelID, ActualModel: "aaa"}
	longMessage := strings.Repeat("界", 513)
	ticket, _ := gate.Begin(key, healthGateTestResolver(cfg), false, false)
	gate.Finish(ticket, healthGateTestResolver(cfg), HealthGateOutcomeUnstable, HealthGateErrorInfo{Message: longMessage})
	snap := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))[0]
	if len([]rune(snap.LastError)) != 512 || snap.LastErrorAt == nil || snap.State != HealthGateStateUnstable {
		t.Fatalf("unstable outcome did not retain bounded diagnostic: %+v", snap)
	}
	if view := gate.Inspect(other, healthGateTestResolver(cfg)); view.State != HealthGateStateHealthy {
		t.Fatalf("other model inherited unstable state: %+v", view)
	}
	if models := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg)); len(models) != 1 {
		t.Fatalf("inspecting an unused model created a health record: %+v", models)
	}
	ticket, ok := gate.Begin(other, healthGateTestResolver(cfg), false, false)
	if !ok {
		t.Fatal("other model was unexpectedly gated")
	}
	gate.Finish(ticket, healthGateTestResolver(cfg), HealthGateOutcomeNeutral, HealthGateErrorInfo{})
	models := gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))
	if len(models) != 2 || models[0].ActualModel != "aaa" || models[1].ActualModel != key.ActualModel {
		t.Fatalf("model snapshots not ordered: %+v", models)
	}
}

func TestHealthGatePolicyNormalizationAndOverrides(t *testing.T) {
	defaults := DefaultHealthGatePolicy()
	if got := (*RetryPolicy)(nil).HealthGateOrDefault(); got != defaults {
		t.Fatalf("nil policy did not use defaults: %+v", got)
	}
	cases := []struct {
		name string
		in   HealthGatePolicy
		want HealthGatePolicy
	}{
		{"missing values", HealthGatePolicy{}, HealthGatePolicy{FailureThreshold: 0, OpenDurationSeconds: 300, MaxOpenDurationSeconds: 300, ProbeSuccessThreshold: 2, UnstableWindowSeconds: 300, OwnerFailoverThreshold: 2}},
		{"negative threshold and short max", HealthGatePolicy{FailureThreshold: -1, OpenDurationSeconds: 700, MaxOpenDurationSeconds: 10, ProbeSuccessThreshold: -1, UnstableWindowSeconds: -1}, HealthGatePolicy{FailureThreshold: 0, OpenDurationSeconds: 700, MaxOpenDurationSeconds: 700, ProbeSuccessThreshold: 2, UnstableWindowSeconds: 300, OwnerFailoverThreshold: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := &RetryPolicy{HealthGate: &tc.in}
			normalizeHealthGatePolicy(policy)
			if got := policy.HealthGateOrDefault(); got != tc.want {
				t.Fatalf("normalized policy = %+v, want %+v", got, tc.want)
			}
		})
	}
	policy := &RetryPolicy{}
	normalizeHealthGatePolicy(policy)
	if got := policy.HealthGateOrDefault(); got != defaults {
		t.Fatalf("missing policy did not get defaults: %+v", got)
	}
	zero, negative := 0, -2
	for _, tc := range []struct {
		name     string
		override *int
		want     int
	}{
		{"inherited", nil, defaults.FailureThreshold},
		{"disabled", &zero, 0},
		{"negative ignored", &negative, defaults.FailureThreshold},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{Channel: &ent.Channel{Settings: &objects.ChannelSettings{HealthGateFailureThreshold: tc.override}}}
			cfg := ResolveHealthGateConfig(defaults, channel)
			if cfg.FailureThreshold != tc.want {
				t.Fatalf("threshold = %d, want %d", cfg.FailureThreshold, tc.want)
			}
		})
	}
}

func TestHealthGateTransitionNotifications(t *testing.T) {
	gate, clock, cfg, key := testHealthGate(t)
	var transitions []HealthGateTransition
	gate.SetTransitionHandler(func(transition HealthGateTransition) {
		// Both gate locks must already be released: Snapshot and Inspect reenter them.
		gate.Snapshot(key.ChannelID, healthGateTestResolver(cfg))
		gate.Inspect(key, healthGateTestResolver(cfg))
		transitions = append(transitions, transition)
	})
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	if len(transitions) != 0 {
		t.Fatalf("below threshold produced transition: %+v", transitions)
	}
	finishHealthGate(t, gate, key, cfg, HealthGateOutcomeFailure)
	if len(transitions) != 1 || transitions[0].Kind != "opened" || transitions[0].Key != key || transitions[0].ConsecutiveFailures != cfg.FailureThreshold || transitions[0].FailureThreshold != cfg.FailureThreshold || !transitions[0].OpenUntil.Equal(clock.at.Add(cfg.OpenDuration)) || transitions[0].LastStatusCode != 503 || transitions[0].LastError != "provider unavailable" {
		t.Fatalf("opening transition is missing fields: %+v", transitions)
	}
	clock.advance(cfg.OpenDuration)
	probe, ok := gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("probe rejected")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeFailure, HealthGateErrorInfo{})
	if len(transitions) != 1 {
		t.Fatalf("probe failure emitted transition: %+v", transitions)
	}
	clock.advance(2 * cfg.OpenDuration)
	for range cfg.ProbeSuccessThreshold - 1 {
		probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
		if !ok {
			t.Fatal("probe rejected")
		}
		gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
		if len(transitions) != 1 {
			t.Fatalf("partial probe success emitted transition: %+v", transitions)
		}
	}
	probe, ok = gate.Begin(key, healthGateTestResolver(cfg), true, false)
	if !ok {
		t.Fatal("final recovery probe rejected")
	}
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if len(transitions) != 2 || transitions[1].Kind != "recovered" || transitions[1].Key != key || transitions[1].ProbeSuccesses != cfg.ProbeSuccessThreshold || transitions[1].ProbeSuccessThreshold != cfg.ProbeSuccessThreshold || !transitions[1].OpenUntil.IsZero() {
		t.Fatalf("recovery transition is missing fields: %+v", transitions)
	}
	gate.Reset(key.ChannelID, key.ActualModel)
	disabled := cfg
	disabled.FailureThreshold = 0
	gate.Inspect(key, healthGateTestResolver(disabled))
	gate.Inspect(key, healthGateTestResolver(cfg))
	gate.Finish(probe, healthGateTestResolver(cfg), HealthGateOutcomeSuccess, HealthGateErrorInfo{})
	if len(transitions) != 2 {
		t.Fatalf("reset/config cycle/stale probe emitted transition: %+v", transitions)
	}
}
