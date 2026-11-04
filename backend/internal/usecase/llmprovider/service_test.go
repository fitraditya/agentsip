package llmprovider_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/llmprovider"
)

type fakeRepo struct {
	mu    sync.Mutex
	items map[uuid.UUID]*domain.LLMProvider
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[uuid.UUID]*domain.LLMProvider{}}
}

func (r *fakeRepo) Create(_ context.Context, p *domain.LLMProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Get(_ context.Context, id uuid.UUID) (*domain.LLMProvider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[id]
	if !ok {
		return nil, apperror.ErrNotFound
	}
	return p, nil
}

func (r *fakeRepo) List(_ context.Context) ([]*domain.LLMProvider, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.LLMProvider
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

func (r *fakeRepo) Update(_ context.Context, p *domain.LLMProvider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[p.ID]; !ok {
		return apperror.ErrNotFound
	}
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return apperror.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func TestCreate_ValidationFailsOnMissingFields(t *testing.T) {
	svc := llmprovider.NewService(newFakeRepo())
	err := svc.Create(t.Context(), &domain.LLMProvider{})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestCreateThenGet_RoundTrips(t *testing.T) {
	svc := llmprovider.NewService(newFakeRepo())
	p := &domain.LLMProvider{Name: "OpenAI — Prod", Provider: domain.ProviderOpenAICompat, BaseURL: "https://api.openai.com/v1", APIKey: "key", Model: "gpt-4o-mini"}
	if err := svc.Create(t.Context(), p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.ID == uuid.Nil {
		t.Fatal("expected an assigned ID")
	}

	got, err := svc.Get(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != p.Name || got.BaseURL != p.BaseURL {
		t.Fatalf("got %+v, want %+v", got, p)
	}
}

func TestList_ReturnsCreated(t *testing.T) {
	svc := llmprovider.NewService(newFakeRepo())
	for _, name := range []string{"A", "B"} {
		if err := svc.Create(t.Context(), &domain.LLMProvider{Name: name, Provider: domain.ProviderOpenAICompat, BaseURL: "https://x"}); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	got, err := svc.List(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d providers, want 2", len(got))
	}
}

func TestUpdate_UnknownIDFails(t *testing.T) {
	svc := llmprovider.NewService(newFakeRepo())
	err := svc.Update(t.Context(), &domain.LLMProvider{ID: uuid.New(), Name: "x", Provider: domain.ProviderOpenAICompat, BaseURL: "https://x"})
	if err == nil {
		t.Fatal("expected error updating a nonexistent provider")
	}
}

func TestDelete_RemovesIt(t *testing.T) {
	svc := llmprovider.NewService(newFakeRepo())
	p := &domain.LLMProvider{Name: "A", Provider: domain.ProviderOpenAICompat, BaseURL: "https://x"}
	if err := svc.Create(t.Context(), p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := svc.Delete(t.Context(), p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(t.Context(), p.ID); err == nil {
		t.Fatal("expected get to fail after delete")
	}
}
