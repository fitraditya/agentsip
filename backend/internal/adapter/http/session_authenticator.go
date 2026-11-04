package http

import (
	"context"

	"github.com/fitraditya/agentsip/internal/usecase/auth"
)

// sessionAuthenticator adapts auth.Service to middleware.Authenticator —
// RequireAuth only needs "is this bearer token valid," not the session
// itself.
type sessionAuthenticator struct {
	sessions auth.Service
}

func (a sessionAuthenticator) Authenticate(ctx context.Context, presented string) error {
	_, err := a.sessions.Authenticate(ctx, presented)
	return err
}
