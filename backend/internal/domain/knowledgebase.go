package domain

import (
	"time"

	"github.com/google/uuid"
)

// EmbeddingConfig is the OpenAI-compatible /embeddings endpoint a knowledge
// base uses for both ingestion and query-time embedding. It lives on the
// knowledge base, not the trunk: a KB can be shared across trunks that have
// different LLM configs, and ingested vectors must all come from the same
// embedding space, so the provider can't be picked per-trunk per-query.
type EmbeddingConfig struct {
	Provider ProviderKind
	BaseURL  string
	APIKey   string
	Model    string
}

// VectorStoreConfig selects where a knowledge base's embedded chunks are
// stored. An empty Provider means "use this deployment's platform-managed
// vector store" (config.VectorStoreDSN in cmd/engine) — the common case. A
// deployment that wants their own Postgres instead sets Provider: "pgvector"
// and DSN to their own connection string, which the pgvector adapter connects
// to on demand (see adapter/vectorstore/pgvector) rather than using the
// platform's shared pool. DSN is field-encrypted at rest: it's a full
// connection string, which usually embeds a password.
type VectorStoreConfig struct {
	Provider ProviderKind
	DSN      string
}

// KnowledgeBase does not belong to a single trunk, so it can be shared across
// more than one trunk (see Trunk.KnowledgeBaseIDs).
type KnowledgeBase struct {
	ID          uuid.UUID
	Name        string
	Embedding   EmbeddingConfig
	VectorStore VectorStoreConfig
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type DocumentStatus string

const (
	DocumentStatusPending    DocumentStatus = "PENDING"
	DocumentStatusProcessing DocumentStatus = "PROCESSING"
	DocumentStatusReady      DocumentStatus = "READY"
	DocumentStatusFailed     DocumentStatus = "FAILED"
)

// Document is one ingested source file. Status tracks the async
// extract->chunk->embed pipeline run by a worker job.
type Document struct {
	ID              uuid.UUID
	KnowledgeBaseID uuid.UUID
	SourceURI       string
	Status          DocumentStatus
	FailureReason   *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Chunk is one embedded slice of a Document's extracted text.
// KnowledgeBaseID is denormalized onto the chunk (not joined through
// Document) so retrieval can filter directly on it — every retrieval query
// filters on it as a required argument, never optional.
type Chunk struct {
	ID              uuid.UUID
	DocumentID      uuid.UUID
	KnowledgeBaseID uuid.UUID
	Index           int
	Content         string
	Embedding       []float32
}
