package sip

import (
	"sync"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

// Admission enforces each trunk's MaxConcurrentCalls at the moment a call is
// admitted — counted at INVITE, before any media port or provider session is
// allocated, same reasoning as obrelin's Decision 48: a ringing call has
// already cost us an RTP socket (and, here, is about to cost us an STT
// session), so counting only answered calls would let a trunk hold unlimited
// resources with calls that never connect.
//
// In-memory, not persisted: one process owns these calls, so it sees every
// one there is, and a restart correctly resets the count along with the
// calls themselves. There is deliberately no deployment-wide or rate-based
// limit yet (obrelin's quota usecase has one, backed by Redis so it
// survives a restart) — that needs shared state this deployment doesn't
// have wired up, and a per-trunk cap is enough to stop one trunk from
// starving the process.
type Admission struct {
	mu    sync.Mutex
	calls map[uuid.UUID]int // keyed by trunk ID
}

func NewAdmission() *Admission {
	return &Admission{calls: make(map[uuid.UUID]int)}
}

// release gives back the slot a call held. Safe to call more than once: the
// call path has more than one teardown trigger (BYE, a transport failure),
// and both must be free to call it without coordinating who goes first.
type release func()

// Admit reserves a slot for a call on this trunk, or refuses it with
// apperror.ErrTrunkAtCapacity if maxConcurrent is already reached. maxConcurrent
// <= 0 means unlimited — not expected in practice since
// usecase/trunk.Service defaults it, but an existing trunk predating that
// default, or a direct repository write, shouldn't be refused calls over it.
func (a *Admission) Admit(trunkID uuid.UUID, maxConcurrent int) (release, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if maxConcurrent > 0 && a.calls[trunkID] >= maxConcurrent {
		return nil, apperror.ErrTrunkAtCapacity
	}
	a.calls[trunkID]++

	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			a.calls[trunkID]--
			if a.calls[trunkID] <= 0 {
				delete(a.calls, trunkID) // don't let the map grow with zero-count trunks forever
			}
		})
	}, nil
}
