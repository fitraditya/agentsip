package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fitraditya/agentsip/internal/domain"
)

// SessionRepository implements usecase/auth.SessionRepository against
// Postgres. Only TokenHash is ever written — the plaintext token exists
// solely in the login/register response and is never persisted.
type SessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *domain.Session) error {
	const q = `
		INSERT INTO sessions (id, user_id, token_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q, s.ID, s.UserID, s.TokenHash, s.CreatedAt, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

// GetByHash returns (nil, nil) when no session matches — not found is not an
// error here, it's an expected authentication outcome.
func (r *SessionRepository) GetByHash(ctx context.Context, hash string) (*domain.Session, error) {
	const q = `
		SELECT id, user_id, token_hash, created_at, expires_at, revoked_at
		FROM sessions WHERE token_hash = $1`
	row := r.pool.QueryRow(ctx, q, hash)

	var s domain.Session
	err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.CreatedAt, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan session: %w", err)
	}
	return &s, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, hash string) (bool, error) {
	const q = `UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, hash)
	if err != nil {
		return false, fmt.Errorf("postgres: revoke session: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID) error {
	const q = `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`
	if _, err := r.pool.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("postgres: revoke all sessions for user: %w", err)
	}
	return nil
}
