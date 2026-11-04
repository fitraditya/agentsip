// Package middleware holds cross-cutting chi middleware. RequireAuth is the
// admin API's authorization gate: every handler behind it is reachable only
// by a request that presented a valid credential.
package middleware

import (
	"context"
	"net/http"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

// Authenticator validates a presented bearer credential — usecase/auth.Service
// (human session tokens), via the adapter/http.sessionAuthenticator wrapper.
type Authenticator interface {
	Authenticate(ctx context.Context, presented string) error
}

// RequireAuth authenticates the Authorization: Bearer credential and rejects
// the request if it doesn't validate. On success it calls next unchanged —
// there is no per-request identity to thread through context.
func RequireAuth(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			presented, ok := httpx.BearerToken(r)
			if !ok {
				httpx.WriteError(w, apperror.ErrUnauthorized)
				return
			}

			if err := auth.Authenticate(r.Context(), presented); err != nil {
				httpx.WriteError(w, err)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
