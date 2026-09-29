package orchestrator

import (
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/stretchr/testify/require"
	"testing"
)

func healthGateBegin(t *testing.T, gate *biz.HealthGate, key biz.HealthGateKey, resolve biz.HealthGateConfigResolver, probe, lastResort bool) (biz.HealthGateTicket, bool) {
	t.Helper()
	ticket, ok, err := gate.Begin(t.Context(), key, resolve, probe, lastResort)
	require.NoError(t, err)
	return ticket, ok
}

func healthGateInspect(t *testing.T, gate *biz.HealthGate, key biz.HealthGateKey, resolve biz.HealthGateConfigResolver) biz.HealthGateView {
	t.Helper()
	view, err := gate.Inspect(t.Context(), key, resolve)
	require.NoError(t, err)
	return view
}

func healthGateFinish(t *testing.T, gate *biz.HealthGate, ticket biz.HealthGateTicket, resolve biz.HealthGateConfigResolver, outcome biz.HealthGateOutcome, info biz.HealthGateErrorInfo) {
	t.Helper()
	require.NoError(t, gate.Finish(t.Context(), ticket, resolve, outcome, info))
}

func healthGateSnapshot(t *testing.T, gate *biz.HealthGate, channelID int, resolve biz.HealthGateConfigResolver) []biz.HealthGateModelSnapshot {
	t.Helper()
	snapshot, err := gate.Snapshot(t.Context(), channelID, resolve)
	require.NoError(t, err)
	return snapshot
}
