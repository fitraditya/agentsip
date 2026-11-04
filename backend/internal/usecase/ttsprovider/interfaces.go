package ttsprovider

import (
	"context"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, p *domain.TTSProvider) error
	Get(ctx context.Context, id uuid.UUID) (*domain.TTSProvider, error)
	List(ctx context.Context) ([]*domain.TTSProvider, error)
	Update(ctx context.Context, p *domain.TTSProvider) error
	// Delete returns apperror.ErrValidation (not a raw foreign-key error) if
	// a trunk still references this provider — see the postgres
	// implementation's doc comment.
	Delete(ctx context.Context, id uuid.UUID) error
}

type Service interface {
	Create(ctx context.Context, p *domain.TTSProvider) error
	Get(ctx context.Context, id uuid.UUID) (*domain.TTSProvider, error)
	List(ctx context.Context) ([]*domain.TTSProvider, error)
	Update(ctx context.Context, p *domain.TTSProvider) error
	Delete(ctx context.Context, id uuid.UUID) error
}
