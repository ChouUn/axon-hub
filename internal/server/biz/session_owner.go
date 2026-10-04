package biz

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/eko/gocache/lib/v4/store"
	"golang.org/x/sync/semaphore"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/objects"
	"github.com/looplj/axonhub/internal/pkg/xcache"
)

const (
	sessionOwnerTTL = 5 * time.Minute
	// Keep expired identity for a bounded grace period for late completions.
	// Cache retention does not extend the logical lease.
	sessionOwnerExpiryRetention = 30 * time.Minute
)

// Mutable ownership must not use the chain's asynchronous L2-to-L1 promotion:
// a delayed read promotion can overwrite a newer successful migration in L1.
func newSessionOwnerCache(cfg xcache.Config) xcache.Cache[SessionOwner] {
	if cfg.Mode == xcache.ModeTwoLevel {
		cfg.Mode = xcache.ModeMemory
		if cfg.Redis.Addr != "" || len(cfg.Redis.Addrs) != 0 || cfg.Redis.URL != "" {
			cfg.Mode = xcache.ModeRedis
		}
	}
	return xcache.NewFromConfig[SessionOwner](cfg)
}

// ownerUpdateLockShards serializes updates to one authoritative thread (or trace)
// within this process. A fixed shard count bounds memory regardless of how many
// sessions have existed; unrelated sessions may occasionally share a shard.
// Shared-cache deployments require a distributed compare-and-swap for
// cross-instance serialization.
var ownerUpdateLockShards = func() [256]*semaphore.Weighted {
	var locks [256]*semaphore.Weighted
	for i := range locks {
		locks[i] = semaphore.NewWeighted(1)
	}
	return locks
}()

func ownerUpdateLock(key string) *semaphore.Weighted {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return ownerUpdateLockShards[h.Sum32()%uint32(len(ownerUpdateLockShards))]
}

// SessionOwner is the channel bound to a thread (or a standalone trace), plus
// the number of consecutive successful requests completed by other channels.
type SessionOwner struct {
	ChannelID            int `json:"channel_id"`
	ConsecutiveFailovers int `json:"consecutive_failovers"`
	// ExpiresAt is the logical deadline, independent of cache eviction.
	// Missing deadlines are rejected; old v1/legacy entries are not imported.
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	// Generation identifies a new binding; renewing a live binding preserves it.
	Generation time.Time `json:"generation,omitempty"`
}

func buildSessionThreadOwnerCacheKey(threadID int) string {
	return fmt.Sprintf("axonhub:routing:session-owner:v2:thread:%d", threadID)
}

func buildSessionTraceOwnerCacheKey(traceID int) string {
	return fmt.Sprintf("axonhub:routing:session-owner:v2:trace:%d", traceID)
}

// GetSessionOwner reads only the authoritative thread when available. The v2
// boundary deliberately reevaluates old bindings once on upgrade: no v1 or
// legacy value without a provable deadline can revive an expired owner.
func (s *RequestService) GetSessionOwner(ctx context.Context, threadID, traceID int) (SessionOwner, bool, error) {
	ownerKey := sessionOwnerKey(threadID, traceID)
	if ownerKey == "" {
		return SessionOwner{}, false, nil
	}
	mu := ownerUpdateLock(ownerKey)
	if err := mu.Acquire(ctx, 1); err != nil {
		return SessionOwner{}, false, err
	}
	defer mu.Release(1)
	owner, found, err := s.readSessionOwner(ctx, ownerKey)
	if !found {
		return SessionOwner{}, false, err
	}
	return owner, true, err
}

func sessionOwnerKey(threadID, traceID int) string {
	if threadID > 0 {
		return buildSessionThreadOwnerCacheKey(threadID)
	}
	if traceID > 0 {
		return buildSessionTraceOwnerCacheKey(traceID)
	}
	return ""
}

// readSessionOwner must run under the authoritative key's lock. It preserves
// expired identity for completion's late-migration guard, but reports found=false.
func (s *RequestService) readSessionOwner(ctx context.Context, ownerKey string) (SessionOwner, bool, error) {
	owner, err := s.sessionOwnerCache.Get(ctx, ownerKey)
	var missing *store.NotFound
	if err != nil {
		if !errors.As(err, &missing) {
			return SessionOwner{}, false, fmt.Errorf("read session owner %q: %w", ownerKey, err)
		}
		return SessionOwner{}, false, nil
	}
	if owner.ChannelID <= 0 || owner.ExpiresAt.IsZero() {
		return SessionOwner{}, false, nil
	}
	return owner, time.Now().Before(owner.ExpiresAt), nil
}

func (s *RequestService) cacheSessionOwner(ctx context.Context, key string, owner SessionOwner) error {
	retention := max(time.Until(owner.ExpiresAt), 0) + sessionOwnerExpiryRetention
	return s.sessionOwnerCache.Set(ctx, key, owner, store.WithExpiration(retention))
}

// UpdateSessionOwner serializes read/modify/write for an authoritative session
// key. A thread never falls back to a trace owner, including on cache expiry.
// The callback runs under the lock and must not recursively update this key.
// Serialization is process-local when the cache is shared across instances.
func (s *RequestService) UpdateSessionOwner(ctx context.Context, threadID, traceID int, update func(SessionOwner, bool) (SessionOwner, bool)) {
	if update == nil {
		return
	}
	key := sessionOwnerKey(threadID, traceID)
	if key == "" {
		return
	}
	mu := ownerUpdateLock(key)
	if err := mu.Acquire(ctx, 1); err != nil {
		log.Warn(ctx, "session owner update canceled while waiting", log.Cause(err))
		return
	}
	defer mu.Release(1)

	current, found, err := s.readSessionOwner(ctx, key)
	if err != nil {
		log.Warn(ctx, "failed to read session owner for update", log.Cause(err))
		return
	}
	next, write := update(current, found)
	if !write || next.ChannelID <= 0 || ctx.Err() != nil {
		return
	}
	if next.ExpiresAt.IsZero() {
		next.ExpiresAt = time.Now().Add(sessionOwnerTTL)
	}
	refreshLegacy := next.ChannelID != current.ChannelID || !next.ExpiresAt.Equal(current.ExpiresAt)
	if threadID > 0 {
		if err := s.cacheSessionOwner(ctx, buildSessionThreadOwnerCacheKey(threadID), next); err != nil {
			log.Warn(ctx, "failed to cache thread session owner", log.Cause(err), log.Int("thread_id", threadID))
		}
		if refreshLegacy {
			s.setPreviousThreadChannelID(ctx, threadID, next.ChannelID)
		}
	}
	if traceID > 0 {
		if err := s.cacheSessionOwner(ctx, buildSessionTraceOwnerCacheKey(traceID), next); err != nil {
			log.Warn(ctx, "failed to cache trace session owner", log.Cause(err), log.Int("trace_id", traceID))
		}
		if refreshLegacy {
			s.setPreviousTraceChannelID(ctx, traceID, next.ChannelID)
		}
	}
}

// SessionOwnerChannelName resolves a channel omitted from this request's
// candidates, including disabled channels, without constructing an outbound.
func (s *RequestService) SessionOwnerChannelName(ctx context.Context, channelID int) (string, error) {
	bypass := authz.WithSystemBypass(ctx, "session-owner-channel-name")
	channel, err := s.entFromContext(bypass).Channel.Get(bypass, channelID)
	if err != nil {
		return "", err
	}
	return channel.Name, nil
}

// UpdateRequestChannelIDWithoutSticky tracks the selected channel without
// changing the current session owner before a successful request completes.
func (s *RequestService) UpdateRequestChannelIDWithoutSticky(ctx context.Context, requestID, channelID int) error {
	if err := s.entFromContext(ctx).Request.UpdateOneID(requestID).SetChannelID(channelID).Exec(ctx); err != nil {
		return fmt.Errorf("failed to update request channel ID: %w", err)
	}
	return nil
}

// SetRequestRoutingDecision persists the completed health-gated routing trace.
func (s *RequestService) SetRequestRoutingDecision(ctx context.Context, requestID int, d *objects.RequestRoutingDecision) error {
	if d == nil {
		return nil
	}
	bypassCtx := authz.WithSystemBypass(ctx, "request-routing-decision")
	if err := s.entFromContext(bypassCtx).Request.UpdateOneID(requestID).SetRoutingDecision(d).Exec(bypassCtx); err != nil {
		return fmt.Errorf("failed to set request routing decision: %w", err)
	}
	return nil
}
