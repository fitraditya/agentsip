package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/auth"
)

// AuthHandler is mounted unauthenticated at the router root (router.go) —
// register and login are how a credential is obtained in the first place,
// so neither can require one; logout only needs whatever session token is
// being logged out, read directly from the header rather than through
// middleware.RequireAuth (which would reject an already-expired or
// already-revoked token instead of letting it log out cleanly).
type AuthHandler struct {
	svc auth.Service
}

func NewAuthHandler(svc auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

func (h *AuthHandler) Routes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)

	r.Post("/verify-email", h.verifyEmail)
	// GET too, so the link in the verification email can be clicked directly
	// in a browser rather than requiring a client to issue a POST.
	r.Get("/verify-email", h.verifyEmail)
	r.Post("/resend-verification", h.resendVerification)

	r.Post("/password-reset/request", h.requestPasswordReset)
	r.Post("/password-reset/confirm", h.confirmPasswordReset)

	// /me reads the Bearer token itself and authenticates manually — same
	// reasoning as logout: this handler's whole purpose is resolving who
	// that token belongs to, so middleware.RequireAuth (which validates but
	// discards the session) wouldn't give it anything to work with.
	r.Get("/me", h.me)
	r.Put("/me/email", h.updateEmail)
	r.Put("/me/password", h.changePassword)
}

// authenticate reads the Bearer token off the request and resolves it to a
// session, writing an error response and reporting false if that fails.
func (h *AuthHandler) authenticate(w http.ResponseWriter, r *http.Request) (*domain.Session, bool) {
	presented, ok := httpx.BearerToken(r)
	if !ok {
		httpx.WriteError(w, apperror.ErrUnauthorized)
		return nil, false
	}
	sess, err := h.svc.Authenticate(r.Context(), presented)
	if err != nil {
		httpx.WriteError(w, err)
		return nil, false
	}
	return sess, true
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID            uuid.UUID `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
}

func toUserResponse(u *domain.User) userResponse {
	return userResponse{
		ID: u.ID, Email: u.Email,
		EmailVerified: u.IsEmailVerified(), CreatedAt: u.CreatedAt,
	}
}

// toIssuedSessionResponse is the one response shape that carries a session's
// plaintext token — only ever returned from the call that just created it.
func toIssuedSessionResponse(issued *auth.IssuedSession) map[string]any {
	return map[string]any{
		"token":      issued.Plaintext,
		"expires_at": issued.Session.ExpiresAt,
	}
}

func (h *AuthHandler) register(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[registerRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	u, err := h.svc.Register(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{
		"user": toUserResponse(u),
	})
}

func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[loginRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	u, issued, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	httpx.WriteSuccess(w, http.StatusOK, map[string]any{
		"user":    toUserResponse(u),
		"session": toIssuedSessionResponse(issued),
	})
}

func (h *AuthHandler) logout(w http.ResponseWriter, r *http.Request) {
	presented, ok := httpx.BearerToken(r)
	if !ok {
		httpx.WriteError(w, apperror.ErrUnauthorized)
		return
	}

	if err := h.svc.Logout(r.Context(), presented); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"logged_out": true})
}

type tokenRequest struct {
	Token string `json:"token"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type confirmPasswordResetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// tokenFromRequest reads a token from either a query param (a clicked email
// link, GET /verify-email?token=...) or the JSON envelope body (a POST from
// a client), so both work against the same handler.
func tokenFromRequest(r *http.Request) (string, error) {
	if token := r.URL.Query().Get("token"); token != "" {
		return token, nil
	}
	req, err := httpx.DecodeEnvelope[tokenRequest](r)
	if err != nil {
		return "", apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"})
	}
	return req.Token, nil
}

func (h *AuthHandler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	token, err := tokenFromRequest(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := h.svc.VerifyEmail(r.Context(), token); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"verified": true})
}

func (h *AuthHandler) resendVerification(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[emailRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	if err := h.svc.ResendVerificationEmail(r.Context(), req.Email); err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Always the same response, whether or not the address exists or is
	// already verified — see Service.ResendVerificationEmail's doc comment.
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{
		"message": "if that email exists and is not yet verified, a verification link has been sent",
	})
}

func (h *AuthHandler) requestPasswordReset(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[emailRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	if err := h.svc.RequestPasswordReset(r.Context(), req.Email); err != nil {
		httpx.WriteError(w, err)
		return
	}
	// Always the same response — see Service.RequestPasswordReset's doc
	// comment on why an unregistered email must not be distinguishable here.
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{
		"message": "if that email is registered, a password reset link has been sent",
	})
}

func (h *AuthHandler) confirmPasswordReset(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[confirmPasswordResetRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	if err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"password_reset": true})
}

func (h *AuthHandler) me(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	u, err := h.svc.Me(r.Context(), sess.UserID)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": toUserResponse(u)})
}

type updateEmailRequest struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"current_password"`
}

func (h *AuthHandler) updateEmail(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	req, err := httpx.DecodeEnvelope[updateEmailRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	u, err := h.svc.UpdateEmail(r.Context(), sess.UserID, req.Email, req.CurrentPassword)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"user": toUserResponse(u)})
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *AuthHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	req, err := httpx.DecodeEnvelope[changePasswordRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	if err := h.svc.ChangePassword(r.Context(), sess.UserID, req.CurrentPassword, req.NewPassword); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"password_changed": true})
}
