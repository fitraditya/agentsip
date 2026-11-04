// Package auth implements human admin-dashboard login: self-serve
// registration, login, logout, and authenticating a presented session
// token.
package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

type UserRepository interface {
	Create(ctx context.Context, u *domain.User) error
	// GetByEmail resolves a login identifier. Returns (nil, nil) when no
	// user matches; callers must treat that as an authentication failure,
	// not an error.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	// GetByID resolves a user by their primary key — used to rehydrate the
	// signed-in user's own profile (GET /me). Returns (nil, nil) when no
	// user matches.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	// SetPasswordHash overwrites a user's password hash — the only field
	// ResetPassword changes. A narrow setter rather than a general Update,
	// so a future field can't be rewritten from the wrong call site by
	// accident.
	SetPasswordHash(ctx context.Context, userID uuid.UUID, hash string) error
	// UpdateEmail overwrites a user's email. Callers check for an existing
	// owner of the new address themselves (same up-front-check convention
	// as Register) rather than relying on the column's UNIQUE constraint.
	UpdateEmail(ctx context.Context, userID uuid.UUID, newEmail string) error
	// MarkEmailVerified sets email_verified_at. Idempotent: verifying an
	// already-verified email is not an error.
	MarkEmailVerified(ctx context.Context, userID uuid.UUID, at time.Time) error
	// Any reports whether at least one user exists in the system. Used by
	// Bootstrap to decide whether startup wiring still needs to create the
	// first user — implemented as a trivial
	// `SELECT EXISTS(SELECT 1 FROM users)`-style query.
	Any(ctx context.Context) (bool, error)
}

type SessionRepository interface {
	Create(ctx context.Context, s *domain.Session) error
	// GetByHash resolves a presented session token's digest. Returns
	// (nil, nil) when no session matches.
	GetByHash(ctx context.Context, hash string) (*domain.Session, error)
	// Revoke reports whether a row was actually updated — false covers both
	// a nonexistent session and one already revoked, which Logout does not
	// need to distinguish.
	Revoke(ctx context.Context, hash string) (bool, error)
	// RevokeAllForUser ends every live session for a user. Called on
	// password reset: a credential stolen before the reset must not go on
	// working after it, which simply changing the password hash would not
	// by itself guarantee — a session is a separate, already-issued bearer
	// token that owes nothing to the password once it exists.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) error
}

// EmailTokenRepository backs both email verification and password reset —
// one table, two domain.EmailTokenPurpose values (see domain.EmailToken's
// doc comment for why they share a shape).
type EmailTokenRepository interface {
	Create(ctx context.Context, t *domain.EmailToken) error
	// GetByHash resolves a presented token's digest, regardless of purpose
	// — the caller checks Purpose against what it expected. Returns
	// (nil, nil) when no token matches.
	GetByHash(ctx context.Context, hash string) (*domain.EmailToken, error)
	// MarkUsed reports whether a row was actually updated — false covers
	// both a nonexistent token and one already used, which callers do not
	// need to distinguish (same reasoning as SessionRepository.Revoke).
	MarkUsed(ctx context.Context, hash string) (bool, error)
}

// Mailer sends the verification/reset emails. Declared here rather than
// importing adapter/mail directly, so this package depends on the smallest
// possible contract — adapter/mail.Sender satisfies it structurally.
// Registration and password-reset requests must still succeed when no
// Mailer is configured (ErrNotConfigured, or Mailer being nil): the token is
// still created and usable by whoever has it directly, the email is just
// best-effort delivery on top.
type Mailer interface {
	Send(to, subject, body string) error
}

// IssuedSession pairs a freshly created session record with its plaintext
// token. Plaintext is populated only here, only at creation, and is never
// persisted — the handler returns it to the caller once and then it is gone.
type IssuedSession struct {
	Session   *domain.Session
	Plaintext string
}

type Service interface {
	// Register creates a new user (self-serve signup — there is no invite
	// flow). It does not log the user in; callers that need an active
	// session call Login afterward.
	Register(ctx context.Context, email, plainPassword string) (*domain.User, error)
	// Bootstrap is idempotent startup wiring, not a login: it creates
	// exactly one user (via the same path as Register) if and only if no
	// user exists yet in the system and both email and password are
	// non-empty. It is a no-op (returns nil) otherwise — including when a
	// user already exists, or when email or password is empty. It never
	// creates a session.
	Bootstrap(ctx context.Context, email, plainPassword string) error
	// Login authenticates an email/password pair and starts a new session.
	Login(ctx context.Context, email, plainPassword string) (*domain.User, *IssuedSession, error)
	// Logout revokes the session behind a presented token. Not found and
	// already-revoked both report no error — logging out twice is not a
	// failure.
	Logout(ctx context.Context, presentedToken string) error
	// Authenticate resolves a presented session token to the session it
	// belongs to, or returns apperror.ErrUnauthorized.
	Authenticate(ctx context.Context, presentedToken string) (*domain.Session, error)

	// VerifyEmail redeems a verify_email token, marking the user's email
	// verified. A used, expired, unknown, or wrong-purpose token all report
	// the same apperror.ErrUnauthorized — none of those distinctions is
	// something the caller (someone who clicked a link) can act on.
	VerifyEmail(ctx context.Context, presentedToken string) error
	// ResendVerificationEmail issues a fresh verify_email token and emails
	// it, replacing any still-outstanding one is unnecessary — old tokens
	// simply expire on their own. Always returns nil: whether the address
	// exists or is already verified is not something a caller should be
	// able to learn from this response (same reasoning as RequestPasswordReset).
	ResendVerificationEmail(ctx context.Context, email string) error

	// RequestPasswordReset issues a password_reset token and emails it, if
	// the address belongs to a user. Always returns nil regardless —
	// whether an email is registered must not be learnable from this
	// endpoint's response.
	RequestPasswordReset(ctx context.Context, email string) error
	// ResetPassword redeems a password_reset token, sets the new password,
	// and revokes every existing session for that user. Same error
	// collapsing as VerifyEmail.
	ResetPassword(ctx context.Context, presentedToken, newPassword string) error

	// Me returns the signed-in user's own profile. Returns
	// apperror.ErrUnauthorized if the user no longer exists (e.g. deleted
	// out from under a still-live session).
	Me(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	// UpdateEmail changes the signed-in user's email, after verifying
	// currentPassword against their stored hash. Returns
	// apperror.ErrEmailAlreadyRegistered if another user already holds
	// newEmail.
	UpdateEmail(ctx context.Context, userID uuid.UUID, newEmail, currentPassword string) (*domain.User, error)
	// ChangePassword sets a new password for the signed-in user, after
	// verifying currentPassword against their stored hash. Unlike
	// ResetPassword, it does not revoke other sessions — the caller already
	// proved possession of the current password, not just a mailed token.
	ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error
}
