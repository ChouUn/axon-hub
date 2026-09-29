package biz

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func healthGateBegin(t *testing.T, gate *HealthGate, key HealthGateKey, resolve HealthGateConfigResolver, probe, lastResort bool) (HealthGateTicket, bool) {
	t.Helper()
	ticket, ok, err := gate.Begin(t.Context(), key, resolve, probe, lastResort)
	require.NoError(t, err)
	return ticket, ok
}

func healthGateInspect(t *testing.T, gate *HealthGate, key HealthGateKey, resolve HealthGateConfigResolver) HealthGateView {
	t.Helper()
	view, err := gate.Inspect(t.Context(), key, resolve)
	require.NoError(t, err)
	return view
}

func healthGateFinish(t *testing.T, gate *HealthGate, ticket HealthGateTicket, resolve HealthGateConfigResolver, outcome HealthGateOutcome, info HealthGateErrorInfo) {
	t.Helper()
	require.NoError(t, gate.Finish(t.Context(), ticket, resolve, outcome, info))
}

func healthGateSnapshot(t *testing.T, gate *HealthGate, channelID int, resolve HealthGateConfigResolver) []HealthGateModelSnapshot {
	t.Helper()
	snapshot, err := gate.Snapshot(t.Context(), channelID, resolve)
	require.NoError(t, err)
	return snapshot
}
