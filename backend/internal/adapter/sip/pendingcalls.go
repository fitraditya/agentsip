package sip

import (
	"sync"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

// defaultRejectStatusCode is used both for an explicit Reject with
// statusCode <= 0 and for a decision that times out. 480 "Temporarily
// Unavailable", not 603 "Decline": nobody actively declined this call, the
// external service just didn't answer in time, which is a different fact
// than an affirmative no.
const defaultRejectStatusCode = 480

type decision struct {
	accept     bool
	statusCode int
}

type pendingCall struct {
	ch   chan decision // buffered 1; at most one value is ever sent
	once sync.Once
}

// PendingCalls tracks webhook-mode calls currently waiting on an external
// accept/reject decision. It implements usecase/call.Decisions; handleInvite
// is the other side, registering a call and selecting on the channel
// register returns against a timeout.
type PendingCalls struct {
	mu    sync.Mutex
	calls map[uuid.UUID]*pendingCall
}

func NewPendingCalls() *PendingCalls {
	return &PendingCalls{calls: make(map[uuid.UUID]*pendingCall)}
}

// register creates a new pending call and returns its ID, the channel to
// wait on, and a forget function the waiter must call exactly once its wait
// ends (accepted, rejected, or timed out) — forgetting makes any Accept/
// Reject call arriving after that point correctly fail with
// apperror.ErrNotFound instead of resolving into a channel nobody reads
// anymore.
func (p *PendingCalls) register() (id uuid.UUID, ch <-chan decision, forget func()) {
	id = uuid.New()
	pc := &pendingCall{ch: make(chan decision, 1)}

	p.mu.Lock()
	p.calls[id] = pc
	p.mu.Unlock()

	return id, pc.ch, func() {
		p.mu.Lock()
		delete(p.calls, id)
		p.mu.Unlock()
	}
}

// resolve looks up a pending call by its ID alone — the random
// pendingCallID (a UUID handed out only to the webhook receiver in
// IncomingCallWebhookPayload) is the sole capability token for this
// callback; there is no separate ownership check layered on top of it.
func (p *PendingCalls) resolve(id uuid.UUID, d decision) error {
	p.mu.Lock()
	pc, ok := p.calls[id]
	p.mu.Unlock()
	if !ok {
		return apperror.ErrNotFound
	}

	sent := false
	pc.once.Do(func() {
		sent = true
		pc.ch <- d
	})
	if !sent {
		return apperror.ErrCallDecisionAlreadyMade
	}
	return nil
}

func (p *PendingCalls) Accept(pendingCallID uuid.UUID) error {
	return p.resolve(pendingCallID, decision{accept: true})
}

func (p *PendingCalls) Reject(pendingCallID uuid.UUID, statusCode int) error {
	if statusCode <= 0 {
		statusCode = defaultRejectStatusCode
	}
	return p.resolve(pendingCallID, decision{accept: false, statusCode: statusCode})
}

// reasonPhrase gives a SIP response a standard-ish reason for the status
// codes this package actually sends. The reason phrase isn't meaningful to
// a conforming client (only the numeric code is), so an unrecognized code
// — e.g. a caller-supplied status via the accept/reject API — gets a plain
// fallback rather than a lookup table covering every code in the registry.
func reasonPhrase(statusCode int) string {
	switch statusCode {
	case 480:
		return "Temporarily Unavailable"
	case 486:
		return "Busy Here"
	case 500:
		return "Internal Server Error"
	case 503:
		return "Service Unavailable"
	case 603:
		return "Decline"
	default:
		return "Call Rejected"
	}
}
