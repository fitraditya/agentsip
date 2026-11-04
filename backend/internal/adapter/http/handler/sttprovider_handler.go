package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/sttprovider"
)

type STTProviderHandler struct {
	svc sttprovider.Service
}

func NewSTTProviderHandler(svc sttprovider.Service) *STTProviderHandler {
	return &STTProviderHandler{svc: svc}
}

func (h *STTProviderHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

type sttProviderRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
	Model    string `json:"model"`
}

// sttProviderResponse omits the API key: write-only through this API, same
// reasoning as trunkResponse's secret fields.
type sttProviderResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	Model            string    `json:"model"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toSTTProviderResponse(p *domain.STTProvider) sttProviderResponse {
	return sttProviderResponse{
		ID:               p.ID,
		Name:             p.Name,
		Provider:         string(p.Provider),
		APIKeyConfigured: p.APIKey != "",
		Model:            p.Model,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func (req sttProviderRequest) toDomain(id uuid.UUID) *domain.STTProvider {
	return &domain.STTProvider{
		ID:       id,
		Name:     req.Name,
		Provider: domain.ProviderKind(req.Provider),
		APIKey:   req.APIKey,
		Model:    req.Model,
	}
}

func (h *STTProviderHandler) create(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[sttProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(uuid.Nil)
	if err := h.svc.Create(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"stt_provider": toSTTProviderResponse(p)})
}

func (h *STTProviderHandler) list(w http.ResponseWriter, r *http.Request) {
	providers, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]sttProviderResponse, 0, len(providers))
	for _, p := range providers {
		resp = append(resp, toSTTProviderResponse(p))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"stt_providers": resp})
}

func (h *STTProviderHandler) get(w http.ResponseWriter, r *http.Request) {
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
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"stt_provider": toSTTProviderResponse(p)})
}

func (h *STTProviderHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid id"}))
		return
	}

	req, err := httpx.DecodeEnvelope[sttProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(id)
	if err := h.svc.Update(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"stt_provider": toSTTProviderResponse(p)})
}

func (h *STTProviderHandler) delete(w http.ResponseWriter, r *http.Request) {
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
