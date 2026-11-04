// Package apperror defines the uniform error shape every usecase and adapter
// returns instead of a bare error, so a handler can map it to the correct
// HTTP status and wire shape without string matching. Mirrors obrelin's
// internal/pkg/apperror — same *Error{Code, Message, Details, HTTPStatus}
// type and the same WithDetails pattern for dynamic detail, kept consistent
// deliberately: these are sibling services, and a client library written
// against one error envelope should work against both.
package apperror

import "net/http"

// Error is the typed application error every usecase/adapter returns instead
// of a bare error, so handlers can map it to the correct HTTP status without
// string matching.
type Error struct {
	Code       string
	Message    string
	Details    map[string]any
	HTTPStatus int
}

func (e *Error) Error() string { return e.Message }

func New(code, message string, httpStatus int) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: httpStatus}
}

// WithDetails returns a NEW *Error carrying details — it must never mutate e
// in place. The sentinels below are shared package-level singletons; every
// call site in this codebase calls apperror.ErrX.WithDetails(...) directly
// on them, so an in-place mutation here would be a data race across every
// concurrent request using the same sentinel.
func (e *Error) WithDetails(details map[string]any) *Error {
	cp := *e
	cp.Details = details
	return &cp
}

var (
	ErrNotFound           = New("NOT_FOUND", "resource not found", http.StatusNotFound)
	ErrValidation         = New("VALIDATION_FAILED", "request failed validation", http.StatusBadRequest)
	ErrConflict           = New("CONFLICT", "conflict", http.StatusConflict)
	ErrTrunkLimitExceeded = New("TRUNK_LIMIT_EXCEEDED", "this deployment already has the maximum number of trunks", http.StatusConflict)
	ErrUnauthorized       = New("UNAUTHORIZED", "valid credentials required", http.StatusUnauthorized)
	// ErrTrunkAtCapacity means a trunk's concurrent-call limit is already
	// reached. The SIP adapter maps this to a 503, not a 486 "Busy Here" —
	// 486 asserts a fact about a callee being busy, which would misstate a
	// capacity refusal as something about the person being called. Only
	// relevant to SIP responses (adapter/sip builds its own SIP status from
	// this, not HTTPStatus) — HTTPStatus here is for the rare case this
	// surfaces through the admin API instead.
	ErrTrunkAtCapacity = New("TRUNK_AT_CAPACITY", "trunk is at its concurrent call limit", http.StatusTooManyRequests)
	// ErrCallDecisionAlreadyMade mirrors OpenAI's decision_already_made: a
	// webhook-mode call's accept/reject endpoint was called a second time,
	// or after the call already timed out. The first decision stands.
	ErrCallDecisionAlreadyMade = New("CALL_DECISION_ALREADY_MADE", "a decision for this call was already made", http.StatusConflict)
	// ErrEmailAlreadyRegistered means a registration attempt used an email
	// usecase/auth already has a user for — email is the login identifier
	// and is globally unique.
	ErrEmailAlreadyRegistered = New("EMAIL_ALREADY_REGISTERED", "email is already registered", http.StatusConflict)
)
