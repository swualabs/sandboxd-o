package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newGuardedRouter(secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api", Middleware(secret))
	g.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestMiddleware(t *testing.T) {
	cases := []struct {
		name   string
		secret string
		header string
		want   int
	}{
		{name: "valid bearer token", secret: "s3cr3t", header: "Bearer s3cr3t", want: http.StatusOK},
		{name: "valid bare token", secret: "s3cr3t", header: "s3cr3t", want: http.StatusOK},
		{name: "case-insensitive scheme", secret: "s3cr3t", header: "bearer s3cr3t", want: http.StatusOK},
		{name: "missing token", secret: "s3cr3t", header: "", want: http.StatusUnauthorized},
		{name: "wrong token", secret: "s3cr3t", header: "Bearer nope", want: http.StatusUnauthorized},
		{name: "empty configured secret fails closed", secret: "", header: "Bearer anything", want: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newGuardedRouter(tc.secret)
			req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
			if tc.header != "" {
				req.Header.Set(HeaderAuthorization, tc.header)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("code=%d want=%d", w.Code, tc.want)
			}
		})
	}
}

func TestSetRequestSecret(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	SetRequestSecret(req, "abc")
	if got := req.Header.Get(HeaderAuthorization); got != "Bearer abc" {
		t.Fatalf("header=%q", got)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	SetRequestSecret(req2, "")
	if got := req2.Header.Get(HeaderAuthorization); got != "" {
		t.Fatalf("expected no header for empty secret, got %q", got)
	}

	// Must not panic on a nil request.
	SetRequestSecret(nil, "abc")
}
