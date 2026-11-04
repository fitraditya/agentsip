package domain

import (
	"time"

	"github.com/google/uuid"
)

type CallDirection string

const (
	CallDirectionInbound  CallDirection = "INBOUND"
	CallDirectionOutbound CallDirection = "OUTBOUND"
)

type CallDisposition string

const (
	DispositionInProgress CallDisposition = "IN_PROGRESS"
	DispositionAnswered   CallDisposition = "ANSWERED"
	DispositionBusy       CallDisposition = "BUSY"
	DispositionNoAnswer   CallDisposition = "NO_ANSWER"
	DispositionFailed     CallDisposition = "FAILED"
	DispositionCancelled  CallDisposition = "CANCELLED"
)

// CallRecord is written when a trunk accepts an INVITE and finalized once, at
// call end — the same one-row-per-attempt, written-twice shape obrelin's CDR
// uses, so reporting code doesn't have to track a mutable in-progress row.
type CallRecord struct {
	ID      uuid.UUID
	TrunkID uuid.UUID

	// SIPCallID is the SIP Call-ID header value. The SIP adapter only knows
	// this, not the row's primary key, so it's the join key between the two.
	SIPCallID string

	CallerNumber string
	CalleeNumber string
	Direction    CallDirection
	Disposition  CallDisposition

	StartTime  time.Time
	AnswerTime *time.Time
	EndTime    *time.Time

	DurationSeconds int
	RecordingURL    *string

	CreatedAt time.Time
	UpdatedAt time.Time
}
