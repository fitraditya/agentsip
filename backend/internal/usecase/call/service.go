package call

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

type service struct {
	repo Repository
}

func NewService(repo Repository) Service {
	return &service{repo: repo}
}

func (s *service) Get(ctx context.Context, id uuid.UUID) (*domain.CallRecord, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("call: get: %w", err)
	}
	return c, nil
}

func (s *service) List(ctx context.Context, page Page) ([]*domain.CallRecord, error) {
	calls, err := s.repo.List(ctx, page)
	if err != nil {
		return nil, fmt.Errorf("call: list: %w", err)
	}
	return calls, nil
}

func (s *service) RecordAttempt(ctx context.Context, trunkID uuid.UUID, sipCallID, callerNumber, calleeNumber string, direction domain.CallDirection) (*domain.CallRecord, error) {
	now := time.Now().UTC()
	c := &domain.CallRecord{
		ID:           uuid.New(),
		TrunkID:      trunkID,
		SIPCallID:    sipCallID,
		CallerNumber: callerNumber,
		CalleeNumber: calleeNumber,
		Direction:    direction,
		Disposition:  domain.DispositionInProgress,
		StartTime:    now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("call: record attempt: %w", err)
	}
	return c, nil
}

func (s *service) RecordEnd(ctx context.Context, sipCallID string, disposition domain.CallDisposition, answerTime *time.Time) error {
	c, err := s.repo.GetBySIPCallID(ctx, sipCallID)
	if err != nil {
		return fmt.Errorf("call: record end: %w", err)
	}

	endTime := time.Now().UTC()
	durationSeconds := 0
	if answerTime != nil {
		durationSeconds = int(endTime.Sub(*answerTime).Seconds())
	}

	if err := s.repo.UpdateDisposition(ctx, c.ID, disposition, answerTime, &endTime, durationSeconds); err != nil {
		return fmt.Errorf("call: record end: %w", err)
	}
	return nil
}
