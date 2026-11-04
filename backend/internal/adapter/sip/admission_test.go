package sip_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	adaptersip "github.com/fitraditya/agentsip/internal/adapter/sip"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

func TestAdmission_RefusesAtCapacity(t *testing.T) {
	a := adaptersip.NewAdmission()
	trunkID := uuid.New()

	release1, err := a.Admit(trunkID, 2)
	if err != nil {
		t.Fatalf("Admit 1/2: %v", err)
	}
	_, err = a.Admit(trunkID, 2)
	if err != nil {
		t.Fatalf("Admit 2/2: %v", err)
	}

	if _, err := a.Admit(trunkID, 2); !errors.Is(err, apperror.ErrTrunkAtCapacity) {
		t.Fatalf("Admit 3/2 = %v, want ErrTrunkAtCapacity", err)
	}

	release1() // give back one of the two held slots

	if _, err := a.Admit(trunkID, 2); err != nil {
		t.Fatalf("Admit after release: %v", err)
	}
}

func TestAdmission_ReleaseIsIdempotent(t *testing.T) {
	a := adaptersip.NewAdmission()
	trunkID := uuid.New()

	release, err := a.Admit(trunkID, 1)
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	release()
	release() // must not double-decrement below zero or panic

	// A fresh Admit must succeed exactly as if only one release ever happened.
	if _, err := a.Admit(trunkID, 1); err != nil {
		t.Fatalf("Admit after double release: %v", err)
	}
	if _, err := a.Admit(trunkID, 1); !errors.Is(err, apperror.ErrTrunkAtCapacity) {
		t.Fatalf("expected capacity to still be 1, got err=%v", err)
	}
}

func TestAdmission_ZeroMeansUnlimited(t *testing.T) {
	a := adaptersip.NewAdmission()
	trunkID := uuid.New()

	for i := range 1000 {
		if _, err := a.Admit(trunkID, 0); err != nil {
			t.Fatalf("Admit #%d with limit 0: %v", i, err)
		}
	}
}

func TestAdmission_TrunksAreIndependent(t *testing.T) {
	a := adaptersip.NewAdmission()
	trunkA, trunkB := uuid.New(), uuid.New()

	if _, err := a.Admit(trunkA, 1); err != nil {
		t.Fatalf("Admit trunkA: %v", err)
	}
	if _, err := a.Admit(trunkA, 1); !errors.Is(err, apperror.ErrTrunkAtCapacity) {
		t.Fatalf("expected trunkA to be at capacity, got %v", err)
	}
	if _, err := a.Admit(trunkB, 1); err != nil {
		t.Fatalf("trunkB should be unaffected by trunkA's limit: %v", err)
	}
}

func TestAdmission_ConcurrentAdmitNeverExceedsLimit(t *testing.T) {
	a := adaptersip.NewAdmission()
	trunkID := uuid.New()
	const limit = 10
	const attempts = 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0

	for range attempts {
		wg.Go(func() {
			if _, err := a.Admit(trunkID, limit); err == nil {
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if admitted != limit {
		t.Fatalf("admitted %d calls concurrently, want exactly %d", admitted, limit)
	}
}
