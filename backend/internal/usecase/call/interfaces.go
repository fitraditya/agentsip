package call

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

// Page is one window of a call list, cursored like obrelin's call listing: the
// cursor is (start_time, id), not an offset, since calls arrive continuously
// and an offset shifts under a reader paging through live data.
type Page struct {
	Limit  int
	Before string
}

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.CallRecord, error)
	GetBySIPCallID(ctx context.Context, sipCallID string) (*domain.CallRecord, error)
	List(ctx context.Context, page Page) ([]*domain.CallRecord, error)
	Create(ctx context.Context, c *domain.CallRecord) error
	// UpdateDisposition is the one-time terminal write: a call's disposition
	// and end time are set once, when the call ends.
	UpdateDisposition(ctx context.Context, id uuid.UUID, disposition domain.CallDisposition, answerTime, endTime *time.Time, durationSeconds int) error
}

type Service interface {
	Get(ctx context.Context, id uuid.UUID) (*domain.CallRecord, error)
	List(ctx context.Context, page Page) ([]*domain.CallRecord, error)
	// RecordAttempt is called by the SIP adapter when an INVITE is accepted
	// for a trunk, creating the call's row.
	RecordAttempt(ctx context.Context, trunkID uuid.UUID, sipCallID, callerNumber, calleeNumber string, direction domain.CallDirection) (*domain.CallRecord, error)
	// RecordEnd finalizes the row. There is no RecordAnswer: disposition/end
	// time are written once, at call end — the SIP adapter tracks answer
	// time in memory and passes it here directly.
	RecordEnd(ctx context.Context, sipCallID string, disposition domain.CallDisposition, answerTime *time.Time) error
}

// Decisions lets an external system — reached via a webhook-mode trunk's
// webhook — accept or reject a call that is currently waiting on that
// decision. Declared here and implemented by adapter/sip (the only thing
// that can act on a live SIP dialog), so adapter/http's admin API depends on
// this interface rather than importing adapter/sip directly — the same
// dependency-inversion rule as every other cross-adapter port in this
// codebase.
type Decisions interface {
	// Accept and Reject both return apperror.ErrNotFound for an unknown or
	// already-resolved pendingCallID (resolved includes "timed out"), and
	// apperror.ErrCallDecisionAlreadyMade if a decision for this call was
	// already made but the call hasn't finished tearing down yet.
	Accept(pendingCallID uuid.UUID) error
	// Reject declines the call with the given SIP status code (300-699,
	// same constraint OpenAI's reject endpoint documents). statusCode <= 0
	// means "use the mode's default" (480 Temporarily Unavailable).
	Reject(pendingCallID uuid.UUID, statusCode int) error
}
