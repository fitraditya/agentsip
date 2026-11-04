package sip

import (
	"context"
	"sync"
)

// byeSender is the one thing a graceful drain needs of a dialog to end it —
// narrow so a test can exercise draining without a real SIP stack.
// *sipgo.DialogServerSession satisfies it.
type byeSender interface {
	Bye(ctx context.Context) error
}

type activeCall struct {
	sipCallID string
	dlg       byeSender
}

// ActiveCalls tracks every call currently established (200 OK sent, not yet
// ended) so that a graceful shutdown can count how many are still up and, if
// the drain deadline passes, hang up whatever is left with a BYE rather than
// closing the transport under them.
type ActiveCalls struct {
	mu    sync.Mutex
	calls map[string]*activeCall
}

func NewActiveCalls() *ActiveCalls {
	return &ActiveCalls{calls: map[string]*activeCall{}}
}

// add registers a call as active and returns a func to remove it. The
// returned func is idempotent-safe to call more than once — callers don't
// need their own sync.Once, deleting an already-absent key is a no-op.
func (a *ActiveCalls) add(sipCallID string, dlg byeSender) func() {
	a.mu.Lock()
	a.calls[sipCallID] = &activeCall{sipCallID: sipCallID, dlg: dlg}
	a.mu.Unlock()

	return func() {
		a.mu.Lock()
		delete(a.calls, sipCallID)
		a.mu.Unlock()
	}
}

// Count reports how many calls are currently active.
func (a *ActiveCalls) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.calls)
}

// snapshot copies out every call currently active, so HangupRemaining can
// send a BYE on each without holding the lock while doing network I/O.
func (a *ActiveCalls) snapshot() []*activeCall {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]*activeCall, 0, len(a.calls))
	for _, c := range a.calls {
		out = append(out, c)
	}
	return out
}
