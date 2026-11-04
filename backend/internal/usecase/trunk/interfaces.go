package trunk

import (
	"context"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

// Repository persists trunks. Count backs the MaxTrunks check in
// Service.Create — kept as its own method rather than len(List()) so the
// limit check doesn't require loading every trunk's full config.
type Repository interface {
	Create(ctx context.Context, t *domain.Trunk) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Trunk, error)
	List(ctx context.Context) ([]*domain.Trunk, error)
	Count(ctx context.Context) (int, error)
	Update(ctx context.Context, t *domain.Trunk) error
	Delete(ctx context.Context, id uuid.UUID) error
	// FindByInboundMatch looks up the trunk whose SIPAuth matches an incoming
	// call — this is a process-lifecycle lookup (routing an unauthenticated
	// inbound call), not a request made on behalf of any particular caller,
	// so it searches across every trunk rather than being scoped like every
	// other method here.
	FindByInboundMatch(ctx context.Context, match InboundMatch) (*domain.Trunk, error)
}

// InboundMatch is what the SIP adapter needs to resolve an incoming INVITE to
// a trunk, by source IP and/or the SIP digest username presented. Declared
// here, consumed by adapter/sip, per the same dependency-inversion rule as
// every other port in this codebase: the adapter depends on the usecase, not
// the other way around.
type InboundMatch struct {
	SourceIP       string
	DigestUsername string
}

type Service interface {
	Create(ctx context.Context, t *domain.Trunk) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Trunk, error)
	List(ctx context.Context) ([]*domain.Trunk, error)
	Update(ctx context.Context, t *domain.Trunk) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ResolveInbound finds the trunk an incoming call belongs to. Returns
	// apperror.ErrNotFound if no trunk's SIPAuth matches.
	ResolveInbound(ctx context.Context, match InboundMatch) (*domain.Trunk, error)
}
