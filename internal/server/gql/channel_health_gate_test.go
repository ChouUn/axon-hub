package gql

import (
	"context"
	"testing"
	"time"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/channel"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/stretchr/testify/require"
)

func healthGateResolver(cfg biz.HealthGateConfig) biz.HealthGateConfigResolver {
	return func() (biz.HealthGateConfig, bool) { return cfg, true }
}

func TestChannelHealthGateStatusCountsAndDisabled(t *testing.T) {
	now := time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC)
	gate := biz.NewHealthGate(func() time.Time { return now })
	policy := biz.DefaultHealthGatePolicy()
	policy.FailureThreshold = 1
	policy.OpenDurationSeconds = 10
	ch := &ent.Channel{ID: 42}
	cfg := biz.ResolveHealthGateConfig(policy, &biz.Channel{Channel: ch})

	for _, name := range []string{"open", "probing"} {
		key := biz.HealthGateKey{ChannelID: ch.ID, ActualModel: name}
		ticket, ok := gate.Begin(key, healthGateResolver(cfg), false, false)
		if !ok {
			t.Fatalf("initial attempt rejected for %s", name)
		}
		gate.Finish(ticket, healthGateResolver(cfg), biz.HealthGateOutcomeFailure, biz.HealthGateErrorInfo{Message: "upstream error", StatusCode: 503})
	}
	now = now.Add(11 * time.Second)
	key := biz.HealthGateKey{ChannelID: ch.ID, ActualModel: "open"}
	probe, ok := gate.Begin(key, healthGateResolver(cfg), true, false)
	if !ok {
		t.Fatal("failed to start probe")
	}
	gate.Finish(probe, healthGateResolver(cfg), biz.HealthGateOutcomeFailure, biz.HealthGateErrorInfo{})

	status := channelHealthGateStatus(gate, policy, ch, func() (biz.HealthGateConfig, bool) {
		return biz.ResolveHealthGateConfig(policy, &biz.Channel{Channel: ch}), true
	})
	if status.OpenCount != 2 || status.UnstableCount != 0 || len(status.Models) != 2 || status.Models[0].State != "open" || status.Models[1].State != "probing" {
		t.Fatalf("open/probing status mismatch: %+v", status)
	}
	if status.Models[0].LastError != nil || status.Models[0].LastStatusCode != nil {
		t.Fatalf("empty probe error must be nullable: %+v", status.Models[0])
	}
	if status.Models[1].LastError == nil || *status.Models[1].LastError != "upstream error" || status.Models[1].LastStatusCode == nil || *status.Models[1].LastStatusCode != 503 {
		t.Fatalf("error details lost: %+v", status.Models[1])
	}

	zero := 0
	ch.Settings = &objects.ChannelSettings{HealthGateFailureThreshold: &zero}
	status = channelHealthGateStatus(gate, policy, ch, func() (biz.HealthGateConfig, bool) {
		return biz.ResolveHealthGateConfig(policy, &biz.Channel{Channel: ch}), true
	})
	if !status.Disabled || status.FailureThreshold != 0 || status.OpenCount != 0 || status.UnstableCount != 0 || status.Models == nil || len(status.Models) != 0 {
		t.Fatalf("disabled channel must have empty health models: %+v", status)
	}
}

func TestChannelHealthGateReadScopeAndSystemPolicy(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:health-gate-policy?mode=memory&_fk=1")
	defer client.Close()

	writeCtx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	defaults := biz.DefaultHealthGatePolicy()
	policy := defaults
	policy.FailureThreshold = 0
	writer := biz.NewSystemService(biz.SystemServiceParams{
		Ent:         client,
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
	})
	require.NoError(t, writer.SetRetryPolicy(writeCtx, &biz.RetryPolicy{HealthGate: &policy}))

	// A separate service guarantees the scoped read misses its cache and reaches Ent privacy.
	reader := biz.NewSystemService(biz.SystemServiceParams{
		Ent:         client,
		CacheConfig: xcache.Config{Mode: xcache.ModeMemory},
	})
	resolver := &channelResolver{&Resolver{client: client, systemService: reader, channelService: &biz.ChannelService{}}}
	ch, err := client.Channel.Create().SetType(channel.TypeOpenai).SetName("Health gate read").SetCredentials(objects.ChannelCredentials{APIKey: "key"}).SetSupportedModels([]string{"model"}).SetDefaultTestModel("model").SetStatus(channel.StatusEnabled).Save(writeCtx)
	require.NoError(t, err)
	unauthorized := authz.NewUserContext(ent.NewContext(context.Background(), client), 1)
	unauthorized = contexts.WithUser(unauthorized, &ent.User{ID: 1, Scopes: []string{"write_requests"}})
	status, err := resolver.HealthGate(unauthorized, ch)
	require.NoError(t, err)
	require.Nil(t, status)

	readChannels := authz.NewUserContext(ent.NewContext(context.Background(), client), 2)
	readChannels = contexts.WithUser(readChannels, &ent.User{ID: 2, Scopes: []string{"read_channels"}})
	status, err = resolver.HealthGate(readChannels, ch)
	require.NoError(t, err)
	require.NotNil(t, status)
	require.True(t, status.Disabled)
	require.Zero(t, status.FailureThreshold)
	require.Empty(t, status.Models)
}
