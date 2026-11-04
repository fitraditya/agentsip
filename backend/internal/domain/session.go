package domain

import (
	"time"

	"github.com/google/uuid"
)

// Session is a logged-in User's bearer credential.
//
// TokenHash is a SHA-256 digest (hex-encoded for storage) — the plaintext
// token exists only in the login response and is never recoverable
// afterwards.
type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (s *Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

func (s *Session) IsExpired(now time.Time) bool {
	return now.After(s.ExpiresAt)
}
