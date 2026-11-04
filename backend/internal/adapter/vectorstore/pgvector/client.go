// Package pgvector implements usecase/knowledgebase.VectorStore against
// Postgres' pgvector extension.
//
// Most knowledge bases use the platform's own pool (see
// config.VectorStoreDatabaseURL in cmd/engine) — its own *pgxpool.Pool, kept
// separate from the main one so a burst of knowledge-base queries can never
// exhaust the connection budget the SIP-facing trunk/call path depends on. A
// knowledge base configured with its own domain.VectorStoreConfig.DSN (a
// user bringing their own Postgres) instead gets a lazily-created,
// per-DSN pool, cached for the life of this adapter.
package pgvector

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	pgvectorpgx "github.com/pgvector/pgvector-go/pgx"

	"github.com/fitraditya/agentsip/internal/domain"
)

type VectorStore struct {
	defaultPool *pgxpool.Pool

	mu          sync.Mutex
	customPools map[string]*pgxpool.Pool // keyed by DSN
}

func New(defaultPool *pgxpool.Pool) *VectorStore {
	return &VectorStore{defaultPool: defaultPool, customPools: map[string]*pgxpool.Pool{}}
}

// Close closes every custom pool opened via poolFor. It does not
// close the platform's default pool — that one's lifecycle belongs to
// whoever constructed it (cmd/engine), same as any other shared dependency.
//
// There is no eviction of idle custom pools before this: at MVP scale (one
// pool per distinct custom DSN actually in use, for the process lifetime)
// that's not worth the complexity yet. Revisit if the number of
// bring-your-own-Postgres knowledge bases grows large enough for that to matter.
func (v *VectorStore) Close() {
	v.mu.Lock()
	defer v.mu.Unlock()
	for _, pool := range v.customPools {
		pool.Close()
	}
}

// poolFor returns the platform pool, or a knowledge base's own custom one —
// connecting to and provisioning the latter on first use. A custom Postgres
// is treated as the operator's to own, not ours to migrate by hand, so
// provisioning here is idempotent CREATE ... IF NOT EXISTS only: it never
// alters or drops anything, matching what a first-use "just works"
// experience needs without risking existing data there.
func (v *VectorStore) poolFor(ctx context.Context, cfg domain.VectorStoreConfig) (*pgxpool.Pool, error) {
	if cfg.DSN == "" {
		return v.defaultPool, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if pool, ok := v.customPools[cfg.DSN]; ok {
		return pool, nil
	}

	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse custom vector store dsn: %w", err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgvectorpgx.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect custom vector store: %w", err)
	}
	if err := ensureChunksSchema(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("provision custom vector store: %w", err)
	}

	v.customPools[cfg.DSN] = pool
	return pool, nil
}

// chunksSchemaStatements mirrors db/migrations/0002_vectorstore.sql. Kept as
// idempotent statements (not a shared file with that migration) because this
// runs against a custom database, which may already have other things
// in it — IF NOT EXISTS is the whole point, not an optimization.
var chunksSchemaStatements = []string{
	`CREATE EXTENSION IF NOT EXISTS vector`,
	`CREATE TABLE IF NOT EXISTS chunks (
		id                uuid PRIMARY KEY,
		document_id       uuid NOT NULL,
		knowledge_base_id uuid NOT NULL,
		chunk_index       int NOT NULL,
		content           text NOT NULL,
		embedding         vector(1536) NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS chunks_document_id_idx ON chunks (document_id)`,
	`CREATE INDEX IF NOT EXISTS chunks_kb_idx ON chunks (knowledge_base_id)`,
	`CREATE INDEX IF NOT EXISTS chunks_embedding_idx ON chunks USING hnsw (embedding vector_cosine_ops)`,
}

func ensureChunksSchema(ctx context.Context, pool *pgxpool.Pool) error {
	for _, stmt := range chunksSchemaStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (v *VectorStore) Upsert(ctx context.Context, cfg domain.VectorStoreConfig, chunks []domain.Chunk) error {
	if len(chunks) == 0 {
		return nil
	}
	pool, err := v.poolFor(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pgvector: upsert: %w", err)
	}

	var batch pgx.Batch
	for _, c := range chunks {
		batch.Queue(
			`INSERT INTO chunks (id, document_id, knowledge_base_id, chunk_index, content, embedding)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			c.ID, c.DocumentID, c.KnowledgeBaseID, c.Index, c.Content, pgvector.NewVector(c.Embedding),
		)
	}

	results := pool.SendBatch(ctx, &batch)
	defer results.Close()
	for range chunks {
		if _, err := results.Exec(); err != nil {
			return fmt.Errorf("pgvector: upsert: %w", err)
		}
	}
	return nil
}

func (v *VectorStore) DeleteByDocument(ctx context.Context, cfg domain.VectorStoreConfig, documentID uuid.UUID) error {
	pool, err := v.poolFor(ctx, cfg)
	if err != nil {
		return fmt.Errorf("pgvector: delete by document: %w", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM chunks WHERE document_id = $1`, documentID); err != nil {
		return fmt.Errorf("pgvector: delete by document: %w", err)
	}
	return nil
}

func (v *VectorStore) Search(ctx context.Context, cfg domain.VectorStoreConfig, knowledgeBaseIDs []uuid.UUID, queryVector []float32, topK int) ([]domain.Chunk, error) {
	pool, err := v.poolFor(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}
	if topK <= 0 {
		topK = 5
	}

	const q = `
		SELECT id, document_id, knowledge_base_id, chunk_index, content
		FROM chunks
		WHERE knowledge_base_id = ANY($1)
		ORDER BY embedding <=> $2
		LIMIT $3`
	rows, err := pool.Query(ctx, q, knowledgeBaseIDs, pgvector.NewVector(queryVector), topK)
	if err != nil {
		return nil, fmt.Errorf("pgvector: search: %w", err)
	}
	defer rows.Close()

	var out []domain.Chunk
	for rows.Next() {
		var c domain.Chunk
		if err := rows.Scan(&c.ID, &c.DocumentID, &c.KnowledgeBaseID, &c.Index, &c.Content); err != nil {
			return nil, fmt.Errorf("pgvector: scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
