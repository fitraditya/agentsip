package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmailTokenPurpose distinguishes what an EmailToken authorizes — the same
// row shape serves email verification and password reset, since both are
// "a single-use, time-limited, emailed proof that this address's owner
// clicked a link," differing only in what that proof is then allowed to do.
type EmailTokenPurpose string

const (
	EmailTokenPurposeVerifyEmail   EmailTokenPurpose = "verify_email"
	EmailTokenPurposePasswordReset EmailTokenPurpose = "password_reset"
)

// EmailToken is a single-use, emailed credential.
//
// TokenHash is a SHA-256 digest (hex-encoded for storage), same convention
// as Session.TokenHash; the plaintext exists only in the email it was sent
// in and is never recoverable afterwards.
type EmailToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Purpose   EmailTokenPurpose
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

func (t *EmailToken) IsUsed() bool {
	return t.UsedAt != nil
}

func (t *EmailToken) IsExpired(now time.Time) bool {
	return now.After(t.ExpiresAt)
}
