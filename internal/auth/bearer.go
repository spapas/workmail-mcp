// Package auth contains authentication middleware for localhost HTTP mode.
package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Bearer wraps an HTTP handler with constant-time bearer-token validation.
func Bearer(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(strings.TrimPrefix(header, prefix))), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="workmail-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
