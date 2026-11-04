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

// TTSProviderRepository implements usecase/ttsprovider.Repository against
// Postgres. Secrets (the provider API key) are field-encrypted, same as a
// trunk's own secrets used to be before STT/LLM/TTS became standalone,
// reusable resources.
type TTSProviderRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.FieldCipher
}

func NewTTSProviderRepository(pool *pgxpool.Pool, cipher *crypto.FieldCipher) *TTSProviderRepository {
	return &TTSProviderRepository{pool: pool, cipher: cipher}
}

func (r *TTSProviderRepository) Create(ctx context.Context, p *domain.TTSProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt tts provider api key: %w", err)
	}
	const q = `
		INSERT INTO tts_providers (id, name, provider, api_key, voice_id, model, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, enc, p.VoiceID, p.Model, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create tts provider: %w", err)
	}
	return nil
}

const selectTTSProviderColumns = `SELECT id, name, provider, api_key, voice_id, model, created_at, updated_at FROM tts_providers`

func (r *TTSProviderRepository) Get(ctx context.Context, id uuid.UUID) (*domain.TTSProvider, error) {
	row := r.pool.QueryRow(ctx, selectTTSProviderColumns+` WHERE id = $1`, id)
	p, err := r.scanRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return p, err
}

func (r *TTSProviderRepository) List(ctx context.Context) ([]*domain.TTSProvider, error) {
	rows, err := r.pool.Query(ctx, selectTTSProviderColumns+` ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list tts providers: %w", err)
	}
	defer rows.Close()

	var out []*domain.TTSProvider
	for rows.Next() {
		p, err := r.scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *TTSProviderRepository) Update(ctx context.Context, p *domain.TTSProvider) error {
	enc, err := r.cipher.Encrypt(p.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt tts provider api key: %w", err)
	}
	const q = `UPDATE tts_providers SET name = $2, provider = $3, api_key = $4, voice_id = $5, model = $6, updated_at = $7 WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, p.ID, p.Name, p.Provider, enc, p.VoiceID, p.Model, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: update tts provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *TTSProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM tts_providers WHERE id = $1`, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return apperror.ErrConflict.WithDetails(map[string]any{
				"reason": "this TTS provider is still in use by one or more trunks",
			})
		}
		return fmt.Errorf("postgres: delete tts provider: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *TTSProviderRepository) scanRow(row rowScanner) (*domain.TTSProvider, error) {
	var p domain.TTSProvider
	var encAPIKey string
	if err := row.Scan(&p.ID, &p.Name, &p.Provider, &encAPIKey, &p.VoiceID, &p.Model, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("postgres: scan tts provider: %w", err)
	}
	apiKey, err := r.cipher.Decrypt(encAPIKey)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt tts provider api key: %w", err)
	}
	p.APIKey = apiKey
	return &p, nil
}
