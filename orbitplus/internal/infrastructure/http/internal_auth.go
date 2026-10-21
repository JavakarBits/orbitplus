package http

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// InternalTokenAuth gates service-to-service endpoints (e.g. the worker
// credential ping) with a shared bearer token. It is distinct from the
// cookie-based UI session gate (UIAccessAuth): workers present an
// Authorization: Bearer <token> header, not a UI session cookie.
type InternalTokenAuth struct {
	token []byte
}

// NewInternalTokenAuth constructs the gate. An empty token disables the
// protected endpoints (every request is rejected), so a missing token never
// silently exposes credentials.
func NewInternalTokenAuth(token string) *InternalTokenAuth {
	return &InternalTokenAuth{token: []byte(strings.TrimSpace(token))}
}

func (auth *InternalTokenAuth) enabled() bool {
	return auth != nil && len(auth.token) > 0
}

// Require wraps next so it only runs when the request carries the correct
// bearer token. Responses are JSON to match the other API handlers.
func (auth *InternalTokenAuth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if !auth.enabled() {
			writeJSONStatus(response, http.StatusServiceUnavailable, 0, "Internal API is not configured")
			return
		}
		supplied := bearerToken(request.Header.Get("Authorization"))
		if len(supplied) != len(auth.token) || subtle.ConstantTimeCompare(supplied, auth.token) != 1 {
			writeJSONStatus(response, http.StatusUnauthorized, 0, "Unauthorized")
			return
		}
		next.ServeHTTP(response, request)
	})
}

func bearerToken(header string) []byte {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return nil
	}
	return []byte(strings.TrimSpace(header[len(prefix):]))
}
