package ttsprovider

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Create(ctx context.Context, p *domain.TTSProvider) error {
	if err := validate(p); err != nil {
		return err
	}

	now := time.Now().UTC()
	p.ID = uuid.New()
	p.CreatedAt, p.UpdatedAt = now, now

	if err := s.repo.Create(ctx, p); err != nil {
		return fmt.Errorf("ttsprovider: create: %w", err)
	}
	return nil
}

func (s *service) Get(ctx context.Context, id uuid.UUID) (*domain.TTSProvider, error) {
	p, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("ttsprovider: get: %w", err)
	}
	return p, nil
}

func (s *service) List(ctx context.Context) ([]*domain.TTSProvider, error) {
	providers, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("ttsprovider: list: %w", err)
	}
	return providers, nil
}

func (s *service) Update(ctx context.Context, p *domain.TTSProvider) error {
	if err := validate(p); err != nil {
		return err
	}
	if _, err := s.repo.Get(ctx, p.ID); err != nil {
		return fmt.Errorf("ttsprovider: update: %w", err)
	}
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.Update(ctx, p); err != nil {
		return fmt.Errorf("ttsprovider: update: %w", err)
	}
	return nil
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("ttsprovider: delete: %w", err)
	}
	return nil
}

func validate(p *domain.TTSProvider) error {
	reason := ""
	switch {
	case p.Name == "":
		reason = "name is required"
	case p.Provider == "":
		reason = "provider is required"
	default:
		return nil
	}
	return apperror.ErrValidation.WithDetails(map[string]any{"reason": reason})
}
