package sip

import (
	"errors"
	"fmt"
	"sync"
)

// ErrNoPortsAvailable means every RTP port in the configured range is
// already assigned to a live call.
var ErrNoPortsAvailable = errors.New("sip: no rtp ports available")

// RTPPortAllocator hands out RTP ports from a bounded range
// (config.RTPPortMin/RTPPortMax) instead of letting the OS pick an ephemeral
// one per call — the same reasoning as obrelin's media.Relay: a real
// network's firewall rules want a fixed range to open, not whatever the
// kernel handed out that call.
//
// Unlike obrelin's allocator, this hands out single ports, not RTP/RTCP
// pairs — this codebase doesn't process RTCP, so there's nothing to pair
// with.
type RTPPortAllocator struct {
	mu       sync.Mutex
	minPort  int
	maxPort  int
	nextPort int
	inUse    map[int]bool
}

// NewRTPPortAllocator validates the range up front, because a bad range
// would otherwise surface as a failure to answer the first call rather than
// at boot.
func NewRTPPortAllocator(minPort, maxPort int) (*RTPPortAllocator, error) {
	if minPort <= 0 || maxPort <= 0 {
		return nil, fmt.Errorf("sip: rtp port range must be positive, got %d-%d", minPort, maxPort)
	}
	if maxPort <= minPort {
		return nil, fmt.Errorf("sip: rtp port range %d-%d is empty or inverted", minPort, maxPort)
	}

	return &RTPPortAllocator{
		minPort:  minPort,
		maxPort:  maxPort,
		nextPort: minPort,
		inUse:    make(map[int]bool),
	}, nil
}

// Allocate reserves a port. It sweeps forward from the last allocation
// rather than always restarting at minPort, so a just-released port isn't
// immediately reused — a stray late packet from the previous call would
// otherwise arrive at a socket now belonging to a different one.
func (a *RTPPortAllocator) Allocate() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	span := a.maxPort - a.minPort + 1
	for range span {
		port := a.nextPort
		a.nextPort++
		if a.nextPort > a.maxPort {
			a.nextPort = a.minPort
		}
		if !a.inUse[port] {
			a.inUse[port] = true
			return port, nil
		}
	}
	return 0, ErrNoPortsAvailable
}

// Release gives a port back. Safe to call for a port that was never
// allocated (a no-op) — the bind-failure retry path in server.go releases a
// port it just got back from Allocate without tracking whether it's the one
// that actually failed to bind.
func (a *RTPPortAllocator) Release(port int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.inUse, port)
}
