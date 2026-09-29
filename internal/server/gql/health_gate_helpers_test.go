package gql

import (
	"testing"

	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/stretchr/testify/require"
)

func healthGateBegin(t *testing.T, gate *biz.HealthGate, key biz.HealthGateKey, resolve biz.HealthGateConfigResolver, probe, lastResort bool) (biz.HealthGateTicket, bool) {
	t.Helper()
	ticket, ok, err := gate.Begin(t.Context(), key, resolve, probe, lastResort)
	require.NoError(t, err)
	return ticket, ok
}


func healthGateFinish(t *testing.T, gate *biz.HealthGate, ticket biz.HealthGateTicket, resolve biz.HealthGateConfigResolver, outcome biz.HealthGateOutcome, info biz.HealthGateErrorInfo) {
	t.Helper()
	require.NoError(t, gate.Finish(t.Context(), ticket, resolve, outcome, info))
}

