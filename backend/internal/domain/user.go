package domain

import (
	"time"

	"github.com/google/uuid"
)

// User is a human admin-dashboard login. Email is the login identifier,
// globally unique across all users.
type User struct {
	ID              uuid.UUID
	Email           string
	PasswordHash    string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (u *User) IsEmailVerified() bool {
	return u.EmailVerifiedAt != nil
}
