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

// LLMProviderRepository implements usecase/llmprovider.Repository against
// Postgres. Secrets (the provider API key) are field-encrypted, same as a
// trunk's own secrets used to be before STT/LLM/TTS became standalone,
// reusable resources.
type LLMProviderRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.FieldCipher
}

func NewLLMProviderRepository(pool *pgxpool.Pool, cipher *crypto.FieldCipher) *LLMProviderRepository {
	return &LLMProviderRepository{pool: pool, cipher: cipher}
}

func (r *LLMProviderRepository) Create(ctx context.Context, p *domain.LLMProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt llm provider api key: %w", err)
	}
	const q = `
		INSERT INTO llm_providers (id, name, provider, base_url, api_key, model, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, p.BaseURL, enc, p.Model, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create llm provider: %w", err)
	}
	return nil
}

const selectLLMProviderColumns = `SELECT id, name, provider, base_url, api_key, model, created_at, updated_at FROM llm_providers`

func (r *LLMProviderRepository) Get(ctx context.Context, id uuid.UUID) (*domain.LLMProvider, error) {
	row := r.pool.QueryRow(ctx, selectLLMProviderColumns+` WHERE id = $1`, id)
	p, err := r.scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return p, err
}

func (r *LLMProviderRepository) List(ctx context.Context) ([]*domain.LLMProvider, error) {
	rows, err := r.pool.Query(ctx, selectLLMProviderColumns+` ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list llm providers: %w", err)
	}
	defer rows.Close()

	var out []*domain.LLMProvider
	for rows.Next() {
		p, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *LLMProviderRepository) Update(ctx context.Context, p *domain.LLMProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt llm provider api key: %w", err)
	}
	const q = `UPDATE llm_providers SET name = $2, provider = $3, base_url = $4, api_key = $5, model = $6, updated_at = $7 WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, p.BaseURL, enc, p.Model, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: update llm provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *LLMProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM llm_providers WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperror.ErrConflict.WithDetails(map[string]any{
				"reason": "this LLM provider is still in use by one or more trunks",
			})
		}
		return fmt.Errorf("postgres: delete llm provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *LLMProviderRepository) scanRow(row rowScanner) (*domain.LLMProvider, error) {
	var p domain.LLMProvider
	var encAPIKey string
	if err := row.Scan(&p.ID, &p.Name, &p.Provider, &p.BaseURL, &encAPIKey, &p.Model, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("postgres: scan llm provider: %w", err)
	}
	apiKey, err := r.cipher.Decrypt(encAPIKey)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt llm provider api key: %w", err)
	}
	p.APIKey = apiKey
	return &p, nil
}
