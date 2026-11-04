package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fitraditya/agentsip/internal/domain"
)

// EmailTokenRepository implements usecase/auth.EmailTokenRepository against
// Postgres. Backs both email verification and password reset — see
// domain.EmailToken's doc comment for why one table serves both.
type EmailTokenRepository struct {
	pool *pgxpool.Pool
}

func NewEmailTokenRepository(pool *pgxpool.Pool) *EmailTokenRepository {
	return &EmailTokenRepository{pool: pool}
}

func (r *EmailTokenRepository) Create(ctx context.Context, t *domain.EmailToken) error {
	const q = `
		INSERT INTO email_tokens (id, user_id, purpose, token_hash, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, q, t.ID, t.UserID, string(t.Purpose), t.TokenHash, t.CreatedAt, t.ExpiresAt)
	if err != nil {
		return fmt.Errorf("postgres: create email token: %w", err)
	}
	return nil
}

// GetByHash returns (nil, nil) when no token matches — not found is not an
// error here, it's an expected outcome usecase/auth.Service turns into
// apperror.ErrUnauthorized.
func (r *EmailTokenRepository) GetByHash(ctx context.Context, hash string) (*domain.EmailToken, error) {
	const q = `
		SELECT id, user_id, purpose, token_hash, created_at, expires_at, used_at
		FROM email_tokens WHERE token_hash = $1`
	row := r.pool.QueryRow(ctx, q, hash)

	var t domain.EmailToken
	var purpose string
	err := row.Scan(&t.ID, &t.UserID, &purpose, &t.TokenHash, &t.CreatedAt, &t.ExpiresAt, &t.UsedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan email token: %w", err)
	}
	t.Purpose = domain.EmailTokenPurpose(purpose)
	return &t, nil
}

func (r *EmailTokenRepository) MarkUsed(ctx context.Context, hash string) (bool, error) {
	const q = `UPDATE email_tokens SET used_at = now() WHERE token_hash = $1 AND used_at IS NULL`
	tag, err := r.pool.Exec(ctx, q, hash)
	if err != nil {
		return false, fmt.Errorf("postgres: mark email token used: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}
