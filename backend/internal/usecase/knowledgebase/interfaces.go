// Package knowledgebase will own RAG ingestion and retrieval. Only the ports
// are defined here so far — the service depends on the extraction/embedding/
// vector-store adapters, which are the next build slice.
package knowledgebase

import (
	"context"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
)

// Extractor turns a source file's raw bytes into plain text. Keyed by media
// type so a worker can dispatch to the right implementation (txt/md need no
// extraction; pdf/docx do).
type Extractor interface {
	Extract(ctx context.Context, mediaType string, raw []byte) (text string, err error)
}

// Chunker splits extracted text into overlapping slices sized for embedding.
type Chunker interface {
	Chunk(text string) []string
}

// Embedder turns text into a vector. The same port is used for a document's
// chunks at ingestion time and for a query at retrieval time, so both paths
// are guaranteed to produce vectors in the same space. cfg comes from the
// knowledge base being embedded for — not baked into the adapter at
// construction — since different knowledge bases can use different
// embedding providers/keys through one shared adapter instance, the same
// pattern usecase/conversation uses for STT/LLM/TTS.
type Embedder interface {
	Embed(ctx context.Context, cfg domain.EmbeddingConfig, texts []string) ([][]float32, error)
}

// VectorStore persists and searches embedded chunks. Search requires
// knowledgeBaseIDs so a query cannot accidentally span every knowledge base
// in the store. cfg selects which backing store a call lands on — the
// platform's shared one, or a deployment's own (see
// domain.VectorStoreConfig) — the same per-call-config pattern as Embedder,
// so one adapter instance serves every knowledge base regardless of where
// its data actually lives.
type VectorStore interface {
	Upsert(ctx context.Context, cfg domain.VectorStoreConfig, chunks []domain.Chunk) error
	DeleteByDocument(ctx context.Context, cfg domain.VectorStoreConfig, documentID uuid.UUID) error
	Search(ctx context.Context, cfg domain.VectorStoreConfig, knowledgeBaseIDs []uuid.UUID, queryVector []float32, topK int) ([]domain.Chunk, error)
}

// FileStore persists the original uploaded document bytes (object storage),
// independent of the extracted/chunked representation in VectorStore.
type FileStore interface {
	Put(ctx context.Context, key string, data []byte) (uri string, err error)
	Get(ctx context.Context, uri string) ([]byte, error)
}

// Repository persists KnowledgeBase and Document metadata (not chunk
// content/vectors — that's VectorStore).
type Repository interface {
	CreateKnowledgeBase(ctx context.Context, kb *domain.KnowledgeBase) error
	GetKnowledgeBase(ctx context.Context, id uuid.UUID) (*domain.KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context) ([]*domain.KnowledgeBase, error)

	CreateDocument(ctx context.Context, d *domain.Document) error
	UpdateDocumentStatus(ctx context.Context, id uuid.UUID, status domain.DocumentStatus, failureReason *string) error
	GetDocument(ctx context.Context, id uuid.UUID) (*domain.Document, error)
	ListDocuments(ctx context.Context, knowledgeBaseID uuid.UUID) ([]*domain.Document, error)
}
