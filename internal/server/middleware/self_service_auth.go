package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/looplj/axonhub/internal/contexts"
	"github.com/looplj/axonhub/internal/ent"
	"github.com/looplj/axonhub/internal/ent/apikey"
	"github.com/looplj/axonhub/internal/server/biz"
)

// WithSelfServiceAPIKeyAuth authenticates user and personal API keys for the
// self-service GraphQL endpoint. No-auth fallback and service accounts are
// intentionally excluded, even when no-auth is enabled on AuthService.
func WithSelfServiceAPIKeyAuth(auth *biz.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		invalidKey := func() {
			blocked, retryAfter := RecordSelfServiceAuthFailure(c.Request)
			if blocked {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
				AbortWithError(c, http.StatusTooManyRequests, errors.New("Too many authentication attempts"))
				return
			}
			AbortWithError(c, http.StatusUnauthorized, errors.New("Invalid API key"))
		}

		key, err := ExtractAPIKeyFromRequest(c.Request, apiKeyAuthConfig)
		if err != nil || key == biz.NoAuthAPIKeyValue {
			invalidKey()
			return
		}

		apiKey, err := auth.AuthenticateAPIKey(c.Request.Context(), key)
		if err != nil {
			if ent.IsNotFound(err) || errors.Is(err, biz.ErrInvalidAPIKey) {
				invalidKey()
			} else {
				AbortWithError(c, http.StatusInternalServerError, errors.New("Failed to validate API key"))
			}
			return
		}
		if apiKey == nil {
			invalidKey()
			return
		}
		if apiKey.Type != apikey.TypeUser && apiKey.Type != apikey.TypePersonal {
			invalidKey()
			return
		}

		if len(apiKey.AllowedIps) > 0 && !isAnyAllowedIP(clientIPCandidates(c), apiKey.AllowedIps) {
			AbortWithError(c, http.StatusForbidden, errors.New("IP address is not allowed for this API key"))
			return
		}

		ctx := contexts.WithAPIKey(c.Request.Context(), apiKey)
		if apiKey.Edges.Project != nil {
			ctx = contexts.WithProjectID(ctx, apiKey.Edges.Project.ID)
		}
		ctx = withSessionScopeForAPIKey(ctx, apiKey)
		ctx, err = withAPIKeyPrincipal(ctx, apiKey)
		if err != nil {
			invalidKey()
			return
		}

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
