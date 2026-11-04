package sip_test

import (
	"errors"
	"testing"

	adaptersip "github.com/fitraditya/agentsip/internal/adapter/sip"
)

func TestRTPPortAllocator_RejectsBadRanges(t *testing.T) {
	cases := []struct {
		name     string
		min, max int
	}{
		{"zero min", 0, 100},
		{"negative min", -1, 100},
		{"max equals min", 100, 100},
		{"max below min", 200, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := adaptersip.NewRTPPortAllocator(tc.min, tc.max); err == nil {
				t.Fatalf("expected an error for range %d-%d", tc.min, tc.max)
			}
		})
	}
}

func TestRTPPortAllocator_AllocatesWithinRangeAndRejectsDuplicates(t *testing.T) {
	a, err := adaptersip.NewRTPPortAllocator(20000, 20002) // 3 ports: 20000, 20001, 20002
	if err != nil {
		t.Fatalf("NewRTPPortAllocator: %v", err)
	}

	seen := map[int]bool{}
	for i := 0; i < 3; i++ {
		port, err := a.Allocate()
		if err != nil {
			t.Fatalf("Allocate #%d: %v", i, err)
		}
		if port < 20000 || port > 20002 {
			t.Fatalf("port %d out of range 20000-20002", port)
		}
		if seen[port] {
			t.Fatalf("port %d allocated twice before any release", port)
		}
		seen[port] = true
	}

	if _, err := a.Allocate(); !errors.Is(err, adaptersip.ErrNoPortsAvailable) {
		t.Fatalf("expected ErrNoPortsAvailable once the range is exhausted, got %v", err)
	}
}

func TestRTPPortAllocator_ReleaseAllowsReuse(t *testing.T) {
	a, err := adaptersip.NewRTPPortAllocator(21000, 21001) // 2 ports
	if err != nil {
		t.Fatalf("NewRTPPortAllocator: %v", err)
	}

	first, err := a.Allocate()
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if _, err := a.Allocate(); err != nil {
		t.Fatalf("Allocate second port: %v", err)
	}
	if _, err := a.Allocate(); !errors.Is(err, adaptersip.ErrNoPortsAvailable) {
		t.Fatalf("expected exhaustion, got %v", err)
	}

	a.Release(first)
	if _, err := a.Allocate(); err != nil {
		t.Fatalf("expected a released port to be allocatable again: %v", err)
	}
}

func TestRTPPortAllocator_ReleaseOfUnallocatedPortIsANoop(t *testing.T) {
	a, err := adaptersip.NewRTPPortAllocator(22000, 22001)
	if err != nil {
		t.Fatalf("NewRTPPortAllocator: %v", err)
	}
	a.Release(22000) // never allocated — must not panic or corrupt state
	if _, err := a.Allocate(); err != nil {
		t.Fatalf("Allocate: %v", err)
	}
}
