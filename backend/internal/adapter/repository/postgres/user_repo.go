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
)

// UserRepository implements usecase/auth.UserRepository against Postgres.
type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	const q = `
		INSERT INTO users (id, email, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.pool.Exec(ctx, q, u.ID, u.Email, u.PasswordHash, u.CreatedAt, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

// GetByEmail returns (nil, nil) when no user matches — not found is not an
// error here, it's an expected authentication outcome usecase/auth.Service
// turns into apperror.ErrUnauthorized.
func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `
		SELECT id, email, password_hash, email_verified_at, created_at, updated_at
		FROM users WHERE email = $1`
	row := r.pool.QueryRow(ctx, q, email)

	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.EmailVerifiedAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan user: %w", err)
	}
	return &u, nil
}

// GetByID returns (nil, nil) when no user matches.
func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	const q = `
		SELECT id, email, password_hash, email_verified_at, created_at, updated_at
		FROM users WHERE id = $1`
	row := r.pool.QueryRow(ctx, q, id)

	var u domain.User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.EmailVerifiedAt, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: scan user: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) UpdateEmail(ctx context.Context, userID uuid.UUID, newEmail string) error {
	const q = `UPDATE users SET email = $1, updated_at = now() WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, newEmail, userID)
	if err != nil {
		return fmt.Errorf("postgres: update email: %w", err)
	}
	return nil
}

func (r *UserRepository) SetPasswordHash(ctx context.Context, userID uuid.UUID, hash string) error {
	const q = `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`
	_, err := r.pool.Exec(ctx, q, hash, userID)
	if err != nil {
		return fmt.Errorf("postgres: set password hash: %w", err)
	}
	return nil
}

func (r *UserRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID, at time.Time) error {
	const q = `UPDATE users SET email_verified_at = $1, updated_at = now() WHERE id = $2 AND email_verified_at IS NULL`
	_, err := r.pool.Exec(ctx, q, at, userID)
	if err != nil {
		return fmt.Errorf("postgres: mark email verified: %w", err)
	}
	return nil
}

// Any reports whether at least one user exists — used by auth.Service's
// Bootstrap to decide whether startup wiring still needs to create the
// first user.
func (r *UserRepository) Any(ctx context.Context) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM users)`
	var exists bool
	if err := r.pool.QueryRow(ctx, q).Scan(&exists); err != nil {
		return false, fmt.Errorf("postgres: check any user exists: %w", err)
	}
	return exists, nil
}
