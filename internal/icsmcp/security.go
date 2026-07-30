package icsmcp

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// HTTPOptions configures the public HTTP boundary. An empty bearer token keeps
// the trusted-local behavior used by earlier releases.
type HTTPOptions struct {
	BearerToken string
}

func authMiddleware(next http.Handler, token string) http.Handler {
	token = strings.TrimSpace(token)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		provided := strings.TrimSpace(r.Header.Get("X-ICS-MCP-Token"))
		if provided == "" {
			if scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " "); ok && strings.EqualFold(scheme, "Bearer") {
				provided = strings.TrimSpace(value)
			}
		}
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="icsmcp"`)
			writeError(w, http.StatusUnauthorized, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
