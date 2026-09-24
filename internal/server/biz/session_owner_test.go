package biz

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

func testSessionOwnerService(t *testing.T) (*RequestService, *gocache.Cache, *gocache.Cache) {
	t.Helper()
	owners := gocache.New(0, 0)
	previous := gocache.New(0, 0)
	return &RequestService{
		sessionOwnerCache:    xcache.NewMemory[SessionOwner](owners),
		previousChannelCache: xcache.NewMemory[int](previous),
	}, owners, previous
}

func setTestSessionOwner(svc *RequestService, ctx context.Context, threadID, traceID int, owner SessionOwner) {
	svc.UpdateSessionOwner(ctx, threadID, traceID, func(SessionOwner, bool) (SessionOwner, bool) {
		return owner, true
	})
}

func TestSessionOwnerThreadIsAuthoritative(t *testing.T) {
	svc, _, _ := testSessionOwnerService(t)
	ctx := t.Context()
	require.NoError(t, svc.sessionOwnerCache.Set(ctx, buildSessionTraceOwnerCacheKey(17), SessionOwner{ChannelID: 9}))
	require.NoError(t, svc.previousChannelCache.Set(ctx, buildPreviousTraceChannelCacheKey(17), 10))
	owner, found, err := svc.GetSessionOwner(ctx, 23, 17)
	require.NoError(t, err)
	require.False(t, found)
	require.Zero(t, owner)

	require.NoError(t, svc.sessionOwnerCache.Set(ctx, buildSessionThreadOwnerCacheKey(23), SessionOwner{ChannelID: 4, ConsecutiveFailovers: 1}))
	owner, found, err = svc.GetSessionOwner(ctx, 23, 17)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, SessionOwner{ChannelID: 4, ConsecutiveFailovers: 1}, owner)
}

func TestSessionOwnerSeedsFromSameLegacyKey(t *testing.T) {
	svc, _, _ := testSessionOwnerService(t)
	ctx := t.Context()
	require.NoError(t, svc.previousChannelCache.Set(ctx, buildPreviousTraceChannelCacheKey(7), 99))
	require.NoError(t, svc.previousChannelCache.Set(ctx, buildPreviousThreadChannelCacheKey(5), 12))
	owner, found, err := svc.GetSessionOwner(ctx, 5, 7)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, SessionOwner{ChannelID: 12}, owner)

	owner, found, err = svc.GetSessionOwner(ctx, 0, 7)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, SessionOwner{ChannelID: 99}, owner)

	owner, found, err = svc.GetSessionOwner(ctx, 0, 0)
	require.NoError(t, err)
	require.False(t, found)
	require.Zero(t, owner)
}

func TestSessionOwnerCommitRefreshesBothKeyVersions(t *testing.T) {
	svc, owners, previous := testSessionOwnerService(t)
	ctx := t.Context()
	owner := SessionOwner{ChannelID: 42, ConsecutiveFailovers: 1}
	setTestSessionOwner(svc, ctx, 5, 7, owner)
	for _, key := range []string{buildSessionThreadOwnerCacheKey(5), buildSessionTraceOwnerCacheKey(7)} {
		cached, err := svc.sessionOwnerCache.Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, owner, cached)
		_, expireAt, ok := owners.GetWithExpiration(key)
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(sessionOwnerTTL), expireAt, 5*time.Second)
	}
	for _, key := range []string{buildPreviousThreadChannelCacheKey(5), buildPreviousTraceChannelCacheKey(7)} {
		cached, err := svc.previousChannelCache.Get(ctx, key)
		require.NoError(t, err)
		require.Equal(t, owner.ChannelID, cached)
		_, expireAt, ok := previous.GetWithExpiration(key)
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(sessionOwnerTTL), expireAt, 5*time.Second)
	}
}

type sessionOwnerReadErrorCache struct {
	xcache.Cache[SessionOwner]
	err error
}

func (c sessionOwnerReadErrorCache) Get(context.Context, any) (SessionOwner, error) {
	return SessionOwner{}, c.err
}

type legacyChannelReadErrorCache struct {
	xcache.Cache[int]
	err error
}

func (c legacyChannelReadErrorCache) Get(context.Context, any) (int, error) {
	return 0, c.err
}

func TestSessionOwnerReadErrorDoesNotFallThroughToTrace(t *testing.T) {
	svc, _, _ := testSessionOwnerService(t)
	ctx := t.Context()
	require.NoError(t, svc.previousChannelCache.Set(ctx, buildPreviousTraceChannelCacheKey(7), 99))
	failure := errors.New("cache unavailable")
	svc.sessionOwnerCache = sessionOwnerReadErrorCache{Cache: svc.sessionOwnerCache, err: failure}
	owner, found, err := svc.GetSessionOwner(ctx, 5, 7)
	require.ErrorIs(t, err, failure)
	require.False(t, found)
	require.Zero(t, owner)

	svc.sessionOwnerCache = xcache.NewNoop[SessionOwner]()
	svc.previousChannelCache = legacyChannelReadErrorCache{Cache: svc.previousChannelCache, err: failure}
	owner, found, err = svc.GetSessionOwner(ctx, 5, 7)
	require.ErrorIs(t, err, failure)
	require.False(t, found)
	require.Zero(t, owner)
}

func TestSessionOwnerUpdateDoesNotWriteOnReadError(t *testing.T) {
	svc, _, _ := testSessionOwnerService(t)
	failure := errors.New("cache down")
	svc.sessionOwnerCache = sessionOwnerReadErrorCache{Cache: svc.sessionOwnerCache, err: failure}
	called := false
	svc.UpdateSessionOwner(t.Context(), 5, 7, func(SessionOwner, bool) (SessionOwner, bool) {
		called = true
		return SessionOwner{ChannelID: 3}, true
	})
	require.False(t, called)
}

func TestSessionOwnerUpdateSerializesConcurrentReadModifyWrite(t *testing.T) {
	svc, _, _ := testSessionOwnerService(t)
	ctx := t.Context()
	setTestSessionOwner(svc, ctx, 5, 7, SessionOwner{ChannelID: 1})
	entered := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Errorf("owner update panic: %v", recovered)
			}
		}()
		svc.UpdateSessionOwner(ctx, 5, 7, func(current SessionOwner, found bool) (SessionOwner, bool) {
			close(entered)
			<-release
			return SessionOwner{ChannelID: current.ChannelID, ConsecutiveFailovers: current.ConsecutiveFailovers + 1}, found
		})
	}()
	<-entered
	go func() {
		defer wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Errorf("owner update panic: %v", recovered)
			}
		}()
		svc.UpdateSessionOwner(ctx, 5, 7, func(current SessionOwner, found bool) (SessionOwner, bool) {
			return SessionOwner{ChannelID: current.ChannelID, ConsecutiveFailovers: current.ConsecutiveFailovers + 1}, found
		})
	}()
	close(release)
	wg.Wait()
	owner, found, err := svc.GetSessionOwner(ctx, 5, 7)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, SessionOwner{ChannelID: 1, ConsecutiveFailovers: 2}, owner)
}

func TestRoutingDecisionIsPersisted(t *testing.T) {
	client := enttest.NewEntClient(t, "sqlite3", "file:session-routing-decision?mode=memory&_fk=0")
	defer client.Close()
	ctx := authz.WithTestBypass(ent.NewContext(t.Context(), client))
	svc := NewRequestService(client, xcache.Config{Mode: xcache.ModeMemory}, nil, nil, nil, nil)
	req, err := client.Request.Create().SetTraceID(7).SetModelID("test-model").SetRequestBody(objects.JSONRawMessage(`{}`)).SetStatus("completed").Save(ctx)
	require.NoError(t, err)
	decision := &objects.RequestRoutingDecision{
		Owner:                &objects.RoutingDecisionCombo{ChannelID: 4, ChannelName: "primary", ActualModel: "test-model"},
		Skipped:              []objects.RoutingDecisionCombo{{ChannelID: 9, ChannelName: "broken", ActualModel: "test-model", State: "open"}},
		TemporaryFailover:    true,
		ConsecutiveFailovers: 1,
	}
	require.NoError(t, svc.SetRequestRoutingDecision(ctx, req.ID, nil))
	require.NoError(t, svc.SetRequestRoutingDecision(ctx, req.ID, decision))
	stored, err := client.Request.Get(ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, decision, stored.RoutingDecision)
	require.NoError(t, svc.UpdateRequestChannelIDWithoutSticky(ctx, req.ID, 42))
	stored, err = client.Request.Get(ctx, req.ID)
	require.NoError(t, err)
	require.Equal(t, 42, stored.ChannelID)
	_, err = svc.previousChannelCache.Get(ctx, buildPreviousTraceChannelCacheKey(7))
	require.Error(t, err)
}
