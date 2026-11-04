package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/llmprovider"
)

type LLMProviderHandler struct {
	svc llmprovider.Service
}

func NewLLMProviderHandler(svc llmprovider.Service) *LLMProviderHandler {
	return &LLMProviderHandler{svc: svc}
}

func (h *LLMProviderHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

type llmProviderRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
}

// llmProviderResponse omits the API key: write-only through this API, same
// reasoning as trunkResponse's secret fields.
type llmProviderResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	BaseURL          string    `json:"base_url"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	Model            string    `json:"model"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toLLMProviderResponse(p *domain.LLMProvider) llmProviderResponse {
	return llmProviderResponse{
		ID:               p.ID,
		Name:             p.Name,
		Provider:         string(p.Provider),
		BaseURL:          p.BaseURL,
		APIKeyConfigured: p.APIKey != "",
		Model:            p.Model,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func (req llmProviderRequest) toDomain(id uuid.UUID) *domain.LLMProvider {
	return &domain.LLMProvider{
		ID:       id,
		Name:     req.Name,
		Provider: domain.ProviderKind(req.Provider),
		BaseURL:  req.BaseURL,
		APIKey:   req.APIKey,
		Model:    req.Model,
	}
}

func (h *LLMProviderHandler) create(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[llmProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(uuid.Nil)
	if err := h.svc.Create(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"llm_provider": toLLMProviderResponse(p)})
}

func (h *LLMProviderHandler) list(w http.ResponseWriter, r *http.Request) {
	providers, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]llmProviderResponse, 0, len(providers))
	for _, p := range providers {
		resp = append(resp, toLLMProviderResponse(p))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"llm_providers": resp})
}

func (h *LLMProviderHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid id"}))
		return
	}

	p, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"llm_provider": toLLMProviderResponse(p)})
}

func (h *LLMProviderHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid id"}))
		return
	}

	req, err := httpx.DecodeEnvelope[llmProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(id)
	if err := h.svc.Update(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"llm_provider": toLLMProviderResponse(p)})
}

func (h *LLMProviderHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid id"}))
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"id": id.String(), "deleted": true})
}
