package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/ttsprovider"
)

type TTSProviderHandler struct {
	svc ttsprovider.Service
}

func NewTTSProviderHandler(svc ttsprovider.Service) *TTSProviderHandler {
	return &TTSProviderHandler{svc: svc}
}

func (h *TTSProviderHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

type ttsProviderRequest struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	APIKey   string `json:"api_key,omitempty"`
	VoiceID  string `json:"voice_id"`
	Model    string `json:"model"`
}

// ttsProviderResponse omits the API key: write-only through this API, same
// reasoning as trunkResponse's secret fields.
type ttsProviderResponse struct {
	ID               uuid.UUID `json:"id"`
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	APIKeyConfigured bool      `json:"api_key_configured"`
	VoiceID          string    `json:"voice_id"`
	Model            string    `json:"model"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func toTTSProviderResponse(p *domain.TTSProvider) ttsProviderResponse {
	return ttsProviderResponse{
		ID:               p.ID,
		Name:             p.Name,
		Provider:         string(p.Provider),
		APIKeyConfigured: p.APIKey != "",
		VoiceID:          p.VoiceID,
		Model:            p.Model,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}

func (req ttsProviderRequest) toDomain(id uuid.UUID) *domain.TTSProvider {
	return &domain.TTSProvider{
		ID:       id,
		Name:     req.Name,
		Provider: domain.ProviderKind(req.Provider),
		APIKey:   req.APIKey,
		VoiceID:  req.VoiceID,
		Model:    req.Model,
	}
}

func (h *TTSProviderHandler) create(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[ttsProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(uuid.Nil)
	if err := h.svc.Create(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"tts_provider": toTTSProviderResponse(p)})
}

func (h *TTSProviderHandler) list(w http.ResponseWriter, r *http.Request) {
	providers, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]ttsProviderResponse, 0, len(providers))
	for _, p := range providers {
		resp = append(resp, toTTSProviderResponse(p))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"tts_providers": resp})
}

func (h *TTSProviderHandler) get(w http.ResponseWriter, r *http.Request) {
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
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"tts_provider": toTTSProviderResponse(p)})
}

func (h *TTSProviderHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid id"}))
		return
	}

	req, err := httpx.DecodeEnvelope[ttsProviderRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	p := req.toDomain(id)
	if err := h.svc.Update(r.Context(), p); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"tts_provider": toTTSProviderResponse(p)})
}

func (h *TTSProviderHandler) delete(w http.ResponseWriter, r *http.Request) {
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
