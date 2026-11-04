package openaicompat_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/fitraditya/agentsip/internal/adapter/embedder/openaicompat"
	"github.com/fitraditya/agentsip/internal/domain"
)

func TestEmbed_OrdersByResponseIndexNotArrayOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		var reqBody struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(reqBody.Input) != 2 {
			t.Fatalf("expected 2 inputs, got %d", len(reqBody.Input))
		}

		// Respond with index 1 before index 0, to prove the adapter places
		// embeddings by their reported index rather than response order.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[
			{"index":1,"embedding":[0.3,0.4]},
			{"index":0,"embedding":[0.1,0.2]}
		]}`))
	}))
	defer srv.Close()

	e := openaicompat.New()
	got, err := e.Embed(t.Context(), domain.EmbeddingConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "text-embedding-3-small"}, []string{"first", "second"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	want := [][]float32{{0.1, 0.2}, {0.3, 0.4}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Embed() = %v, want %v", got, want)
	}
}

func TestEmbed_MismatchedResponseCountIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1]}]}`))
	}))
	defer srv.Close()

	e := openaicompat.New()
	_, err := e.Embed(t.Context(), domain.EmbeddingConfig{BaseURL: srv.URL, APIKey: "k"}, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected an error when the response has fewer embeddings than inputs")
	}
}

func TestEmbed_EmptyInputReturnsNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()

	e := openaicompat.New()
	got, err := e.Embed(t.Context(), domain.EmbeddingConfig{BaseURL: srv.URL, APIKey: "k"}, nil)
	if err != nil {
		t.Fatalf("Embed(nil): %v", err)
	}
	if got != nil {
		t.Fatalf("Embed(nil) = %v, want nil", got)
	}
	if called {
		t.Fatal("expected no HTTP request for empty input")
	}
}
