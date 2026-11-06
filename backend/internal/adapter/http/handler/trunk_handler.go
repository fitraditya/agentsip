package handler

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
)

type TrunkHandler struct {
	svc trunk.Service
}

func NewTrunkHandler(svc trunk.Service) *TrunkHandler {
	return &TrunkHandler{svc: svc}
}

func (h *TrunkHandler) Routes(r chi.Router) {
	r.Post("/", h.create)
	r.Get("/", h.list)
	r.Get("/{trunkID}", h.get)
	r.Put("/{trunkID}", h.update)
	r.Delete("/{trunkID}", h.delete)
}

type sipAuthDTO struct {
	AllowedIPs     []string `json:"allowed_ips"`
	DigestUsername string   `json:"digest_username"`
	DigestPassword string   `json:"digest_password,omitempty"`
}

type trunkRequest struct {
	Name    string     `json:"name"`
	SIPAuth sipAuthDTO `json:"sip_auth"`

	// STT/LLM/TTSProviderID each reference a standalone provider config
	// (see sttprovider/llmprovider/ttsprovider handlers) — create one of
	// each there first, then pick it here. A nonexistent ID is reported as
	// VALIDATION_FAILED, not a generic error.
	STTProviderID uuid.UUID `json:"stt_provider_id"`
	LLMProviderID uuid.UUID `json:"llm_provider_id"`
	TTSProviderID uuid.UUID `json:"tts_provider_id"`

	SystemPrompt     string      `json:"system_prompt"`
	KnowledgeBaseIDs []uuid.UUID `json:"knowledge_base_ids"`
	// MaxConcurrentCalls: 0/omitted defaults to domain.DefaultMaxConcurrentCalls
	// (see usecase/trunk.Service.Create).
	MaxConcurrentCalls int `json:"max_concurrent_calls,omitempty"`

	// AnswerMode: "auto" (default) or "webhook" — see domain.AnswerMode.
	AnswerMode            string `json:"answer_mode,omitempty"`
	RingDelaySeconds      int    `json:"ring_delay_seconds,omitempty"`
	WebhookURL            string `json:"webhook_url,omitempty"`
	WebhookTimeoutSeconds int    `json:"webhook_timeout_seconds,omitempty"`

	GreetingText         string `json:"greeting_text,omitempty"`
	GreetingDelaySeconds int    `json:"greeting_delay_seconds,omitempty"`
}

// trunkResponse deliberately omits every secret (API keys, digest password):
// it only reports whether one is configured. Trunk secrets are write-only
// through this API, same as a password field.
type trunkResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`

	SIPAuth struct {
		AllowedIPs       []string `json:"allowed_ips"`
		DigestUsername   string   `json:"digest_username"`
		DigestConfigured bool     `json:"digest_configured"`
	} `json:"sip_auth"`

	// STT/LLM/TTS repeat the referenced provider's own kind/model (read-only,
	// resolved from the join in trunk_repo.go) alongside the ID, so a
	// trunk's response is self-describing without a second fetch — but the
	// provider's user-facing Name is not duplicated here; the frontend maps
	// {STT,LLM,TTS}ProviderID to a name via its own fetched provider lists.
	STTProviderID uuid.UUID `json:"stt_provider_id"`
	STT           struct {
		Provider         string `json:"provider"`
		Model            string `json:"model"`
		APIKeyConfigured bool   `json:"api_key_configured"`
	} `json:"stt"`

	LLMProviderID uuid.UUID `json:"llm_provider_id"`
	LLM           struct {
		Provider         string `json:"provider"`
		BaseURL          string `json:"base_url"`
		Model            string `json:"model"`
		APIKeyConfigured bool   `json:"api_key_configured"`
	} `json:"llm"`

	TTSProviderID uuid.UUID `json:"tts_provider_id"`
	TTS           struct {
		Provider         string `json:"provider"`
		VoiceID          string `json:"voice_id"`
		Model            string `json:"model"`
		APIKeyConfigured bool   `json:"api_key_configured"`
	} `json:"tts"`

	SystemPrompt       string      `json:"system_prompt"`
	KnowledgeBaseIDs   []uuid.UUID `json:"knowledge_base_ids"`
	MaxConcurrentCalls int         `json:"max_concurrent_calls"`

	AnswerMode            string `json:"answer_mode"`
	RingDelaySeconds      int    `json:"ring_delay_seconds"`
	WebhookURL            string `json:"webhook_url,omitempty"`
	WebhookTimeoutSeconds int    `json:"webhook_timeout_seconds,omitempty"`

	GreetingText         string `json:"greeting_text"`
	GreetingDelaySeconds int    `json:"greeting_delay_seconds"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toTrunkResponse(t *domain.Trunk) trunkResponse {
	var resp trunkResponse
	resp.ID = t.ID
	resp.Name = t.Name

	resp.SIPAuth.AllowedIPs = t.SIPAuth.AllowedIPs
	resp.SIPAuth.DigestUsername = t.SIPAuth.DigestUsername
	resp.SIPAuth.DigestConfigured = t.SIPAuth.DigestPassword != ""

	resp.STTProviderID = t.STTProviderID
	resp.STT.Provider = string(t.STT.Provider)
	resp.STT.Model = t.STT.Model
	resp.STT.APIKeyConfigured = t.STT.APIKey != ""

	resp.LLMProviderID = t.LLMProviderID
	resp.LLM.Provider = string(t.LLM.Provider)
	resp.LLM.BaseURL = t.LLM.BaseURL
	resp.LLM.Model = t.LLM.Model
	resp.LLM.APIKeyConfigured = t.LLM.APIKey != ""

	resp.TTSProviderID = t.TTSProviderID
	resp.TTS.Provider = string(t.TTS.Provider)
	resp.TTS.VoiceID = t.TTS.VoiceID
	resp.TTS.Model = t.TTS.Model
	resp.TTS.APIKeyConfigured = t.TTS.APIKey != ""

	resp.SystemPrompt = t.SystemPrompt
	resp.KnowledgeBaseIDs = t.KnowledgeBaseIDs
	resp.MaxConcurrentCalls = t.MaxConcurrentCalls
	resp.AnswerMode = string(t.AnswerMode)
	resp.RingDelaySeconds = t.RingDelaySeconds
	resp.WebhookURL = t.WebhookURL
	resp.WebhookTimeoutSeconds = t.WebhookTimeoutSeconds
	resp.GreetingText = t.GreetingText
	resp.GreetingDelaySeconds = t.GreetingDelaySeconds
	resp.CreatedAt = t.CreatedAt
	resp.UpdatedAt = t.UpdatedAt
	return resp
}

func (req trunkRequest) toDomain(id uuid.UUID) *domain.Trunk {
	return &domain.Trunk{
		ID:   id,
		Name: req.Name,
		SIPAuth: domain.SIPAuth{
			AllowedIPs:     req.SIPAuth.AllowedIPs,
			DigestUsername: req.SIPAuth.DigestUsername,
			DigestPassword: req.SIPAuth.DigestPassword,
		},
		STTProviderID:        req.STTProviderID,
		LLMProviderID:        req.LLMProviderID,
		TTSProviderID:        req.TTSProviderID,
		SystemPrompt:         req.SystemPrompt,
		KnowledgeBaseIDs:     req.KnowledgeBaseIDs,
		MaxConcurrentCalls:   req.MaxConcurrentCalls,
		AnswerMode:           domain.AnswerMode(req.AnswerMode),
		RingDelaySeconds:     req.RingDelaySeconds,
		WebhookURL:           req.WebhookURL,
		WebhookTimeoutSeconds: req.WebhookTimeoutSeconds,
		GreetingText:         req.GreetingText,
		GreetingDelaySeconds: req.GreetingDelaySeconds,
	}
}

func (h *TrunkHandler) create(w http.ResponseWriter, r *http.Request) {
	req, err := httpx.DecodeEnvelope[trunkRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	t := req.toDomain(uuid.Nil)
	if err := h.svc.Create(r.Context(), t); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusCreated, map[string]any{"trunk": toTrunkResponse(t)})
}

func (h *TrunkHandler) list(w http.ResponseWriter, r *http.Request) {
	trunks, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]trunkResponse, 0, len(trunks))
	for _, t := range trunks {
		resp = append(resp, toTrunkResponse(t))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"trunks": resp})
}

func (h *TrunkHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "trunkID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid trunk id"}))
		return
	}

	t, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"trunk": toTrunkResponse(t)})
}

func (h *TrunkHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "trunkID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid trunk id"}))
		return
	}

	req, err := httpx.DecodeEnvelope[trunkRequest](r)
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
		return
	}

	t := req.toDomain(id)
	if err := h.svc.Update(r.Context(), t); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"trunk": toTrunkResponse(t)})
}

func (h *TrunkHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "trunkID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid trunk id"}))
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"id": id.String(), "deleted": true})
}
