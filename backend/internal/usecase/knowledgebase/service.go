package knowledgebase

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

// Service owns knowledge base/document metadata and the ingest/search
// pipeline. It takes no Extractor or FileStore in v1: ingestion accepts
// already-extracted plain text rather than raw uploaded files, so there is
// nothing to extract and no original bytes to retain. Adding file upload
// (PDF/DOCX extraction, object storage) later is additive — a new method and
// two new port implementations — not a change to what's here.
type Service struct {
	repo        Repository
	chunker     Chunker
	embedder    Embedder
	vectorStore VectorStore
}

func NewService(repo Repository, chunker Chunker, embedder Embedder, vectorStore VectorStore) *Service {
	return &Service{repo: repo, chunker: chunker, embedder: embedder, vectorStore: vectorStore}
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, kb *domain.KnowledgeBase) error {
	reason := ""
	switch {
	case kb.Name == "":
		reason = "name is required"
	case kb.Embedding.Provider == "":
		reason = "embedding.provider is required"
	case kb.Embedding.BaseURL == "":
		reason = "embedding.base_url is required"
	}
	if reason != "" {
		return apperror.ErrValidation.WithDetails(map[string]any{"reason": reason})
	}

	now := time.Now().UTC()
	kb.ID = uuid.New()
	kb.CreatedAt, kb.UpdatedAt = now, now
	if err := s.repo.CreateKnowledgeBase(ctx, kb); err != nil {
		return fmt.Errorf("knowledgebase: create: %w", err)
	}
	return nil
}

func (s *Service) GetKnowledgeBase(ctx context.Context, id uuid.UUID) (*domain.KnowledgeBase, error) {
	kb, err := s.repo.GetKnowledgeBase(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: get: %w", err)
	}
	return kb, nil
}

func (s *Service) ListKnowledgeBases(ctx context.Context) ([]*domain.KnowledgeBase, error) {
	kbs, err := s.repo.ListKnowledgeBases(ctx)
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: list: %w", err)
	}
	return kbs, nil
}

// ListDocuments returns a knowledge base's documents, newest first. It 404s
// if the knowledge base itself doesn't exist, rather than silently returning
// an empty list, so the admin UI can distinguish "no documents yet" from "no
// such knowledge base."
func (s *Service) ListDocuments(ctx context.Context, knowledgeBaseID uuid.UUID) ([]*domain.Document, error) {
	if _, err := s.repo.GetKnowledgeBase(ctx, knowledgeBaseID); err != nil {
		return nil, fmt.Errorf("knowledgebase: list documents: %w", err)
	}
	docs, err := s.repo.ListDocuments(ctx, knowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: list documents: %w", err)
	}
	return docs, nil
}

// IngestText chunks, embeds, and stores one document's text, synchronously.
// Synchronous is a deliberate v1 scope cut, not an oversight: it runs on the
// admin HTTP API's request path, never the SIP/RTP call-handling path, so it
// has no latency-sensitive neighbor to protect — the one thing that must stay
// off the real-time path is retrieval (Search), not ingestion. A large
// corpus is the point at which this should move to the asynq-based worker
// obrelin's adapter/worker already demonstrates the pattern for; nothing
// about this method's signature would need to change to do that, since the
// caller already treats it as a call that can be slow.
func (s *Service) IngestText(ctx context.Context, knowledgeBaseID uuid.UUID, sourceURI, text string) (*domain.Document, error) {
	kb, err := s.repo.GetKnowledgeBase(ctx, knowledgeBaseID)
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: ingest: %w", err)
	}

	now := time.Now().UTC()
	doc := &domain.Document{
		ID:              uuid.New(),
		KnowledgeBaseID: kb.ID,
		SourceURI:       sourceURI,
		Status:          domain.DocumentStatusProcessing,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.CreateDocument(ctx, doc); err != nil {
		return nil, fmt.Errorf("knowledgebase: ingest: create document: %w", err)
	}

	if err := s.chunkEmbedAndStore(ctx, kb, doc, text); err != nil {
		reason := err.Error()
		if updErr := s.repo.UpdateDocumentStatus(ctx, doc.ID, domain.DocumentStatusFailed, &reason); updErr != nil {
			return nil, fmt.Errorf("knowledgebase: ingest: %w (and failed to record failure: %v)", err, updErr)
		}
		doc.Status, doc.FailureReason = domain.DocumentStatusFailed, &reason
		return doc, fmt.Errorf("knowledgebase: ingest: %w", err)
	}

	if err := s.repo.UpdateDocumentStatus(ctx, doc.ID, domain.DocumentStatusReady, nil); err != nil {
		return nil, fmt.Errorf("knowledgebase: ingest: mark ready: %w", err)
	}
	doc.Status = domain.DocumentStatusReady
	return doc, nil
}

func (s *Service) chunkEmbedAndStore(ctx context.Context, kb *domain.KnowledgeBase, doc *domain.Document, text string) error {
	texts := s.chunker.Chunk(text)
	if len(texts) == 0 {
		return fmt.Errorf("document has no content to ingest")
	}

	vectors, err := s.embedder.Embed(ctx, kb.Embedding, texts)
	if err != nil {
		return fmt.Errorf("embed chunks: %w", err)
	}
	if len(vectors) != len(texts) {
		return fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectors), len(texts))
	}

	chunks := make([]domain.Chunk, len(texts))
	for i, t := range texts {
		chunks[i] = domain.Chunk{
			ID:              uuid.New(),
			DocumentID:      doc.ID,
			KnowledgeBaseID: kb.ID,
			Index:           i,
			Content:         t,
			Embedding:       vectors[i],
		}
	}

	if err := s.vectorStore.Upsert(ctx, kb.VectorStore, chunks); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}
	return nil
}

// Search implements usecase/conversation.Retriever — this is the one method
// of this service that runs on the live-call path, so it does exactly one
// embedding call and one vector query, nothing else.
func (s *Service) Search(ctx context.Context, knowledgeBaseIDs []uuid.UUID, query string, topK int) ([]conversation.RetrievedChunk, error) {
	if len(knowledgeBaseIDs) == 0 {
		return nil, nil
	}

	// All knowledge bases searched together must share one embedding space and
	// one backing vector store — enforced here by using the first KB's config
	// for both, since a query vector from one embedding model is meaningless
	// against another's vectors, and a single query can't span two different
	// Postgres instances. A trunk offering multiple KBs is expected to have
	// provisioned them with the same embedding provider and vector store for
	// this reason.
	kb, err := s.repo.GetKnowledgeBase(ctx, knowledgeBaseIDs[0])
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: search: %w", err)
	}

	vectors, err := s.embedder.Embed(ctx, kb.Embedding, []string{query})
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: search: embed query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("knowledgebase: search: embedder returned no vector for the query")
	}

	chunks, err := s.vectorStore.Search(ctx, kb.VectorStore, knowledgeBaseIDs, vectors[0], topK)
	if err != nil {
		return nil, fmt.Errorf("knowledgebase: search: %w", err)
	}

	out := make([]conversation.RetrievedChunk, len(chunks))
	for i, c := range chunks {
		out[i] = conversation.RetrievedChunk{Content: c.Content}
	}
	return out, nil
}

var _ conversation.Retriever = (*Service)(nil)
