package knowledgebase_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/usecase/knowledgebase"
)

type fakeRepo struct {
	mu   sync.Mutex
	kbs  map[uuid.UUID]*domain.KnowledgeBase
	docs map[uuid.UUID]*domain.Document
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{kbs: map[uuid.UUID]*domain.KnowledgeBase{}, docs: map[uuid.UUID]*domain.Document{}}
}

func (r *fakeRepo) CreateKnowledgeBase(_ context.Context, kb *domain.KnowledgeBase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *kb
	r.kbs[kb.ID] = &cp
	return nil
}
func (r *fakeRepo) GetKnowledgeBase(_ context.Context, id uuid.UUID) (*domain.KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kb, ok := r.kbs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return kb, nil
}
func (r *fakeRepo) ListKnowledgeBases(context.Context) ([]*domain.KnowledgeBase, error) {
	return nil, nil
}
func (r *fakeRepo) CreateDocument(_ context.Context, d *domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *d
	r.docs[d.ID] = &cp
	return nil
}
func (r *fakeRepo) UpdateDocumentStatus(_ context.Context, id uuid.UUID, status domain.DocumentStatus, failureReason *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	d.Status = status
	d.FailureReason = failureReason
	return nil
}
func (r *fakeRepo) GetDocument(_ context.Context, id uuid.UUID) (*domain.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.docs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return d, nil
}
func (r *fakeRepo) ListDocuments(_ context.Context, knowledgeBaseID uuid.UUID) ([]*domain.Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.Document
	for _, d := range r.docs {
		if d.KnowledgeBaseID == knowledgeBaseID {
			out = append(out, d)
		}
	}
	return out, nil
}

type fakeEmbedder struct {
	gotConfig domain.EmbeddingConfig
	failWith  error
}

func (e *fakeEmbedder) Embed(_ context.Context, cfg domain.EmbeddingConfig, texts []string) ([][]float32, error) {
	e.gotConfig = cfg
	if e.failWith != nil {
		return nil, e.failWith
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i)}
	}
	return out, nil
}

type fakeVectorStore struct {
	mu       sync.Mutex
	upserted []domain.Chunk
}

func (v *fakeVectorStore) Upsert(_ context.Context, _ domain.VectorStoreConfig, chunks []domain.Chunk) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.upserted = append(v.upserted, chunks...)
	return nil
}
func (v *fakeVectorStore) DeleteByDocument(context.Context, domain.VectorStoreConfig, uuid.UUID) error {
	return nil
}
func (v *fakeVectorStore) Search(_ context.Context, _ domain.VectorStoreConfig, kbIDs []uuid.UUID, _ []float32, topK int) ([]domain.Chunk, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	var out []domain.Chunk
	for _, c := range v.upserted {
		for _, id := range kbIDs {
			if c.KnowledgeBaseID == id {
				out = append(out, c)
			}
		}
	}
	if topK > 0 && len(out) > topK {
		out = out[:topK]
	}
	return out, nil
}

func TestIngestText_ChunksEmbedsAndStores(t *testing.T) {
	repo := newFakeRepo()
	embedder := &fakeEmbedder{}
	store := &fakeVectorStore{}
	svc := knowledgebase.NewService(repo, &knowledgebase.SimpleChunker{MaxChars: 20, OverlapChars: 0}, embedder, store)

	kb := &domain.KnowledgeBase{
		Name:      "docs",
		Embedding: domain.EmbeddingConfig{Provider: domain.ProviderOpenAICompat, BaseURL: "http://embed.local", APIKey: "k", Model: "m"},
	}
	if err := svc.CreateKnowledgeBase(t.Context(), kb); err != nil {
		t.Fatalf("CreateKnowledgeBase: %v", err)
	}

	doc, err := svc.IngestText(t.Context(), kb.ID, "doc.txt", "alpha beta gamma delta epsilon zeta eta theta")
	if err != nil {
		t.Fatalf("IngestText: %v", err)
	}
	if doc.Status != domain.DocumentStatusReady {
		t.Fatalf("document status = %s, want READY", doc.Status)
	}
	if embedder.gotConfig.BaseURL != kb.Embedding.BaseURL {
		t.Fatalf("embedder got base url %q, want %q", embedder.gotConfig.BaseURL, kb.Embedding.BaseURL)
	}
	if len(store.upserted) == 0 {
		t.Fatal("expected chunks to be upserted into the vector store")
	}
	for _, c := range store.upserted {
		if c.KnowledgeBaseID != kb.ID || c.DocumentID != doc.ID {
			t.Fatalf("chunk has wrong scoping: %+v", c)
		}
	}
}

func TestIngestText_EmbedderFailureMarksDocumentFailed(t *testing.T) {
	repo := newFakeRepo()
	embedder := &fakeEmbedder{failWith: fmt.Errorf("provider rejected the request")}
	store := &fakeVectorStore{}
	svc := knowledgebase.NewService(repo, knowledgebase.NewSimpleChunker(), embedder, store)

	kb := &domain.KnowledgeBase{Name: "docs", Embedding: domain.EmbeddingConfig{Provider: domain.ProviderOpenAICompat, BaseURL: "http://embed.local"}}
	if err := svc.CreateKnowledgeBase(t.Context(), kb); err != nil {
		t.Fatalf("CreateKnowledgeBase: %v", err)
	}

	doc, err := svc.IngestText(t.Context(), kb.ID, "doc.txt", "some content")
	if err == nil {
		t.Fatal("expected IngestText to return an error when the embedder fails")
	}
	if doc == nil || doc.Status != domain.DocumentStatusFailed {
		t.Fatalf("expected a FAILED document, got %+v", doc)
	}
	if len(store.upserted) != 0 {
		t.Fatalf("expected nothing upserted on failure, got %d chunks", len(store.upserted))
	}
}

func TestSearch_UsesFirstKnowledgeBasesEmbeddingConfig(t *testing.T) {
	repo := newFakeRepo()
	embedder := &fakeEmbedder{}
	store := &fakeVectorStore{}
	svc := knowledgebase.NewService(repo, knowledgebase.NewSimpleChunker(), embedder, store)

	kb := &domain.KnowledgeBase{Name: "docs", Embedding: domain.EmbeddingConfig{Provider: domain.ProviderOpenAICompat, BaseURL: "http://embed.local", Model: "m"}}
	if err := svc.CreateKnowledgeBase(t.Context(), kb); err != nil {
		t.Fatalf("CreateKnowledgeBase: %v", err)
	}
	if _, err := svc.IngestText(t.Context(), kb.ID, "doc.txt", "some content about refunds"); err != nil {
		t.Fatalf("IngestText: %v", err)
	}

	results, err := svc.Search(t.Context(), []uuid.UUID{kb.ID}, "refund policy", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one search result")
	}
	if embedder.gotConfig.BaseURL != kb.Embedding.BaseURL {
		t.Fatalf("search embedded the query with base url %q, want %q", embedder.gotConfig.BaseURL, kb.Embedding.BaseURL)
	}
}

func TestSearch_NoKnowledgeBasesReturnsEmpty(t *testing.T) {
	repo := newFakeRepo()
	svc := knowledgebase.NewService(repo, knowledgebase.NewSimpleChunker(), &fakeEmbedder{}, &fakeVectorStore{})

	results, err := svc.Search(t.Context(), nil, "anything", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %v", results)
	}
}
