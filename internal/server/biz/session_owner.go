package biz

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"github.com/eko/gocache/lib/v4/store"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/log"
	"github.com/looplj/axonhub/internal/objects"
)

const sessionOwnerTTL = 30 * time.Minute

// ownerUpdateLockShards serializes updates to one authoritative thread (or trace)
// within this process. A fixed shard count bounds memory regardless of how many
// sessions have existed; unrelated sessions may occasionally share a shard.
// Shared-cache deployments require a distributed compare-and-swap for
// cross-instance serialization.
var ownerUpdateLockShards [256]sync.Mutex

func ownerUpdateLock(key string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &ownerUpdateLockShards[h.Sum32()%uint32(len(ownerUpdateLockShards))]
}

// SessionOwner is the channel bound to a thread (or a standalone trace), plus
// the number of consecutive successful requests completed by other channels.
type SessionOwner struct {
	ChannelID            int `json:"channel_id"`
	ConsecutiveFailovers int `json:"consecutive_failovers"`
}

func buildSessionThreadOwnerCacheKey(threadID int) string {
	return fmt.Sprintf("axonhub:routing:session-owner:v1:thread:%d", threadID)
}

func buildSessionTraceOwnerCacheKey(traceID int) string {
	return fmt.Sprintf("axonhub:routing:session-owner:v1:trace:%d", traceID)
}

// GetSessionOwner reads the thread alone when available. A missing v1 entry
// inherits the previous-channel cache on that same key; trace never overrides
// a thread owner, even when the thread key has expired.
func (s *RequestService) GetSessionOwner(ctx context.Context, threadID, traceID int) (SessionOwner, bool, error) {
	var ownerKey, previousKey string
	switch {
	case threadID > 0:
		ownerKey = buildSessionThreadOwnerCacheKey(threadID)
		previousKey = buildPreviousThreadChannelCacheKey(threadID)
	case traceID > 0:
		ownerKey = buildSessionTraceOwnerCacheKey(traceID)
		previousKey = buildPreviousTraceChannelCacheKey(traceID)
	default:
		return SessionOwner{}, false, nil
	}

	owner, err := s.sessionOwnerCache.Get(ctx, ownerKey)
	if err == nil {
		if owner.ChannelID > 0 {
			return owner, true, nil
		}
		return SessionOwner{}, false, nil
	}
	var missing *store.NotFound
	if !errors.As(err, &missing) {
		return SessionOwner{}, false, fmt.Errorf("read session owner %q: %w", ownerKey, err)
	}

	channelID, err := s.previousChannelCache.Get(ctx, previousKey)
	if err == nil {
		if channelID > 0 {
			return SessionOwner{ChannelID: channelID}, true, nil
		}
		return SessionOwner{}, false, nil
	}
	if !errors.As(err, &missing) {
		return SessionOwner{}, false, fmt.Errorf("read previous session channel %q: %w", previousKey, err)
	}
	return SessionOwner{}, false, nil
}

// UpdateSessionOwner serializes read/modify/write for an authoritative session
// key. A thread never falls back to a trace owner, including on cache expiry.
// The callback runs under the lock and must not recursively update this key.
// Serialization is process-local when the cache is shared across instances.
func (s *RequestService) UpdateSessionOwner(ctx context.Context, threadID, traceID int, update func(SessionOwner, bool) (SessionOwner, bool)) {
	if update == nil {
		return
	}
	var key string
	if threadID > 0 {
		key = buildSessionThreadOwnerCacheKey(threadID)
	} else if traceID > 0 {
		key = buildSessionTraceOwnerCacheKey(traceID)
	} else {
		return
	}
	mu := ownerUpdateLock(key)
	mu.Lock()
	defer mu.Unlock()

	current, found, err := s.GetSessionOwner(ctx, threadID, traceID)
	if err != nil {
		log.Warn(ctx, "failed to read session owner for update", log.Cause(err))
		return
	}
	next, write := update(current, found)
	if !write || next.ChannelID <= 0 {
		return
	}
	if threadID > 0 {
		if err := s.sessionOwnerCache.Set(ctx, buildSessionThreadOwnerCacheKey(threadID), next, store.WithExpiration(sessionOwnerTTL)); err != nil {
			log.Warn(ctx, "failed to cache thread session owner", log.Cause(err), log.Int("thread_id", threadID))
		}
		s.setPreviousThreadChannelID(ctx, threadID, next.ChannelID)
	}
	if traceID > 0 {
		if err := s.sessionOwnerCache.Set(ctx, buildSessionTraceOwnerCacheKey(traceID), next, store.WithExpiration(sessionOwnerTTL)); err != nil {
			log.Warn(ctx, "failed to cache trace session owner", log.Cause(err), log.Int("trace_id", traceID))
		}
		s.setPreviousTraceChannelID(ctx, traceID, next.ChannelID)
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
