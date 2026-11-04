// Package openaicompat implements usecase/knowledgebase.Embedder against any
// OpenAI-compatible /embeddings endpoint — the same provider-agnostic
// approach as adapter/llm/openaicompat, and often literally the same server
// (most OpenAI-compatible deployments serve both routes).
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/fitraditya/agentsip/internal/domain"
)

type Embedder struct {
	httpClient *http.Client
}

func New() *Embedder {
	return &Embedder{httpClient: &http.Client{}}
}

type embeddingsRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingsResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func (e *Embedder) Embed(ctx context.Context, cfg domain.EmbeddingConfig, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(embeddingsRequest{Model: cfg.Model, Input: texts})
	if err != nil {
		return nil, fmt.Errorf("openaicompat embedder: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.BaseURL, "/")+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openaicompat embedder: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openaicompat embedder: request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("openaicompat embedder: unexpected status %d: %s", resp.StatusCode, errBody)
	}

	var parsed embeddingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("openaicompat embedder: decode response: %w", err)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("openaicompat embedder: expected %d embeddings, got %d", len(texts), len(parsed.Data))
	}

	// The API documents index as the input's position but doesn't guarantee
	// response ordering matches request ordering, so this places each
	// embedding by its own index rather than trusting array order.
	out := make([][]float32, len(texts))
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= len(out) {
			return nil, fmt.Errorf("openaicompat embedder: response index %d out of range", d.Index)
		}
		out[d.Index] = d.Embedding
	}
	return out, nil
}
