package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/looplj/axonhub/internal/authz"
	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/ent/enttest"
	"github.com/looplj/axonhub/internal/ent/project"
	"github.com/looplj/axonhub/internal/ent/user"
	"github.com/looplj/axonhub/internal/pkg/xcache"
	"github.com/looplj/axonhub/internal/server/biz"
	"github.com/looplj/axonhub/llm/transformer/shared"
)

func TestWithSelfServiceAPIKeyAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := enttest.NewEntClient(t, "sqlite3", "file:self_service_auth?mode=memory&_fk=1")
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	seedCtx := authz.WithTestBypass(ent.NewContext(context.Background(), client))
	owner := client.User.Create().
		SetEmail("self-service-auth@example.com").SetPassword("test-password").
		SetFirstName("Self").SetLastName("Service").SetStatus(user.StatusActivated).
		SaveX(seedCtx)
	proj := client.Project.Create().SetName("Self Service Auth").SetStatus(project.StatusActive).SaveX(seedCtx)

	makeKey := func(name string, typ apikey.Type, status apikey.Status, allowedIPs ...string) *ent.APIKey {
		t.Helper()
		value, err := biz.GenerateAPIKey("ah")
		require.NoError(t, err)
		return client.APIKey.Create().SetName(name).SetKey(value).
			SetUserID(owner.ID).SetProjectID(proj.ID).SetType(typ).
			SetStatus(status).SetAllowedIps(allowedIPs).SaveX(seedCtx)
	}
	userKey := makeKey("user", apikey.TypeUser, apikey.StatusEnabled)
	personalKey := makeKey("personal", apikey.TypePersonal, apikey.StatusEnabled)
	serviceKey := makeKey("service", apikey.TypeServiceAccount, apikey.StatusEnabled)
	disabledKey := makeKey("disabled", apikey.TypePersonal, apikey.StatusDisabled)
	archivedKey := makeKey("archived", apikey.TypeUser, apikey.StatusArchived)
	ipKey := makeKey("ip-restricted", apikey.TypePersonal, apikey.StatusEnabled, "203.0.113.10")
	archivedProj := client.Project.Create().SetName("Archived Self Service").SetStatus(project.StatusArchived).SaveX(seedCtx)
	archivedProjectValue, err := biz.GenerateAPIKey("ah")
	require.NoError(t, err)
	archivedProjectKey := client.APIKey.Create().SetName("active-key-archived-project").
		SetKey(archivedProjectValue).SetUserID(owner.ID).SetProjectID(archivedProj.ID).
		SetType(apikey.TypeUser).SetStatus(apikey.StatusEnabled).SaveX(seedCtx)

	cacheCfg := xcache.Config{Mode: xcache.ModeMemory}
	projectSvc := &biz.ProjectService{ProjectCache: xcache.NewFromConfig[xcache.Entry[ent.Project]](cacheCfg)}
	keySvc := biz.NewAPIKeyService(biz.APIKeyServiceParams{
		CacheConfig: cacheCfg, Ent: client, ProjectService: projectSvc, KeyPrefix: "ah",
	})
	t.Cleanup(keySvc.Stop)
	// AllowNoAuth must never bypass authentication on this endpoint.
	authSvc := biz.NewAuthService(biz.AuthServiceParams{
		APIKeyService: keySvc, Ent: client, AllowNoAuth: true,
	})

	engine := gin.New()
	engine.Use(WithEntClient(client), NewSelfServiceRateLimiter(nil).WithSelfServiceAuthRateLimit(), WithSelfServiceAPIKeyAuth(authSvc))
	engine.GET("/self-service", func(c *gin.Context) {
		ctx := c.Request.Context()
		key, ok := contexts.GetAPIKey(ctx)
		require.True(t, ok)
		projectID, ok := contexts.GetProjectID(ctx)
		require.True(t, ok)
		require.Equal(t, proj.ID, projectID)
		principal, ok := authz.GetPrincipal(ctx)
		require.True(t, ok)
		require.True(t, principal.IsAPIKey())
		require.NotNil(t, principal.APIKeyID)
		require.Equal(t, key.ID, *principal.APIKeyID)
		require.NotNil(t, principal.ProjectID)
		require.Equal(t, proj.ID, *principal.ProjectID)
		scope, ok := shared.GetSessionScope(ctx)
		require.True(t, ok)
		require.Equal(t, fmt.Sprintf("api_key:%d:project:%d", key.ID, proj.ID), scope)
		c.Status(http.StatusNoContent)
	})

	cases := []struct {
		name, authorization, extraKey string
		status                        int
	}{
		{name: "missing Authorization", status: http.StatusUnauthorized},
		{name: "bare key", authorization: userKey.Key, status: http.StatusUnauthorized},
		{name: "non Bearer scheme", authorization: "Token " + userKey.Key, status: http.StatusUnauthorized},
		{name: "empty Bearer", authorization: "Bearer ", status: http.StatusUnauthorized},
		{name: "alternative key header", extraKey: userKey.Key, status: http.StatusUnauthorized},
		{name: "forged key", authorization: "Bearer ah_forged", status: http.StatusUnauthorized},
		{name: "noauth sentinel", authorization: "Bearer " + biz.NoAuthAPIKeyValue, status: http.StatusUnauthorized},
		{name: "service account", authorization: "Bearer " + serviceKey.Key, status: http.StatusUnauthorized},
		{name: "disabled", authorization: "Bearer " + disabledKey.Key, status: http.StatusUnauthorized},
		{name: "archived", authorization: "Bearer " + archivedKey.Key, status: http.StatusUnauthorized},
		{name: "archived project", authorization: "Bearer " + archivedProjectKey.Key, status: http.StatusUnauthorized},
		{name: "IP mismatch", authorization: "Bearer " + ipKey.Key, status: http.StatusForbidden},
		{name: "user", authorization: "Bearer " + userKey.Key, status: http.StatusNoContent},
		{name: "personal", authorization: "Bearer " + personalKey.Key, status: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/self-service", nil)
			req.RemoteAddr = "198.51.100.77:12345"
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			if tc.extraKey != "" {
				req.Header.Set("X-API-Key", tc.extraKey)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusUnauthorized {
				require.Contains(t, response.Body.String(), "Invalid API key")
				require.NotContains(t, strings.ToLower(response.Body.String()), "bearer")
			}
		})
	}

	// Exercise the real authentication and query chain with a saturated failure
	// counter. A valid key must not inherit another caller's failed-IP lockout.
	limiter := NewSelfServiceRateLimiter(nil)
	limiter.maxEntries = 2
	limited := gin.New()
	limited.Use(WithEntClient(client), limiter.WithSelfServiceAuthRateLimit(),
		WithSelfServiceAPIKeyAuth(authSvc), limiter.WithSelfServiceQueryRateLimit())
	limited.GET("/self-service", func(c *gin.Context) { c.Status(http.StatusOK) })
	request := func(ip, key, forwardedFor string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/self-service", nil)
		r.RemoteAddr = ip + ":12345"
		r.Header.Set("Authorization", "Bearer "+key)
		if forwardedFor != "" {
			r.Header.Set("X-Forwarded-For", forwardedFor)
		}
		w := httptest.NewRecorder()
		limited.ServeHTTP(w, r)
		return w
	}
	const failedIP = "198.51.100.77"
	for i := 1; i < selfServiceAuthLimit; i++ {
		w := request(failedIP, "ah_forged", "")
		require.Equalf(t, http.StatusUnauthorized, w.Code, "failure %d: %s", i, w.Body.String())
	}
	require.Equal(t, selfServiceAuthLimit-1, limiter.authEntries["auth:"+failedIP].count)
	require.Equal(t, http.StatusOK, request(failedIP, userKey.Key, "").Code)
	require.Equal(t, selfServiceAuthLimit-1, limiter.authEntries["auth:"+failedIP].count)
	w := request(failedIP, "ah_forged", "")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.NotEmpty(t, w.Header().Get("Retry-After"))
	w = request(failedIP, "ah_forged", "")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, http.StatusOK, request(failedIP, userKey.Key, "").Code)
	require.Equal(t, http.StatusOK, request(failedIP, userKey.Key, "203.0.113.222").Code)
	require.Equal(t, http.StatusForbidden, request(failedIP, ipKey.Key, "").Code)
	require.Equal(t, http.StatusUnauthorized, request("192.0.2.7", "ah_forged", "").Code)
	w = request("192.0.2.8", "ah_forged", "")
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.NotEmpty(t, w.Header().Get("Retry-After"))
	require.Equal(t, http.StatusOK, request("192.0.2.8", personalKey.Key, "").Code)
}
