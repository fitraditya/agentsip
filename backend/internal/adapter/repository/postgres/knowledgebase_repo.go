package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/pkg/crypto"
)

// KnowledgeBaseRepository implements usecase/knowledgebase.Repository against
// Postgres. It lives on the main connection pool, not the vector store's:
// KnowledgeBase/Document are plain metadata, same as Trunk, and only the
// embedding itself needs pgvector. The embedding provider's API key is
// field-encrypted, same as a trunk's provider keys.
type KnowledgeBaseRepository struct {
	pool   *pgxpool.Pool
	cipher *crypto.FieldCipher
}

func NewKnowledgeBaseRepository(pool *pgxpool.Pool, cipher *crypto.FieldCipher) *KnowledgeBaseRepository {
	return &KnowledgeBaseRepository{pool: pool, cipher: cipher}
}

func (r *KnowledgeBaseRepository) CreateKnowledgeBase(ctx context.Context, kb *domain.KnowledgeBase) error {
	encEmbeddingKey, err := r.cipher.Encrypt(kb.Embedding.APIKey)
	if err != nil {
		return fmt.Errorf("postgres: encrypt embedding api key: %w", err)
	}
	encVectorStoreDSN, err := r.cipher.Encrypt(kb.VectorStore.DSN)
	if err != nil {
		return fmt.Errorf("postgres: encrypt vector store dsn: %w", err)
	}
	const q = `
		INSERT INTO knowledge_bases (
			id, name,
			embedding_provider, embedding_base_url, embedding_api_key, embedding_model,
			vector_store_provider, vector_store_dsn,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err = r.pool.Exec(ctx, q,
		kb.ID, kb.Name,
		kb.Embedding.Provider, kb.Embedding.BaseURL, encEmbeddingKey, kb.Embedding.Model,
		kb.VectorStore.Provider, encVectorStoreDSN,
		kb.CreatedAt, kb.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres: create knowledge base: %w", err)
	}
	return nil
}

const selectKnowledgeBaseColumns = `
	SELECT id, name,
		embedding_provider, embedding_base_url, embedding_api_key, embedding_model,
		vector_store_provider, vector_store_dsn,
		created_at, updated_at
	FROM knowledge_bases`

func (r *KnowledgeBaseRepository) GetKnowledgeBase(ctx context.Context, id uuid.UUID) (*domain.KnowledgeBase, error) {
	row := r.pool.QueryRow(ctx, selectKnowledgeBaseColumns+` WHERE id = $1`, id)
	kb, err := r.scanKnowledgeBaseRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	return kb, err
}

func (r *KnowledgeBaseRepository) ListKnowledgeBases(ctx context.Context) ([]*domain.KnowledgeBase, error) {
	rows, err := r.pool.Query(ctx, selectKnowledgeBaseColumns+` ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list knowledge bases: %w", err)
	}
	defer rows.Close()

	var out []*domain.KnowledgeBase
	for rows.Next() {
		kb, err := r.scanKnowledgeBaseRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, kb)
	}
	return out, rows.Err()
}

func (r *KnowledgeBaseRepository) scanKnowledgeBaseRow(row rowScanner) (*domain.KnowledgeBase, error) {
	var kb domain.KnowledgeBase
	var encEmbeddingKey, encVectorStoreDSN string
	if err := row.Scan(
		&kb.ID, &kb.Name,
		&kb.Embedding.Provider, &kb.Embedding.BaseURL, &encEmbeddingKey, &kb.Embedding.Model,
		&kb.VectorStore.Provider, &encVectorStoreDSN,
		&kb.CreatedAt, &kb.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("postgres: scan knowledge base: %w", err)
	}

	apiKey, err := r.cipher.Decrypt(encEmbeddingKey)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt embedding api key: %w", err)
	}
	kb.Embedding.APIKey = apiKey

	dsn, err := r.cipher.Decrypt(encVectorStoreDSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: decrypt vector store dsn: %w", err)
	}
	kb.VectorStore.DSN = dsn

	return &kb, nil
}

func (r *KnowledgeBaseRepository) CreateDocument(ctx context.Context, d *domain.Document) error {
	const q = `
		INSERT INTO documents (id, knowledge_base_id, source_uri, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.pool.Exec(ctx, q, d.ID, d.KnowledgeBaseID, d.SourceURI, d.Status, d.CreatedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("postgres: create document: %w", err)
	}
	return nil
}

func (r *KnowledgeBaseRepository) UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status domain.DocumentStatus, failureReason *string) error {
	const q = `UPDATE documents SET status = $2, failure_reason = $3, updated_at = now() WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id, status, failureReason)
	if err != nil {
		return fmt.Errorf("postgres: update document status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *KnowledgeBaseRepository) ListDocuments(ctx context.Context, knowledgeBaseID uuid.UUID) ([]*domain.Document, error) {
	const q = `
		SELECT id, knowledge_base_id, source_uri, status, failure_reason, created_at, updated_at
		FROM documents WHERE knowledge_base_id = $1 ORDER BY created_at DESC`
	rows, err := r.pool.Query(ctx, q, knowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list documents: %w", err)
	}
	defer rows.Close()

	var out []*domain.Document
	for rows.Next() {
		var d domain.Document
		if err := rows.Scan(&d.ID, &d.KnowledgeBaseID, &d.SourceURI, &d.Status, &d.FailureReason, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("postgres: scan document: %w", err)
		}
		out = append(out, &d)
	}
	return out, rows.Err()
}

func (r *KnowledgeBaseRepository) GetDocument(ctx context.Context, id uuid.UUID) (*domain.Document, error) {
	const q = `
		SELECT id, knowledge_base_id, source_uri, status, failure_reason, created_at, updated_at
		FROM documents WHERE id = $1`
	var d domain.Document
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&d.ID, &d.KnowledgeBaseID, &d.SourceURI, &d.Status, &d.FailureReason, &d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperror.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: get document: %w", err)
	}
	return &d, nil
}
