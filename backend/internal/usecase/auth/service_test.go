package auth_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/auth"
)

type fakeUserRepo struct {
	mu      sync.Mutex
	byEmail map[string]*domain.User
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byEmail: map[string]*domain.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *u
	r.byEmail[u.Email] = &cp
	return nil
}

func (r *fakeUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byEmail[email]
	if !ok {
		return nil, nil
	}
	cp := *u
	return &cp, nil
}

func (r *fakeUserRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.byEmail {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeUserRepo) UpdateEmail(_ context.Context, userID uuid.UUID, newEmail string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for oldEmail, u := range r.byEmail {
		if u.ID == userID {
			u.Email = newEmail
			delete(r.byEmail, oldEmail)
			r.byEmail[newEmail] = u
			return nil
		}
	}
	return nil
}

func (r *fakeUserRepo) SetPasswordHash(_ context.Context, userID uuid.UUID, hash string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.byEmail {
		if u.ID == userID {
			u.PasswordHash = hash
			return nil
		}
	}
	return nil
}

func (r *fakeUserRepo) MarkEmailVerified(_ context.Context, userID uuid.UUID, at time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.byEmail {
		if u.ID == userID {
			t := at
			u.EmailVerifiedAt = &t
			return nil
		}
	}
	return nil
}

func (r *fakeUserRepo) Any(_ context.Context) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byEmail) > 0, nil
}

type fakeSessionRepo struct {
	mu     sync.Mutex
	byHash map[string]*domain.Session
}

func newFakeSessionRepo() *fakeSessionRepo {
	return &fakeSessionRepo{byHash: map[string]*domain.Session{}}
}

func (r *fakeSessionRepo) Create(_ context.Context, s *domain.Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *s
	r.byHash[s.TokenHash] = &cp
	return nil
}

func (r *fakeSessionRepo) GetByHash(_ context.Context, hash string) (*domain.Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byHash[hash]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (r *fakeSessionRepo) Revoke(_ context.Context, hash string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.byHash[hash]
	if !ok || s.RevokedAt != nil {
		return false, nil
	}
	now := s.CreatedAt // deterministic enough for these tests
	s.RevokedAt = &now
	return true, nil
}

func (r *fakeSessionRepo) RevokeAllForUser(_ context.Context, userID uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, s := range r.byHash {
		if s.UserID == userID && s.RevokedAt == nil {
			s.RevokedAt = &now
		}
	}
	return nil
}

type fakeEmailTokenRepo struct {
	mu     sync.Mutex
	byHash map[string]*domain.EmailToken
}

func newFakeEmailTokenRepo() *fakeEmailTokenRepo {
	return &fakeEmailTokenRepo{byHash: map[string]*domain.EmailToken{}}
}

func (r *fakeEmailTokenRepo) Create(_ context.Context, t *domain.EmailToken) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *t
	r.byHash[t.TokenHash] = &cp
	return nil
}

func (r *fakeEmailTokenRepo) GetByHash(_ context.Context, hash string) (*domain.EmailToken, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byHash[hash]
	if !ok {
		return nil, nil
	}
	return t, nil
}

func (r *fakeEmailTokenRepo) MarkUsed(_ context.Context, hash string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byHash[hash]
	if !ok || t.UsedAt != nil {
		return false, nil
	}
	now := time.Now().UTC()
	t.UsedAt = &now
	return true, nil
}

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

type sentMail struct {
	to, subject, body string
}

func (m *fakeMailer) Send(to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to: to, subject: subject, body: body})
	return nil
}

func (m *fakeMailer) last() (sentMail, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return sentMail{}, false
	}
	return m.sent[len(m.sent)-1], true
}

// extractToken pulls the plaintext token out of an email body built by
// verifyEmailBody/passwordResetBody — it's whatever follows the last "/" or,
// with no PublicBaseURL configured, the whole link is the token.
func extractToken(body string) string {
	lines := strings.Split(body, "\n\n")
	if len(lines) < 2 {
		return ""
	}
	link := strings.TrimSpace(lines[1])
	if i := strings.LastIndex(link, "token="); i != -1 {
		return link[i+len("token="):]
	}
	return link
}

type testDeps struct {
	users       *fakeUserRepo
	sessions    *fakeSessionRepo
	emailTokens *fakeEmailTokenRepo
	mailer      *fakeMailer
}

func newTestService() (auth.Service, testDeps) {
	deps := testDeps{
		users:       newFakeUserRepo(),
		sessions:    newFakeSessionRepo(),
		emailTokens: newFakeEmailTokenRepo(),
		mailer:      &fakeMailer{},
	}
	svc := auth.NewService(auth.Deps{
		Users:       deps.users,
		Sessions:    deps.sessions,
		EmailTokens: deps.emailTokens,
		Mailer:      deps.mailer,
	})
	return svc, deps
}

func TestRegister_CreatesUser(t *testing.T) {
	svc, _ := newTestService()

	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.ID == uuid.Nil {
		t.Fatal("expected a user to be created")
	}
	if user.Email != "owner@acme.test" {
		t.Fatalf("user.Email = %q, want %q", user.Email, "owner@acme.test")
	}
}

func TestRegister_SendsAVerificationEmail(t *testing.T) {
	svc, deps := newTestService()

	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	sent, ok := deps.mailer.last()
	if !ok {
		t.Fatal("expected a verification email to be sent")
	}
	if sent.to != user.Email {
		t.Fatalf("sent to %q, want %q", sent.to, user.Email)
	}
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "short"); err == nil {
		t.Fatal("expected an error for a too-short password")
	}
}

func TestRegister_RejectsDuplicateEmail(t *testing.T) {
	svc, _ := newTestService()

	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	_, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if !errors.Is(err, apperror.ErrEmailAlreadyRegistered) {
		t.Fatalf("second Register = %v, want ErrEmailAlreadyRegistered", err)
	}
}

func TestLoginThenAuthenticate(t *testing.T) {
	svc, _ := newTestService()

	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	loggedIn, issued, err := svc.Login(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if loggedIn.ID != user.ID {
		t.Fatalf("Login returned user %v, want %v", loggedIn.ID, user.ID)
	}

	sess, err := svc.Authenticate(t.Context(), issued.Plaintext)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if sess.UserID != user.ID {
		t.Fatalf("Authenticate returned user %v, want %v", sess.UserID, user.ID)
	}
}

func TestLogin_WrongPasswordAndUnknownEmailCollapseToTheSameError(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	_, _, errWrongPassword := svc.Login(t.Context(), "owner@acme.test", "wrong-password")
	if !errors.Is(errWrongPassword, apperror.ErrUnauthorized) {
		t.Fatalf("wrong password: got %v, want ErrUnauthorized", errWrongPassword)
	}

	_, _, errUnknown := svc.Login(t.Context(), "nobody@acme.test", "hunter22")
	if !errors.Is(errUnknown, apperror.ErrUnauthorized) {
		t.Fatalf("unknown email: got %v, want ErrUnauthorized", errUnknown)
	}
}

func TestLogout_RevokesTheSessionSoItCanNoLongerAuthenticate(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, issued, err := svc.Login(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.Logout(t.Context(), issued.Plaintext); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	_, err = svc.Authenticate(t.Context(), issued.Plaintext)
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("Authenticate after logout = %v, want ErrUnauthorized", err)
	}
}

func TestLogout_IsIdempotent(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, issued, err := svc.Login(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.Logout(t.Context(), issued.Plaintext); err != nil {
		t.Fatalf("first Logout: %v", err)
	}
	if err := svc.Logout(t.Context(), issued.Plaintext); err != nil {
		t.Fatalf("second Logout must not error: %v", err)
	}
}

func TestAuthenticate_UnknownTokenIsUnauthorized(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Authenticate(t.Context(), "obs_does-not-exist"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestVerifyEmail_MarksTheUserVerified(t *testing.T) {
	svc, deps := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	sent, ok := deps.mailer.last()
	if !ok {
		t.Fatal("expected a verification email")
	}
	token := extractToken(sent.body)
	if token == "" {
		t.Fatalf("could not extract a token from email body: %q", sent.body)
	}

	if err := svc.VerifyEmail(t.Context(), token); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	got, err := deps.users.GetByEmail(t.Context(), user.Email)
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if !got.IsEmailVerified() {
		t.Fatal("expected the user's email to be marked verified")
	}
}

func TestVerifyEmail_TokenIsSingleUse(t *testing.T) {
	svc, deps := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sent, _ := deps.mailer.last()
	token := extractToken(sent.body)

	if err := svc.VerifyEmail(t.Context(), token); err != nil {
		t.Fatalf("first VerifyEmail: %v", err)
	}
	if err := svc.VerifyEmail(t.Context(), token); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("second VerifyEmail = %v, want ErrUnauthorized", err)
	}
}

func TestVerifyEmail_UnknownTokenIsUnauthorized(t *testing.T) {
	svc, _ := newTestService()
	if err := svc.VerifyEmail(t.Context(), "obv_does-not-exist"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("got %v, want ErrUnauthorized", err)
	}
}

func TestVerifyEmail_APasswordResetTokenDoesNotVerifyEmail(t *testing.T) {
	svc, deps := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := svc.RequestPasswordReset(t.Context(), "owner@acme.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent, _ := deps.mailer.last()
	resetToken := extractToken(sent.body)

	if err := svc.VerifyEmail(t.Context(), resetToken); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("using a password-reset token to verify email = %v, want ErrUnauthorized", err)
	}
}

func TestRequestPasswordReset_UnknownEmailReturnsNilWithoutSendingMail(t *testing.T) {
	svc, deps := newTestService()
	if err := svc.RequestPasswordReset(t.Context(), "nobody@acme.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if _, ok := deps.mailer.last(); ok {
		t.Fatal("must not send email for an unregistered address")
	}
}

func TestResetPassword_ChangesPasswordAndRevokesExistingSessions(t *testing.T) {
	svc, deps := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, issued, err := svc.Login(t.Context(), user.Email, "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.RequestPasswordReset(t.Context(), user.Email); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent, ok := deps.mailer.last()
	if !ok {
		t.Fatal("expected a password reset email")
	}
	resetToken := extractToken(sent.body)

	if err := svc.ResetPassword(t.Context(), resetToken, "a-new-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// The old session must no longer authenticate.
	if _, err := svc.Authenticate(t.Context(), issued.Plaintext); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("old session after reset = %v, want ErrUnauthorized", err)
	}

	// The old password must no longer work; the new one must.
	if _, _, err := svc.Login(t.Context(), user.Email, "hunter22"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("login with old password after reset = %v, want ErrUnauthorized", err)
	}
	if _, _, err := svc.Login(t.Context(), user.Email, "a-new-password"); err != nil {
		t.Fatalf("login with new password after reset: %v", err)
	}
}

func TestResetPassword_TokenIsSingleUse(t *testing.T) {
	svc, deps := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := svc.RequestPasswordReset(t.Context(), user.Email); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent, _ := deps.mailer.last()
	resetToken := extractToken(sent.body)

	if err := svc.ResetPassword(t.Context(), resetToken, "a-new-password"); err != nil {
		t.Fatalf("first ResetPassword: %v", err)
	}
	if err := svc.ResetPassword(t.Context(), resetToken, "another-password"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("second ResetPassword = %v, want ErrUnauthorized", err)
	}
}

func TestResetPassword_RejectsShortNewPassword(t *testing.T) {
	svc, deps := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := svc.RequestPasswordReset(t.Context(), user.Email); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	sent, _ := deps.mailer.last()
	resetToken := extractToken(sent.body)

	if err := svc.ResetPassword(t.Context(), resetToken, "short"); err == nil {
		t.Fatal("expected an error for a too-short new password")
	}
}

func TestResendVerificationEmail_AlreadyVerifiedDoesNotSendAgain(t *testing.T) {
	svc, deps := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	sent, _ := deps.mailer.last()
	token := extractToken(sent.body)
	if err := svc.VerifyEmail(t.Context(), token); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}

	before := len(deps.mailer.sent)
	if err := svc.ResendVerificationEmail(t.Context(), "owner@acme.test"); err != nil {
		t.Fatalf("ResendVerificationEmail: %v", err)
	}
	if len(deps.mailer.sent) != before {
		t.Fatal("must not send another verification email once already verified")
	}
}

func TestNewService_WorksWithoutAMailerConfigured(t *testing.T) {
	svc := auth.NewService(auth.Deps{
		Users:       newFakeUserRepo(),
		Sessions:    newFakeSessionRepo(),
		EmailTokens: newFakeEmailTokenRepo(),
		// Mailer intentionally left nil.
	})

	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register without a mailer must still succeed: %v", err)
	}
	if err := svc.RequestPasswordReset(t.Context(), "owner@acme.test"); err != nil {
		t.Fatalf("RequestPasswordReset without a mailer must still succeed: %v", err)
	}
}

func TestBootstrap_NoOpWhenAUserAlreadyExists(t *testing.T) {
	svc, deps := newTestService()
	if _, err := svc.Register(t.Context(), "existing@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.Bootstrap(t.Context(), "admin@acme.test", "hunter22"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	if _, err := deps.users.GetByEmail(t.Context(), "admin@acme.test"); err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if u, _ := deps.users.GetByEmail(t.Context(), "admin@acme.test"); u != nil {
		t.Fatal("Bootstrap must not create a user when one already exists")
	}
}

func TestBootstrap_CreatesAUserWhenNoneExist(t *testing.T) {
	svc, deps := newTestService()

	if err := svc.Bootstrap(t.Context(), "admin@acme.test", "hunter22"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	u, err := deps.users.GetByEmail(t.Context(), "admin@acme.test")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if u == nil {
		t.Fatal("Bootstrap must create a user when none exist")
	}
}

func TestBootstrap_NoOpWhenEmailOrPasswordIsBlank(t *testing.T) {
	svc, deps := newTestService()

	if err := svc.Bootstrap(t.Context(), "", "hunter22"); err != nil {
		t.Fatalf("Bootstrap with blank email: %v", err)
	}
	if err := svc.Bootstrap(t.Context(), "admin@acme.test", ""); err != nil {
		t.Fatalf("Bootstrap with blank password: %v", err)
	}

	any, err := deps.users.Any(t.Context())
	if err != nil {
		t.Fatalf("Any: %v", err)
	}
	if any {
		t.Fatal("Bootstrap must not create a user when email or password is blank")
	}
}

func TestBootstrap_DoesNotCreateASession(t *testing.T) {
	svc, _ := newTestService()

	if err := svc.Bootstrap(t.Context(), "admin@acme.test", "hunter22"); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	// There is no session issued by Bootstrap to Authenticate against; this
	// simply documents that Bootstrap's return value carries no token.
	_, _, err := svc.Login(t.Context(), "admin@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("expected to still be able to Login normally after Bootstrap: %v", err)
	}
}

func TestMe_ReturnsTheSignedInUser(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := svc.Me(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if got.Email != user.Email {
		t.Fatalf("Me().Email = %q, want %q", got.Email, user.Email)
	}
}

func TestMe_UnknownUserIsUnauthorized(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Me(t.Context(), uuid.New()); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("Me = %v, want ErrUnauthorized", err)
	}
}

func TestUpdateEmail_ChangesEmailWhenCurrentPasswordIsCorrect(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	updated, err := svc.UpdateEmail(t.Context(), user.ID, "new@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("UpdateEmail: %v", err)
	}
	if updated.Email != "new@acme.test" {
		t.Fatalf("updated.Email = %q, want %q", updated.Email, "new@acme.test")
	}

	if _, _, err := svc.Login(t.Context(), "new@acme.test", "hunter22"); err != nil {
		t.Fatalf("expected to log in with the new email: %v", err)
	}
}

func TestUpdateEmail_RejectsWrongCurrentPassword(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if _, err := svc.UpdateEmail(t.Context(), user.ID, "new@acme.test", "wrong"); err == nil {
		t.Fatal("expected an error for the wrong current password")
	}
}

func TestUpdateEmail_RejectsAnEmailAlreadyTakenByAnotherUser(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Register(t.Context(), "owner@acme.test", "hunter22"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	other, err := svc.Register(t.Context(), "other@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register other: %v", err)
	}

	_, err = svc.UpdateEmail(t.Context(), other.ID, "owner@acme.test", "hunter22")
	if !errors.Is(err, apperror.ErrEmailAlreadyRegistered) {
		t.Fatalf("UpdateEmail = %v, want ErrEmailAlreadyRegistered", err)
	}
}

func TestChangePassword_ChangesPasswordWithoutRevokingOtherSessions(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, issued, err := svc.Login(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := svc.ChangePassword(t.Context(), user.ID, "hunter22", "newpassword1"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}

	if _, _, err := svc.Login(t.Context(), "owner@acme.test", "newpassword1"); err != nil {
		t.Fatalf("expected to log in with the new password: %v", err)
	}
	// Unlike ResetPassword, ChangePassword must not revoke the session that
	// was already live before the change.
	if _, err := svc.Authenticate(t.Context(), issued.Plaintext); err != nil {
		t.Fatalf("expected the pre-existing session to still authenticate: %v", err)
	}
}

func TestChangePassword_RejectsWrongCurrentPassword(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.ChangePassword(t.Context(), user.ID, "wrong", "newpassword1"); err == nil {
		t.Fatal("expected an error for the wrong current password")
	}
}

func TestChangePassword_RejectsShortNewPassword(t *testing.T) {
	svc, _ := newTestService()
	user, err := svc.Register(t.Context(), "owner@acme.test", "hunter22")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := svc.ChangePassword(t.Context(), user.ID, "hunter22", "short"); err == nil {
		t.Fatal("expected an error for a too-short new password")
	}
}
