package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/call"
)

const defaultPageLimit = 50

// CallRepository implements usecase/call.Repository against Postgres.
type CallRepository struct {
	pool *pgxpool.Pool
}

func NewCallRepository(pool *pgxpool.Pool) *CallRepository {
	return &CallRepository{pool: pool}
}

const selectCallColumns = `
	SELECT
		id, trunk_id, sip_call_id,
		caller_number, callee_number, direction, disposition,
		start_time, answer_time, end_time,
		duration_seconds, recording_url,
		created_at, updated_at
	FROM calls`

func (r *CallRepository) Create(ctx context.Context, c *domain.CallRecord) error {
	const q = `
		INSERT INTO calls (
			id, trunk_id, sip_call_id,
			caller_number, callee_number, direction, disposition,
			start_time, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := r.pool.Exec(ctx, q,
		c.ID, c.TrunkID, c.SIPCallID,
		c.CallerNumber, c.CalleeNumber, c.Direction, c.Disposition,
		c.StartTime, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create call: %w", err)
	}
	return nil
}

func (r *CallRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.CallRecord, error) {
	row := r.pool.QueryRow(ctx, selectCallColumns+` WHERE id = $1`, id)
	return scanCall(row)
}

func (r *CallRepository) GetBySIPCallID(ctx context.Context, sipCallID string) (*domain.CallRecord, error) {
	row := r.pool.QueryRow(ctx, selectCallColumns+` WHERE sip_call_id = $1`, sipCallID)
	return scanCall(row)
}

func (r *CallRepository) List(ctx context.Context, page call.Page) ([]*domain.CallRecord, error) {
	limit := page.Limit
	if limit <= 0 {
		limit = defaultPageLimit
	}

	var rows pgx.Rows
	var err error
	if page.Before == "" {
		rows, err = r.pool.Query(ctx, selectCallColumns+`
			ORDER BY start_time DESC, id DESC
			LIMIT $1`, limit)
	} else {
		cursorTime, cursorID, perr := decodeCallCursor(page.Before)
		if perr != nil {
			return nil, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid page cursor"})
		}
		rows, err = r.pool.Query(ctx, selectCallColumns+`
			WHERE (start_time, id) < ($1, $2)
			ORDER BY start_time DESC, id DESC
			LIMIT $3`, cursorTime, cursorID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: list calls: %w", err)
	}
	defer rows.Close()

	var out []*domain.CallRecord
	for rows.Next() {
		c, err := scanCallRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CallRepository) UpdateDisposition(ctx context.Context, id uuid.UUID, disposition domain.CallDisposition, answerTime, endTime *time.Time, durationSeconds int) error {
	const q = `
		UPDATE calls SET
			disposition = $2, answer_time = $3, end_time = $4,
			duration_seconds = $5, updated_at = now()
		WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, disposition, answerTime, endTime, durationSeconds)
	if err != nil {
		return fmt.Errorf("postgres: update call disposition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func scanCall(row pgx.Row) (*domain.CallRecord, error) {
	c, err := scanCallRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return c, err
}

func scanCallRow(row rowScanner) (*domain.CallRecord, error) {
	var c domain.CallRecord
	err := row.Scan(
		&c.ID, &c.TrunkID, &c.SIPCallID,
		&c.CallerNumber, &c.CalleeNumber, &c.Direction, &c.Disposition,
		&c.StartTime, &c.AnswerTime, &c.EndTime,
		&c.DurationSeconds, &c.RecordingURL,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan call: %w", err)
	}
	return &c, nil
}
