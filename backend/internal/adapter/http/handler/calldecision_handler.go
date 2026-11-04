package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/call"
)

// CallDecisionHandler exposes usecase/call.Decisions over HTTP — the
// accept/reject endpoints a webhook-mode trunk's external service calls back
// into, the same role as OpenAI's /v1/realtime/calls/{call_id}/accept and
// /reject.
type CallDecisionHandler struct {
	decisions call.Decisions
}

func NewCallDecisionHandler(decisions call.Decisions) *CallDecisionHandler {
	return &CallDecisionHandler{decisions: decisions}
}

func (h *CallDecisionHandler) Routes(r chi.Router) {
	r.Post("/{pendingCallID}/accept", h.accept)
	r.Post("/{pendingCallID}/reject", h.reject)
}

func (h *CallDecisionHandler) accept(w http.ResponseWriter, r *http.Request) {
	pendingCallID, err := h.parseIDs(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	if err := h.decisions.Accept(pendingCallID); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"pending_call_id": pendingCallID.String(), "decision": "accepted"})
}

type rejectRequest struct {
	// StatusCode is the SIP response code to reject with, 300-699, matching
	// the constraint OpenAI's reject endpoint documents. Omitted/zero uses
	// this trunk's default (480 Temporarily Unavailable).
	StatusCode int `json:"status_code,omitempty"`
}

func (h *CallDecisionHandler) reject(w http.ResponseWriter, r *http.Request) {
	pendingCallID, err := h.parseIDs(r)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	var req rejectRequest
	if r.ContentLength != 0 {
		req, err = httpx.DecodeEnvelope[rejectRequest](r)
		if err != nil {
			httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid request body"}))
			return
		}
	}
	if req.StatusCode != 0 && (req.StatusCode < 300 || req.StatusCode > 699) {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "status_code must be between 300 and 699"}))
		return
	}

	if err := h.decisions.Reject(pendingCallID, req.StatusCode); err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"pending_call_id": pendingCallID.String(), "decision": "rejected"})
}

func (h *CallDecisionHandler) parseIDs(r *http.Request) (pendingCallID uuid.UUID, err error) {
	pendingCallID, err = uuid.Parse(chi.URLParam(r, "pendingCallID"))
	if err != nil {
		return uuid.Nil, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid pending_call_id"})
	}
	return pendingCallID, nil
}
