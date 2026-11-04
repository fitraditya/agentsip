package domain

import (
	"time"

	"github.com/google/uuid"
)

type TurnRole string

const (
	TurnRoleUser      TurnRole = "USER"
	TurnRoleAssistant TurnRole = "ASSISTANT"
)

// ConversationTurn is one utterance of a call's transcript, persisted for
// transcript review and QA. LatencyMs is the time from end-of-utterance (STT
// final transcript) to first audio byte out, for assistant turns — the number
// a latency dashboard is judged on.
type ConversationTurn struct {
	ID     uuid.UUID
	CallID uuid.UUID

	Role TurnRole
	Text string

	StartedAt time.Time
	LatencyMs *int
}
