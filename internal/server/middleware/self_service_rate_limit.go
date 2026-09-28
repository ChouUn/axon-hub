package middleware

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/looplj/axonhub/internal/contexts"
)

const (
	selfServiceWindow     = time.Minute
	selfServiceAuthLimit  = 20
	selfServiceQueryLimit = 60
	selfServiceMaxEntries = 10000
)

type selfServiceBucket struct {
	count   int
	expires time.Time
}

// SelfServiceRateLimiter holds independent in-process auth-failure and query
// counters. One instance must be shared by requests sharing these limits.
type SelfServiceRateLimiter struct {
	mu           sync.Mutex
	authEntries  map[string]selfServiceBucket
	queryEntries map[string]selfServiceBucket
	now          func() time.Time
	maxEntries   int
}

// NewSelfServiceRateLimiter creates an isolated limiter. A nil clock uses time.Now.
func NewSelfServiceRateLimiter(clock func() time.Time) *SelfServiceRateLimiter {
	if clock == nil {
		clock = time.Now
	}

	return &SelfServiceRateLimiter{
		authEntries:  make(map[string]selfServiceBucket),
		queryEntries: make(map[string]selfServiceBucket),
		now:          clock,
		maxEntries:   selfServiceMaxEntries,
	}
}

var sharedSelfServiceRateLimiter = NewSelfServiceRateLimiter(nil)

type selfServiceLimiterContextKey struct{}
type selfServiceIPContextKey struct{}

// WithSelfServiceAuthRateLimit supplies the limiter and client IP to the auth
// handler. Only failed authentication attempts consume this limit; successful
// keys are never blocked by an IP's failed attempts.
func WithSelfServiceAuthRateLimit() gin.HandlerFunc {
	return sharedSelfServiceRateLimiter.WithSelfServiceAuthRateLimit()
}

// WithSelfServiceQueryRateLimit permits 60 authenticated queries per key in a
// fixed minute window. Install it after authentication has populated the key.
func WithSelfServiceQueryRateLimit() gin.HandlerFunc {
	return sharedSelfServiceRateLimiter.WithSelfServiceQueryRateLimit()
}

// RecordSelfServiceAuthFailure counts an invalid credential attempt. It returns
// blocked on the twentieth failure and for any later failures in that window.
// When installed, the auth rate middleware provides its limiter and c.ClientIP();
// otherwise the process-wide limiter and the request remote address are used.
func RecordSelfServiceAuthFailure(r *http.Request) (blocked bool, retryAfterSeconds int) {
	limiter, _ := r.Context().Value(selfServiceLimiterContextKey{}).(*SelfServiceRateLimiter)
	if limiter == nil {
		limiter = sharedSelfServiceRateLimiter
	}

	ip, _ := r.Context().Value(selfServiceIPContextKey{}).(string)
	if ip == "" {
		ip = selfServiceRemoteIP(r.RemoteAddr)
	}

	return limiter.increment("auth:"+ip, selfServiceAuthLimit, true)
}

// WithSelfServiceAuthRateLimit supplies this limiter to authentication without
// rejecting requests before their credentials are checked.
func (l *SelfServiceRateLimiter) WithSelfServiceAuthRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if ip == "" {
			ip = selfServiceRemoteIP(c.Request.RemoteAddr)
		}

		ctx := context.WithValue(c.Request.Context(), selfServiceLimiterContextKey{}, l)
		ctx = context.WithValue(ctx, selfServiceIPContextKey{}, ip)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// WithSelfServiceQueryRateLimit returns a post-authentication query guard using
// this limiter. The authenticated API key ID is used, never a caller supplied key.
func (l *SelfServiceRateLimiter) WithSelfServiceQueryRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		key, ok := contexts.GetAPIKey(c.Request.Context())
		if !ok || key == nil || key.ID <= 0 {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		blocked, retry := l.increment("query:"+strconv.Itoa(key.ID), selfServiceQueryLimit, false)
		if blocked {
			selfServiceTooManyRequests(c, retry)
			return
		}

		c.Next()
	}
}

// increment checks and increments under one lock. Auth rejects the failure
// that reaches its limit; query permits the request that reaches its limit.
func (l *SelfServiceRateLimiter) increment(key string, limit int, rejectAtLimit bool) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	// A full failure-IP table must never consume capacity for authenticated keys.
	entries := l.queryEntries
	if rejectAtLimit {
		entries = l.authEntries
	}
	bucket, found := entries[key]
	if found && !now.Before(bucket.expires) {
		delete(entries, key)
		found = false
	}
	if !found {
		if full, retry := l.capacityReached(entries, now); full {
			return true, retry
		}
		bucket = selfServiceBucket{expires: now.Add(selfServiceWindow)}
	}
	previousCount := bucket.count

	if bucket.count < limit {
		bucket.count++
	}
	entries[key] = bucket

	if bucket.count >= limit && (rejectAtLimit || previousCount >= limit) {
		return true, selfServiceRetryAfter(now, bucket.expires)
	}
	return false, 0
}

// capacityReached runs when a new identity arrives at capacity. It reaps
// expired buckets lazily and rejects the newcomer if all buckets are live.
// The earliest expiry determines when the next slot may become available.
func (l *SelfServiceRateLimiter) capacityReached(entries map[string]selfServiceBucket, now time.Time) (bool, int) {
	if len(entries) < l.maxEntries {
		return false, 0
	}

	var earliest time.Time
	for key, bucket := range entries {
		if !now.Before(bucket.expires) {
			delete(entries, key)
			continue
		}
		if earliest.IsZero() || bucket.expires.Before(earliest) {
			earliest = bucket.expires
		}
	}
	if len(entries) >= l.maxEntries {
		return true, selfServiceRetryAfter(now, earliest)
	}
	return false, 0
}

func selfServiceRetryAfter(now, expires time.Time) int {
	remaining := expires.Sub(now)
	if remaining <= 0 {
		return 1
	}
	return int((remaining + time.Second - 1) / time.Second)
}

func selfServiceTooManyRequests(c *gin.Context, seconds int) {
	c.Header("Retry-After", strconv.Itoa(seconds))
	c.AbortWithStatus(http.StatusTooManyRequests)
}

func selfServiceRemoteIP(remote string) string {
	if host, _, err := net.SplitHostPort(remote); err == nil {
		return host
	}
	return strings.TrimSpace(remote)
}
