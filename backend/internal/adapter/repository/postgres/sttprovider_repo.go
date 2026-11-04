package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/pkg/crypto"
)

// STTProviderRepository implements usecase/sttprovider.Repository against
// Postgres. Secrets (the provider API key) are field-encrypted, same as a
// trunk's own secrets used to be before STT/LLM/TTS became standalone,
// reusable resources.
type STTProviderRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.FieldCipher
}

func NewSTTProviderRepository(pool *pgxpool.Pool, cipher *crypto.FieldCipher) *STTProviderRepository {
	return &STTProviderRepository{pool: pool, cipher: cipher}
}

func (r *STTProviderRepository) Create(ctx context.Context, p *domain.STTProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt stt provider api key: %w", err)
	}
	const q = `
		INSERT INTO stt_providers (id, name, provider, api_key, model, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err = r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, enc, p.Model, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create stt provider: %w", err)
	}
	return nil
}

const selectSTTProviderColumns = `SELECT id, name, provider, api_key, model, created_at, updated_at FROM stt_providers`

func (r *STTProviderRepository) Get(ctx context.Context, id uuid.UUID) (*domain.STTProvider, error) {
	row := r.pool.QueryRow(ctx, selectSTTProviderColumns+` WHERE id = $1`, id)
	p, err := r.scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return p, err
}

func (r *STTProviderRepository) List(ctx context.Context) ([]*domain.STTProvider, error) {
	rows, err := r.pool.Query(ctx, selectSTTProviderColumns+` ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list stt providers: %w", err)
	}
	defer rows.Close()

	var out []*domain.STTProvider
	for rows.Next() {
		p, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *STTProviderRepository) Update(ctx context.Context, p *domain.STTProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt stt provider api key: %w", err)
	}
	const q = `UPDATE stt_providers SET name = $2, provider = $3, api_key = $4, model = $5, updated_at = $6 WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, enc, p.Model, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: update stt provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *STTProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM stt_providers WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		// 23503: foreign_key_violation — a trunk still references this
		// provider (ON DELETE RESTRICT). Reported as a conflict, not an
		// internal error: the caller must reassign or delete that trunk
		// first.
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperror.ErrConflict.WithDetails(map[string]any{
				"reason": "this STT provider is still in use by one or more trunks",
			})
		}
		return fmt.Errorf("postgres: delete stt provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *STTProviderRepository) scanRow(row rowScanner) (*domain.STTProvider, error) {
	var p domain.STTProvider
	var encAPIKey string
	if err := row.Scan(&p.ID, &p.Name, &p.Provider, &encAPIKey, &p.Model, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("postgres: scan stt provider: %w", err)
	}
	apiKey, err := r.cipher.Decrypt(encAPIKey)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt stt provider api key: %w", err)
	}
	p.APIKey = apiKey
	return &p, nil
}
