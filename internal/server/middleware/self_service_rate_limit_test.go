package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
)

func TestSelfServiceAuthRateLimitWindowCountsEveryFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := time.Unix(1000, 0)
	limiter := NewSelfServiceRateLimiter(func() time.Time { return clock })
	attempts := 0
	router := gin.New()
	router.Use(limiter.WithSelfServiceAuthRateLimit())
	router.GET("/auth", func(c *gin.Context) {
		attempts++
		blocked, seconds := RecordSelfServiceAuthFailure(c.Request)
		if blocked {
			c.Header("Retry-After", strconv.Itoa(seconds))
			c.Status(http.StatusTooManyRequests)
			return
		}
		c.Status(http.StatusUnauthorized)
	})

	request := func(ip string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/auth", nil)
		r.RemoteAddr = ip + ":8000"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}

	for i := range selfServiceAuthLimit - 1 {
		if w := request("192.0.2.1"); w.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d: got %d, want 401", i+1, w.Code)
		}
	}
	clock = clock.Add(250 * time.Millisecond)
	if w := request("192.0.2.1"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("20th failure: status %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	if w := request("192.0.2.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("later failure: got %d", w.Code)
	}
	if attempts != selfServiceAuthLimit+1 {
		t.Fatalf("auth handler invoked %d times", attempts)
	}
	if w := request("192.0.2.2"); w.Code != http.StatusUnauthorized {
		t.Fatalf("independent IP: got %d", w.Code)
	}

	clock = clock.Add(59 * time.Second)
	if w := request("192.0.2.1"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("before expiry: status %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	clock = clock.Add(750 * time.Millisecond)
	if w := request("192.0.2.1"); w.Code != http.StatusUnauthorized {
		t.Fatalf("at expiry: got %d, want 401", w.Code)
	}
	if attempts != selfServiceAuthLimit+4 {
		t.Fatalf("expected every failure to reach auth handler, got %d invocations", attempts)
	}
}

func TestSelfServiceQueryRateLimitByAuthenticatedKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := time.Unix(2000, 0)
	limiter := NewSelfServiceRateLimiter(func() time.Time { return clock })
	calls := 0
	router := gin.New()
	router.Use(func(c *gin.Context) {
		id, err := strconv.Atoi(c.GetHeader("X-Test-Key-ID"))
		if err == nil {
			c.Request = c.Request.WithContext(contexts.WithAPIKey(c.Request.Context(), &ent.APIKey{ID: id}))
		}
		c.Next()
	}, limiter.WithSelfServiceQueryRateLimit())
	router.POST("/query", func(c *gin.Context) { calls++; c.Status(http.StatusOK) })

	request := func(key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/query", nil)
		r.Header.Set("X-Test-Key-ID", key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}

	if w := request(""); w.Code != http.StatusUnauthorized {
		t.Fatalf("missing authenticated key: got %d", w.Code)
	}
	for i := range selfServiceQueryLimit {
		if w := request("17"); w.Code != http.StatusOK {
			t.Fatalf("query %d: got %d", i+1, w.Code)
		}
	}
	clock = clock.Add(100 * time.Millisecond)
	if w := request("17"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("61st query: status %d, retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	if w := request("18"); w.Code != http.StatusOK {
		t.Fatalf("independent key: got %d", w.Code)
	}
	if calls != selfServiceQueryLimit+1 {
		t.Fatalf("handler invoked %d times", calls)
	}
	clock = clock.Add(59900 * time.Millisecond)
	if w := request("17"); w.Code != http.StatusOK {
		t.Fatalf("after window reset: got %d", w.Code)
	}
}

func TestSelfServiceRateLimiterCapAndLazyExpiry(t *testing.T) {
	clock := time.Unix(3000, 0)
	limiter := NewSelfServiceRateLimiter(func() time.Time { return clock })
	for i := range selfServiceMaxEntries {
		blocked, _ := limiter.increment(fmt.Sprintf("auth:%d", i), selfServiceAuthLimit, true)
		if blocked {
			t.Fatalf("existing capacity available at insertion %d", i)
		}
	}
	for i := range 50 {
		blocked, retry := limiter.increment(fmt.Sprintf("auth:extra-%d", i), selfServiceAuthLimit, true)
		if !blocked || retry != 60 {
			t.Fatalf("new IP at capacity: blocked=%t retry=%d", blocked, retry)
		}
		if len(limiter.authEntries) != selfServiceMaxEntries {
			t.Fatalf("auth map size changed at capacity: %d", len(limiter.authEntries))
		}
	}
	if blocked, _ := limiter.increment("auth:0", selfServiceAuthLimit, true); blocked {
		t.Fatal("existing IP evicted at capacity")
	}
	if blocked, _ := limiter.increment("query:1", selfServiceQueryLimit, false); blocked {
		t.Fatal("full auth failure map blocked a new query key")
	}
	if len(limiter.authEntries) != selfServiceMaxEntries || len(limiter.queryEntries) != 1 {
		t.Fatalf("bucket sizes: auth=%d query=%d", len(limiter.authEntries), len(limiter.queryEntries))
	}

	clock = clock.Add(time.Minute)
	if blocked, _ := limiter.increment("auth:new", selfServiceAuthLimit, true); blocked {
		t.Fatal("new IP rejected after expiry")
	}
	if len(limiter.authEntries) != 1 {
		t.Fatalf("expired auth buckets were not reaped: %d remain", len(limiter.authEntries))
	}
	if blocked, _ := limiter.increment("query:2", selfServiceQueryLimit, false); blocked {
		t.Fatal("new query key rejected after expiry")
	}
	if len(limiter.queryEntries) != 2 {
		t.Fatalf("query buckets not isolated: %d remain", len(limiter.queryEntries))
	}
}

func TestSelfServiceRateLimiterCapacityMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	clock := time.Unix(4000, 0)
	limiter := NewSelfServiceRateLimiter(func() time.Time { return clock })
	limiter.maxEntries = 2
	calls := 0
	router := gin.New()
	router.Use(limiter.WithSelfServiceAuthRateLimit())
	router.GET("/auth", func(c *gin.Context) {
		calls++
		blocked, retry := RecordSelfServiceAuthFailure(c.Request)
		if blocked {
			c.Header("Retry-After", strconv.Itoa(retry))
			c.Status(http.StatusTooManyRequests)
			return
		}
		c.Status(http.StatusUnauthorized)
	})
	request := func(method, path, ip, key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, nil)
		r.RemoteAddr = ip + ":8000"
		r.Header.Set("X-Test-Key-ID", key)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	if w := request(http.MethodGet, "/auth", "192.0.2.1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("first IP: %d", w.Code)
	}
	if w := request(http.MethodGet, "/auth", "192.0.2.2", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("second IP: %d", w.Code)
	}
	if w := request(http.MethodGet, "/auth", "192.0.2.3", ""); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("new IP at capacity: %d retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	if calls != 3 {
		t.Fatalf("new IP did not reach auth handler: %d calls", calls)
	}
	if w := request(http.MethodGet, "/auth", "192.0.2.1", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("known IP evicted: %d", w.Code)
	}
	queryLimiter := NewSelfServiceRateLimiter(func() time.Time { return clock })
	queryLimiter.maxEntries = 2
	queryRouter := gin.New()
	queryRouter.Use(func(c *gin.Context) {
		id, _ := strconv.Atoi(c.GetHeader("X-Test-Key-ID"))
		c.Request = c.Request.WithContext(contexts.WithAPIKey(c.Request.Context(), &ent.APIKey{ID: id}))
		c.Next()
	}, queryLimiter.WithSelfServiceQueryRateLimit())
	queryRouter.POST("/query", func(c *gin.Context) { c.Status(http.StatusOK) })
	queryRequest := func(key string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/query", nil)
		r.Header.Set("X-Test-Key-ID", key)
		w := httptest.NewRecorder()
		queryRouter.ServeHTTP(w, r)
		return w
	}
	if w := queryRequest("19"); w.Code != http.StatusOK {
		t.Fatalf("first query key: %d", w.Code)
	}
	if w := queryRequest("20"); w.Code != http.StatusOK {
		t.Fatalf("second query key: %d", w.Code)
	}
	if w := queryRequest("21"); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") != "60" {
		t.Fatalf("new query key at capacity: %d retry-after %q", w.Code, w.Header().Get("Retry-After"))
	}
	if w := queryRequest("19"); w.Code != http.StatusOK {
		t.Fatalf("known query key evicted: %d", w.Code)
	}
	clock = clock.Add(time.Minute)
	if w := queryRequest("21"); w.Code != http.StatusOK {
		t.Fatalf("query key after expiry: %d", w.Code)
	}
}
