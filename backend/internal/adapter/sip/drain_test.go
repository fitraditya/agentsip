package sip

import (
	"context"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
)

func newTestServer() *Server {
	return &Server{activeCalls: NewActiveCalls(), log: logging.Nop()}
}

type fakeResponder struct {
	statusCode int
	reason     string
	headers    []sip.Header
}

func (f *fakeResponder) Respond(statusCode int, reason string, _ []byte, headers ...sip.Header) error {
	f.statusCode = statusCode
	f.reason = reason
	f.headers = headers
	return nil
}

func TestRefuseNewCall_Responds503WithRetryAfter(t *testing.T) {
	s := newTestServer()
	r := &fakeResponder{}

	s.refuseNewCall(r)

	if r.statusCode != sip.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", r.statusCode, sip.StatusServiceUnavailable)
	}
	found := false
	for _, h := range r.headers {
		if h.Name() == "Retry-After" {
			found = true
			if h.Value() != "10" {
				t.Fatalf("Retry-After = %q, want %q", h.Value(), "10")
			}
		}
	}
	if !found {
		t.Fatal("expected a Retry-After header on the drain refusal")
	}
}

func TestStartDraining_IsIdempotentAndReportedByIsDraining(t *testing.T) {
	s := newTestServer()
	if s.IsDraining() {
		t.Fatal("a fresh server must not report draining")
	}

	s.StartDraining()
	s.StartDraining() // second call must not panic or change behaviour

	if !s.IsDraining() {
		t.Fatal("IsDraining must be true after StartDraining")
	}
}

func TestWaitForCalls_ReturnsZeroImmediatelyWhenNoneActive(t *testing.T) {
	s := newTestServer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if remaining := s.WaitForCalls(ctx); remaining != 0 {
		t.Fatalf("WaitForCalls with no active calls = %d, want 0", remaining)
	}
}

func TestWaitForCalls_ReturnsOnceTheLastCallIsRemoved(t *testing.T) {
	s := newTestServer()
	remove := s.activeCalls.add("call-1", &fakeByeSender{})

	done := make(chan int, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done <- s.WaitForCalls(ctx)
	}()

	time.Sleep(50 * time.Millisecond)
	remove()

	select {
	case remaining := <-done:
		if remaining != 0 {
			t.Fatalf("WaitForCalls = %d, want 0 once the call ended", remaining)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForCalls did not return after the only active call was removed")
	}
}

func TestWaitForCalls_BoundedByContext(t *testing.T) {
	s := newTestServer()
	s.activeCalls.add("call-1", &fakeByeSender{}) // never removed

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	remaining := s.WaitForCalls(ctx)
	if remaining != 1 {
		t.Fatalf("WaitForCalls after deadline = %d, want 1", remaining)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("WaitForCalls took %v, want it bounded close to the 100ms deadline", elapsed)
	}
}

func TestHangupRemaining_SendsByeToEveryActiveCall(t *testing.T) {
	s := newTestServer()
	call1 := &fakeByeSender{}
	call2 := &fakeByeSender{}
	s.activeCalls.add("call-1", call1)
	s.activeCalls.add("call-2", call2)

	cutShort := s.HangupRemaining(context.Background())

	if cutShort != 2 {
		t.Fatalf("HangupRemaining returned %d, want 2", cutShort)
	}
	if !call1.byeCalled || !call2.byeCalled {
		t.Fatal("HangupRemaining must send a BYE to every active call")
	}
}

func TestDrain_ReturnsZeroWhenNoCallsAreUp(t *testing.T) {
	s := newTestServer()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if cutShort := s.Drain(ctx); cutShort != 0 {
		t.Fatalf("Drain with no active calls returned %d, want 0", cutShort)
	}
	if !s.IsDraining() {
		t.Fatal("Drain must leave the server in draining state")
	}
}

func TestDrain_HangsUpWhateverIsLeftAtTheDeadline(t *testing.T) {
	s := newTestServer()
	call := &fakeByeSender{}
	s.activeCalls.add("call-1", call) // never removed on its own

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cutShort := s.Drain(ctx)

	if cutShort != 1 {
		t.Fatalf("Drain cut short = %d, want 1", cutShort)
	}
	if !call.byeCalled {
		t.Fatal("Drain must send a BYE to a call still up at the deadline")
	}
}
