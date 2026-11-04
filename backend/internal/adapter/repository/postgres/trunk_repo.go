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
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
)

// TrunkRepository implements usecase/trunk.Repository against Postgres.
// SIPAuth's digest password is field-encrypted here; STT/LLM/TTS secrets
// live on the standalone stt_providers/llm_providers/tts_providers tables
// instead (see {STT,LLM,TTS}ProviderRepository) — a trunk only stores which
// provider row it references, joined in at read time (see selectTrunkQuery)
// so the SIP hot path (FindByInboundMatch) still gets a fully resolved
// domain.Trunk in one query, with no extra round-trips per call.
type TrunkRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.FieldCipher
}

func NewTrunkRepository(pool *pgxpool.Pool, cipher *crypto.FieldCipher) *TrunkRepository {
	return &TrunkRepository{pool: pool, cipher: cipher}
}

func (r *TrunkRepository) Create(ctx context.Context, t *domain.Trunk) error {
	encDigestPassword, err := r.cipher.Encrypt(t.SIPAuth.DigestPassword)
	if err != nil {
		return fmt.Errorf("postgres: encrypt digest password: %w", err)
	}
	const q = `
		INSERT INTO trunks (
			id, name,
			sip_allowed_ips, sip_digest_username, sip_digest_password,
			stt_provider_id, llm_provider_id, tts_provider_id,
			system_prompt, knowledge_base_ids, max_concurrent_calls,
			answer_mode, ring_delay_seconds, webhook_url, webhook_timeout_seconds,
			created_at, updated_at
		) VALUES (
			$1, $2,
			$3, $4, $5,
			$6, $7, $8,
			$9, $10, $11,
			$12, $13, $14, $15,
			$16, $17
		)`
	_, err = r.pool.Exec(ctx, q,
		t.ID, t.Name,
		t.SIPAuth.AllowedIPs, t.SIPAuth.DigestUsername, encDigestPassword,
		t.STTProviderID, t.LLMProviderID, t.TTSProviderID,
		t.SystemPrompt, t.KnowledgeBaseIDs, t.MaxConcurrentCalls,
		t.AnswerMode, t.RingDelaySeconds, t.WebhookURL, t.WebhookTimeoutSeconds,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		if mapped, ok := mapProviderFKViolation(err); ok {
			return mapped
		}
		return fmt.Errorf("postgres: create trunk: %w", err)
	}
	return nil
}

func (r *TrunkRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Trunk, error) {
	const q = selectTrunkColumns + ` WHERE t.id = $1`
	row := r.pool.QueryRow(ctx, q, id)
	return r.scanTrunk(row)
}

func (r *TrunkRepository) List(ctx context.Context) ([]*domain.Trunk, error) {
	const q = selectTrunkColumns + ` ORDER BY t.created_at`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("postgres: list trunks: %w", err)
	}
	defer rows.Close()

	var out []*domain.Trunk
	for rows.Next() {
		t, err := r.scanTrunkRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *TrunkRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM trunks`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("postgres: count trunks: %w", err)
	}
	return count, nil
}

func (r *TrunkRepository) Update(ctx context.Context, t *domain.Trunk) error {
	encDigestPassword, err := r.cipher.Encrypt(t.SIPAuth.DigestPassword)
	if err != nil {
		return fmt.Errorf("postgres: encrypt digest password: %w", err)
	}
	const q = `
		UPDATE trunks SET
			name = $2,
			sip_allowed_ips = $3, sip_digest_username = $4, sip_digest_password = $5,
			stt_provider_id = $6, llm_provider_id = $7, tts_provider_id = $8,
			system_prompt = $9, knowledge_base_ids = $10, max_concurrent_calls = $11,
			answer_mode = $12, ring_delay_seconds = $13, webhook_url = $14, webhook_timeout_seconds = $15,
			updated_at = $16
		WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q,
		t.ID, t.Name,
		t.SIPAuth.AllowedIPs, t.SIPAuth.DigestUsername, encDigestPassword,
		t.STTProviderID, t.LLMProviderID, t.TTSProviderID,
		t.SystemPrompt, t.KnowledgeBaseIDs, t.MaxConcurrentCalls,
		t.AnswerMode, t.RingDelaySeconds, t.WebhookURL, t.WebhookTimeoutSeconds,
		t.UpdatedAt,
	)
	if err != nil {
		if mapped, ok := mapProviderFKViolation(err); ok {
			return mapped
		}
		return fmt.Errorf("postgres: update trunk: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *TrunkRepository) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM trunks WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete trunk: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *TrunkRepository) FindByInboundMatch(ctx context.Context, match trunk.InboundMatch) (*domain.Trunk, error) {
	const q = selectTrunkColumns + `
		WHERE ($1::text = '' OR $1::text = ANY(t.sip_allowed_ips))
		   OR ($2::text <> '' AND t.sip_digest_username = $2)
		LIMIT 1`
	row := r.pool.QueryRow(ctx, q, match.SourceIP, match.DigestUsername)
	return r.scanTrunk(row)
}

// selectTrunkColumns joins in the trunk's referenced STT/LLM/TTS provider
// rows so a fully resolved domain.Trunk (with usable STT/LLM/TTS config,
// not just provider IDs) comes back in this one query — see this file's
// package doc comment for why that matters on the SIP hot path. An INNER
// JOIN is safe here: stt_provider_id/llm_provider_id/tts_provider_id are
// NOT NULL with ON DELETE RESTRICT, so a trunk can never reference a
// provider row that doesn't exist.
const selectTrunkColumns = `
	SELECT
		t.id, t.name,
		t.sip_allowed_ips, t.sip_digest_username, t.sip_digest_password,
		t.stt_provider_id, sp.provider, sp.api_key, sp.model,
		t.llm_provider_id, lp.provider, lp.base_url, lp.api_key, lp.model,
		t.tts_provider_id, tp.provider, tp.api_key, tp.voice_id, tp.model,
		t.system_prompt, t.knowledge_base_ids, t.max_concurrent_calls,
		t.answer_mode, t.ring_delay_seconds, t.webhook_url, t.webhook_timeout_seconds,
		t.created_at, t.updated_at
	FROM trunks t
	JOIN stt_providers sp ON sp.id = t.stt_provider_id
	JOIN llm_providers lp ON lp.id = t.llm_provider_id
	JOIN tts_providers tp ON tp.id = t.tts_provider_id`

// rowScanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query),
// letting scanTrunk/scanTrunkRow (and every other repo's equivalent
// scanRow) share one Scan call.
type rowScanner interface {
	Scan(dest ...any) error
}

func (r *TrunkRepository) scanTrunk(row pgx.Row) (*domain.Trunk, error) {
	t, err := r.scanTrunkRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return t, err
}

func (r *TrunkRepository) scanTrunkRow(row rowScanner) (*domain.Trunk, error) {
	var t domain.Trunk
	var encDigestPassword, encSTTKey, encLLMKey, encTTSKey string

	err := row.Scan(
		&t.ID, &t.Name,
		&t.SIPAuth.AllowedIPs, &t.SIPAuth.DigestUsername, &encDigestPassword,
		&t.STTProviderID, &t.STT.Provider, &encSTTKey, &t.STT.Model,
		&t.LLMProviderID, &t.LLM.Provider, &t.LLM.BaseURL, &encLLMKey, &t.LLM.Model,
		&t.TTSProviderID, &t.TTS.Provider, &encTTSKey, &t.TTS.VoiceID, &t.TTS.Model,
		&t.SystemPrompt, &t.KnowledgeBaseIDs, &t.MaxConcurrentCalls,
		&t.AnswerMode, &t.RingDelaySeconds, &t.WebhookURL, &t.WebhookTimeoutSeconds,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: scan trunk: %w", err)
	}

	if t.SIPAuth.DigestPassword, err = r.cipher.Decrypt(encDigestPassword); err != nil {
		return nil, fmt.Errorf("postgres: decrypt digest password: %w", err)
	}
	if t.STT.APIKey, err = r.cipher.Decrypt(encSTTKey); err != nil {
		return nil, fmt.Errorf("postgres: decrypt stt api key: %w", err)
	}
	if t.LLM.APIKey, err = r.cipher.Decrypt(encLLMKey); err != nil {
		return nil, fmt.Errorf("postgres: decrypt llm api key: %w", err)
	}
	if t.TTS.APIKey, err = r.cipher.Decrypt(encTTSKey); err != nil {
		return nil, fmt.Errorf("postgres: decrypt tts api key: %w", err)
	}
	return &t, nil
}

// mapProviderFKViolation turns a foreign-key violation on
// stt_provider_id/llm_provider_id/tts_provider_id (23503: referenced
// provider ID does not exist) into a client-actionable apperror.ErrValidation
// instead of a leaked internal error — this is how an invalid provider ID
// gets reported, rather than a separate existence pre-check per provider
// service.
func mapProviderFKViolation(err error) (*apperror.Error, bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return nil, false
	}
	field := "provider_id"
	switch pgErr.ConstraintName {
	case "trunks_stt_provider_id_fkey":
		field = "stt_provider_id"
	case "trunks_llm_provider_id_fkey":
		field = "llm_provider_id"
	case "trunks_tts_provider_id_fkey":
		field = "tts_provider_id"
	}
	return apperror.ErrValidation.WithDetails(map[string]any{
		"reason": fmt.Sprintf("%s does not reference an existing provider", field),
	}), true
}
