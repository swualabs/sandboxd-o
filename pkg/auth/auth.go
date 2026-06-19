// Package auth provides a minimal static shared-secret authentication scheme
// shared by sbxorch, sbxlet, and sbxctl.
//
// Every component is configured with the same secret. Outgoing requests carry
// it in an "Authorization: Bearer <secret>" header, and servers reject any
// request whose token does not match (constant-time comparison). This is the
// most basic mechanism described in issues #22 and #23: communication only
// succeeds when both sides share an identical secret.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	// HeaderAuthorization is the request header carrying the bearer token.
	HeaderAuthorization = "Authorization"
	bearerPrefix        = "Bearer "
)

// SetRequestSecret attaches the shared secret to an outgoing request. A blank
// secret leaves the request untouched; the server will then reject it.
func SetRequestSecret(req *http.Request, secret string) {
	if req == nil || secret == "" {
		return
	}

	req.Header.Set(HeaderAuthorization, bearerPrefix+secret)
}

// tokenFromHeader extracts the bearer token from an Authorization header value.
// The "Bearer " scheme prefix is optional so a bare token is also accepted.
func tokenFromHeader(value string) string {
	v := strings.TrimSpace(value)
	if len(v) >= len(bearerPrefix) && strings.EqualFold(v[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(v[len(bearerPrefix):])
	}

	return v
}

// Middleware returns a gin middleware that enforces the shared secret on every
// request it guards. It fails closed: if the configured secret is empty, all
// requests are rejected.
func Middleware(secret string) gin.HandlerFunc {
	expected := []byte(secret)
	return func(c *gin.Context) {
		if secret == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		got := []byte(tokenFromHeader(c.GetHeader(HeaderAuthorization)))
		if subtle.ConstantTimeCompare(got, expected) != 1 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Next()
	}
}
