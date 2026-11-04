package sip

import (
	"context"
	"strconv"
	"time"

	"github.com/emiago/sipgo/sip"
)

// Graceful drain, matching obrelin's approach (Decision 56).
//
// Without it, shutting down closes the transport and every call in progress
// dies with it — and by the engine-lock's single-instance rule there is no
// second instance to fail over to, so every deploy would hang up on whoever
// was mid-conversation. Draining separates "stop taking new work" from "stop
// working":
//
//  1. refuse new calls, so the set of calls being waited on can only shrink;
//  2. wait for the calls already up to end on their own, bounded by a
//     deadline;
//  3. hang up whatever is left when the deadline arrives, with a BYE rather
//     than silence;
//  4. only then close the transport (the caller's job, via Close).
//
// agentsip has no carrier registration step to undo (trunks are matched by
// source IP, not by registering outbound to a carrier the way obrelin's
// trunks do), so that step of obrelin's Drain has no equivalent here.
const (
	// drainPollInterval is how often WaitForCalls re-checks. Calls end on
	// their own schedule, so this only bounds how long a finished drain
	// lingers before noticing.
	drainPollInterval = 250 * time.Millisecond

	// drainRetryAfterSeconds is the Retry-After offered with the 503. Short
	// on purpose: it tells a carrier/PBX to come back soon rather than mark
	// this destination down for minutes, which is the right hint when the
	// cause is a restart.
	drainRetryAfterSeconds = 10
)

// StartDraining stops this server accepting new calls. Calls already up are
// untouched. Idempotent — a second signal during a drain is an operator
// asking again, not asking for something different.
func (s *Server) StartDraining() {
	if s.draining.CompareAndSwap(false, true) {
		s.log.Info().Int("in_progress", s.activeCalls.Count()).
			Msg("sip: draining — refusing new calls, existing ones continue")
	}
}

// IsDraining reports whether new calls are being refused.
func (s *Server) IsDraining() bool { return s.draining.Load() }

// refuseNewCall answers an INVITE that arrived after draining began. 503
// rather than 486 or 603, same reasoning as admission's capacity refusal:
// this says the *server* cannot take the call right now, which is what an
// upstream carrier/PBX knows how to fail over on.
func (s *Server) refuseNewCall(dlg responder) {
	_ = dlg.Respond(sip.StatusServiceUnavailable, "Service Unavailable — shutting down", nil,
		sip.NewHeader("Retry-After", strconv.Itoa(drainRetryAfterSeconds)))
}

// responder is the one thing refuseNewCall needs of a dialog — narrow so a
// test can refuse a call without standing up a SIP stack.
type responder interface {
	Respond(statusCode int, reason string, body []byte, headers ...sip.Header) error
}

// WaitForCalls blocks until no call is active or ctx expires, and reports how
// many were still up when it stopped waiting. Bounded by the caller's context
// on purpose — an unbounded wait would let one forgotten call hold a deploy
// indefinitely, which in practice means operators learn to skip the drain.
func (s *Server) WaitForCalls(ctx context.Context) int {
	ticker := time.NewTicker(drainPollInterval)
	defer ticker.Stop()

	for {
		remaining := s.activeCalls.Count()
		if remaining == 0 {
			return 0
		}
		select {
		case <-ctx.Done():
			return remaining
		case <-ticker.C:
		}
	}
}

// HangupRemaining ends every call still active with a BYE. Called when the
// drain deadline passes. A BYE is meaningfully better than simply closing the
// socket: the far end learns the call is over and releases its own
// resources, where silence leaves it holding a dialog until its own timers
// fire.
func (s *Server) HangupRemaining(ctx context.Context) int {
	live := s.activeCalls.snapshot()
	for _, c := range live {
		s.log.Info().Str("sip_call_id", c.sipCallID).
			Msg("sip: ending a call that outlasted the drain deadline")
		if err := c.dlg.Bye(ctx); err != nil {
			s.log.Debug().Str("sip_call_id", c.sipCallID).Err(err).
				Msg("sip: BYE failed during drain")
		}
	}
	return len(live)
}

// Drain stops accepting new calls, lets the ones in progress finish (bounded
// by ctx), and ends whatever is left when ctx expires. The transport stays up
// throughout — every step needs it — and Close is the caller's job
// afterwards.
//
// Returns the number of calls that had to be cut short, which is the number
// worth alerting on: zero means the deploy cost nobody a conversation.
func (s *Server) Drain(ctx context.Context) int {
	s.StartDraining()

	if remaining := s.WaitForCalls(ctx); remaining > 0 {
		s.log.Warn().Int("calls", remaining).
			Msg("sip: drain deadline reached with calls still up; ending them")
		// A fresh context: ctx is spent by now, and sending a BYE on a
		// cancelled one fails immediately.
		byeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.HangupRemaining(byeCtx)
	}

	s.log.Info().Msg("sip: drained with no calls cut short")
	return 0
}
