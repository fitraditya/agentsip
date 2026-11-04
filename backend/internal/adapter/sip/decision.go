package sip

import (
	"time"

	"github.com/emiago/sipgo/sip"
)

// waitRingDelay sleeps for delay to simulate normal ring time before
// auto-answering, returning false if the caller hangs up (CANCEL) before it
// elapses — proceeding to answer a call that was already cancelled would
// otherwise still happen, since nothing else here watches for that.
func waitRingDelay(tx sip.ServerTransaction, delay time.Duration) (proceed bool) {
	if delay <= 0 {
		return true
	}

	cancelled := make(chan struct{})
	tx.OnCancel(func(*sip.Request) { close(cancelled) })

	select {
	case <-time.After(delay):
		return true
	case <-cancelled:
		return false
	}
}

// waitWebhookDecision blocks for up to timeout for an accept/reject decision
// on ch, or until the caller hangs up first. cancelled is reported
// separately from an ordinary rejection because sipgo's transaction layer
// already answers a CANCEL with 487 Request Terminated on its own — the
// caller of this function must not also send a response in that case.
func waitWebhookDecision(tx sip.ServerTransaction, ch <-chan decision, timeout time.Duration) (proceed bool, rejectStatus int, cancelled bool) {
	cancelledCh := make(chan struct{})
	tx.OnCancel(func(*sip.Request) { close(cancelledCh) })

	select {
	case d := <-ch:
		if d.accept {
			return true, 0, false
		}
		return false, d.statusCode, false
	case <-time.After(timeout):
		return false, defaultRejectStatusCode, false
	case <-cancelledCh:
		return false, 0, true
	}
}
