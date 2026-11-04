package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/pkg/keygen"
	"github.com/fitraditya/agentsip/internal/pkg/password"
)

const (
	// sessionTTL is how long a session stays valid after login. 24 hours is
	// a deliberate middle ground for a dashboard login: short enough that a
	// forgotten, unrevoked session doesn't stay usable for weeks, long
	// enough that a person doesn't have to log back in mid-workday.
	sessionTTL = 24 * time.Hour

	// verifyEmailTTL is generous on purpose: verifying an email is not
	// security-sensitive the way a password reset is (it proves address
	// ownership, nothing more), and a person may not open their inbox the
	// same day they registered.
	verifyEmailTTL = 72 * time.Hour

	// passwordResetTTL is short — this token grants an active account
	// takeover to whoever presents it, so the window it's usable in is
	// deliberately much tighter than verifyEmailTTL's.
	passwordResetTTL = 1 * time.Hour
)

// Deps is everything NewService needs. A struct rather than positional
// parameters because the list has grown past what's easy to read correctly
// at a call site.
type Deps struct {
	Users       UserRepository
	Sessions    SessionRepository
	EmailTokens EmailTokenRepository
	// Mailer may be nil — every email-sending path degrades to "the token
	// exists and works, it just wasn't emailed" rather than failing the
	// request. See Mailer's doc comment.
	Mailer Mailer
	// PublicBaseURL, if set, is prefixed to verification/reset tokens to
	// build a clickable link (PublicBaseURL + "/verify-email?token=...").
	// Empty sends the raw token instead, for a deployment with no web
	// frontend yet — the API endpoints work identically either way.
	PublicBaseURL string
}

type service struct {
	users       UserRepository
	sessions    SessionRepository
	emailTokens EmailTokenRepository
	mailer      Mailer
	baseURL     string
}

func NewService(deps Deps) Service {
	return &service{
		users:       deps.Users,
		sessions:    deps.Sessions,
		emailTokens: deps.EmailTokens,
		mailer:      deps.Mailer,
		baseURL:     strings.TrimSuffix(deps.PublicBaseURL, "/"),
	}
}

func (s *service) Register(ctx context.Context, email, plainPassword string) (*domain.User, error) {
	email = normalizeEmail(email)
	if email == "" || plainPassword == "" {
		return nil, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": "email and password are both required",
		})
	}
	if len(plainPassword) < minPasswordLength {
		return nil, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": fmt.Sprintf("password must be at least %d characters", minPasswordLength),
		})
	}

	// Checked up front rather than left to the database's UNIQUE constraint:
	// a duplicate email is an ordinary, expected outcome here (someone
	// retrying signup), not the rare-race case trunks leaves to a DB-level
	// backstop.
	if existing, err := s.users.GetByEmail(ctx, email); err != nil {
		return nil, fmt.Errorf("auth: check existing email: %w", err)
	} else if existing != nil {
		return nil, apperror.ErrEmailAlreadyRegistered
	}

	u, err := s.createUser(ctx, email, plainPassword)
	if err != nil {
		return nil, err
	}

	// Best-effort: registration must succeed even if sending fails or no
	// Mailer is configured — see Deps.Mailer's doc comment.
	_ = s.issueAndSendEmailToken(ctx, u, domain.EmailTokenPurposeVerifyEmail,
		verifyEmailTTL, "Verify your email", "verify-email", verifyEmailBody)

	return u, nil
}

// createUser hashes plainPassword and persists a new User. It does not check
// for an existing email or send any email — callers (Register, Bootstrap)
// handle whatever preconditions and side effects are appropriate for them.
func (s *service) createUser(ctx context.Context, email, plainPassword string) (*domain.User, error) {
	hash, err := password.Hash(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("auth: hash password: %w", err)
	}

	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, fmt.Errorf("auth: create user: %w", err)
	}
	return u, nil
}

// Bootstrap is idempotent startup wiring: it creates exactly one user, via
// the same path as Register, if and only if no user exists yet anywhere in
// the system and both email and password are non-empty. It is a no-op
// otherwise. Unlike Register, it never creates a session — this is wiring
// run at process startup, not a login.
func (s *service) Bootstrap(ctx context.Context, email, plainPassword string) error {
	email = normalizeEmail(email)
	if email == "" || plainPassword == "" {
		return nil
	}

	exists, err := s.users.Any(ctx)
	if err != nil {
		return fmt.Errorf("auth: check for existing users: %w", err)
	}
	if exists {
		return nil
	}

	if len(plainPassword) < minPasswordLength {
		return apperror.ErrValidation.WithDetails(map[string]any{
			"reason": fmt.Sprintf("password must be at least %d characters", minPasswordLength),
		})
	}

	_, err = s.createUser(ctx, email, plainPassword)
	return err
}

func (s *service) Login(ctx context.Context, email, plainPassword string) (*domain.User, *IssuedSession, error) {
	email = normalizeEmail(email)
	if email == "" || plainPassword == "" {
		return nil, nil, apperror.ErrUnauthorized
	}

	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return nil, nil, fmt.Errorf("auth: lookup user: %w", err)
	}
	// Unknown email and wrong password collapse to the same response
	// deliberately — distinguishing them tells a prober which emails exist.
	if u == nil || !password.Verify(u.PasswordHash, plainPassword) {
		return nil, nil, apperror.ErrUnauthorized
	}

	issued, err := s.startSession(ctx, u)
	if err != nil {
		return nil, nil, err
	}
	return u, issued, nil
}

func (s *service) startSession(ctx context.Context, u *domain.User) (*IssuedSession, error) {
	plaintext, hash, err := keygen.GenerateWithPrefix(keygen.SessionPrefix)
	if err != nil {
		return nil, fmt.Errorf("auth: generate session token: %w", err)
	}

	now := time.Now().UTC()
	sess := &domain.Session{
		ID:        uuid.New(),
		UserID:    u.ID,
		TokenHash: hash,
		CreatedAt: now,
		ExpiresAt: now.Add(sessionTTL),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return nil, fmt.Errorf("auth: create session: %w", err)
	}
	return &IssuedSession{Session: sess, Plaintext: plaintext}, nil
}

func (s *service) Logout(ctx context.Context, presentedToken string) error {
	if presentedToken == "" {
		return nil
	}
	if _, err := s.sessions.Revoke(ctx, keygen.Hash(presentedToken)); err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

func (s *service) Authenticate(ctx context.Context, presentedToken string) (*domain.Session, error) {
	if presentedToken == "" {
		return nil, apperror.ErrUnauthorized
	}

	sess, err := s.sessions.GetByHash(ctx, keygen.Hash(presentedToken))
	if err != nil {
		return nil, fmt.Errorf("auth: lookup session: %w", err)
	}
	if sess == nil || sess.IsRevoked() || sess.IsExpired(time.Now().UTC()) {
		return nil, apperror.ErrUnauthorized
	}
	return sess, nil
}

// issueAndSendEmailToken generates one EmailToken, stores it, and — if a
// Mailer is configured — emails it to u.Email. Errors from either step are
// returned (so Register/RequestPasswordReset can log them) but are never
// meant to fail the caller's overall request; see each call site.
func (s *service) issueAndSendEmailToken(
	ctx context.Context, u *domain.User, purpose domain.EmailTokenPurpose, ttl time.Duration,
	subject, linkPath string, body func(link, token string) string,
) error {
	tokenPrefix := keygen.VerifyEmailTokenPrefix
	if purpose == domain.EmailTokenPurposePasswordReset {
		tokenPrefix = keygen.PasswordResetTokenPrefix
	}
	plaintext, hash, err := keygen.GenerateWithPrefix(tokenPrefix)
	if err != nil {
		return fmt.Errorf("auth: generate %s token: %w", purpose, err)
	}

	now := time.Now().UTC()
	tok := &domain.EmailToken{
		ID:        uuid.New(),
		UserID:    u.ID,
		Purpose:   purpose,
		TokenHash: hash,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.emailTokens.Create(ctx, tok); err != nil {
		return fmt.Errorf("auth: create %s token: %w", purpose, err)
	}

	if s.mailer == nil {
		return nil
	}
	link := plaintext
	if s.baseURL != "" {
		link = fmt.Sprintf("%s/%s?token=%s", s.baseURL, linkPath, plaintext)
	}
	if err := s.mailer.Send(u.Email, subject, body(link, plaintext)); err != nil {
		return fmt.Errorf("auth: send %s email: %w", purpose, err)
	}
	return nil
}

func verifyEmailBody(link, _ string) string {
	return "Confirm your email address by visiting the link below:\n\n" + link +
		"\n\nThis link expires in 72 hours. If you didn't create this account, ignore this email.\n"
}

func passwordResetBody(link, _ string) string {
	return "A password reset was requested for this account. Visit the link below to choose a new password:\n\n" + link +
		"\n\nThis link expires in 1 hour. If you didn't request this, ignore this email — your password will not change.\n"
}

func (s *service) VerifyEmail(ctx context.Context, presentedToken string) error {
	tok, err := s.redeemEmailToken(ctx, presentedToken, domain.EmailTokenPurposeVerifyEmail)
	if err != nil {
		return err
	}
	if err := s.users.MarkEmailVerified(ctx, tok.UserID, time.Now().UTC()); err != nil {
		return fmt.Errorf("auth: mark email verified: %w", err)
	}
	return nil
}

func (s *service) ResendVerificationEmail(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("auth: lookup user: %w", err)
	}
	if u == nil || u.IsEmailVerified() {
		return nil
	}
	if err := s.issueAndSendEmailToken(ctx, u, domain.EmailTokenPurposeVerifyEmail,
		verifyEmailTTL, "Verify your email", "verify-email", verifyEmailBody); err != nil {
		return fmt.Errorf("auth: resend verification: %w", err)
	}
	return nil
}

func (s *service) RequestPasswordReset(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	u, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("auth: lookup user: %w", err)
	}
	if u == nil {
		return nil
	}
	if err := s.issueAndSendEmailToken(ctx, u, domain.EmailTokenPurposePasswordReset,
		passwordResetTTL, "Reset your password", "reset-password", passwordResetBody); err != nil {
		return fmt.Errorf("auth: request password reset: %w", err)
	}
	return nil
}

func (s *service) ResetPassword(ctx context.Context, presentedToken, newPassword string) error {
	if len(newPassword) < minPasswordLength {
		return apperror.ErrValidation.WithDetails(map[string]any{
			"reason": fmt.Sprintf("password must be at least %d characters", minPasswordLength),
		})
	}

	tok, err := s.redeemEmailToken(ctx, presentedToken, domain.EmailTokenPurposePasswordReset)
	if err != nil {
		return err
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	if err := s.users.SetPasswordHash(ctx, tok.UserID, hash); err != nil {
		return fmt.Errorf("auth: set password hash: %w", err)
	}
	// A credential captured before the reset must not go on working after
	// it — see SessionRepository.RevokeAllForUser's doc comment.
	if err := s.sessions.RevokeAllForUser(ctx, tok.UserID); err != nil {
		return fmt.Errorf("auth: revoke sessions: %w", err)
	}
	return nil
}

func (s *service) Me(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: lookup user by id: %w", err)
	}
	if u == nil {
		return nil, apperror.ErrUnauthorized
	}
	return u, nil
}

func (s *service) UpdateEmail(ctx context.Context, userID uuid.UUID, newEmail, currentPassword string) (*domain.User, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("auth: lookup user by id: %w", err)
	}
	if u == nil {
		return nil, apperror.ErrUnauthorized
	}
	if !password.Verify(u.PasswordHash, currentPassword) {
		return nil, apperror.ErrUnauthorized.WithDetails(map[string]any{
			"reason": "current password is incorrect",
		})
	}

	newEmail = normalizeEmail(newEmail)
	if newEmail == "" {
		return nil, apperror.ErrValidation.WithDetails(map[string]any{
			"reason": "email is required",
		})
	}
	if newEmail != u.Email {
		// Checked up front rather than left to the database's UNIQUE
		// constraint — same reasoning as Register.
		if existing, err := s.users.GetByEmail(ctx, newEmail); err != nil {
			return nil, fmt.Errorf("auth: check existing email: %w", err)
		} else if existing != nil {
			return nil, apperror.ErrEmailAlreadyRegistered
		}
		if err := s.users.UpdateEmail(ctx, userID, newEmail); err != nil {
			return nil, fmt.Errorf("auth: update email: %w", err)
		}
		u.Email = newEmail
	}
	return u, nil
}

func (s *service) ChangePassword(ctx context.Context, userID uuid.UUID, currentPassword, newPassword string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("auth: lookup user by id: %w", err)
	}
	if u == nil {
		return apperror.ErrUnauthorized
	}
	if !password.Verify(u.PasswordHash, currentPassword) {
		return apperror.ErrUnauthorized.WithDetails(map[string]any{
			"reason": "current password is incorrect",
		})
	}
	if len(newPassword) < minPasswordLength {
		return apperror.ErrValidation.WithDetails(map[string]any{
			"reason": fmt.Sprintf("password must be at least %d characters", minPasswordLength),
		})
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("auth: hash password: %w", err)
	}
	if err := s.users.SetPasswordHash(ctx, userID, hash); err != nil {
		return fmt.Errorf("auth: set password hash: %w", err)
	}
	return nil
}

// redeemEmailToken validates a presented token against an expected purpose
// and marks it used, collapsing every failure mode (unknown, wrong purpose,
// expired, already used) into apperror.ErrUnauthorized — none of those
// distinctions is actionable by whoever is holding the token.
func (s *service) redeemEmailToken(ctx context.Context, presentedToken string, want domain.EmailTokenPurpose) (*domain.EmailToken, error) {
	if presentedToken == "" {
		return nil, apperror.ErrUnauthorized
	}

	hash := keygen.Hash(presentedToken)
	tok, err := s.emailTokens.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("auth: lookup %s token: %w", want, err)
	}
	if tok == nil || tok.Purpose != want || tok.IsUsed() || tok.IsExpired(time.Now().UTC()) {
		return nil, apperror.ErrUnauthorized
	}

	if _, err := s.emailTokens.MarkUsed(ctx, hash); err != nil {
		return nil, fmt.Errorf("auth: mark %s token used: %w", want, err)
	}
	return tok, nil
}

// minPasswordLength is a length floor, not a composition rule (no forced
// mixed-case/digit/symbol requirement) — length is what actually resists
// guessing, and composition rules mostly just push people toward
// predictable substitutions. ASSUMPTION: 8 is a reasonable minimum for a
// first cut; revisit alongside whatever password policy the org settles on.
const minPasswordLength = 8

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
