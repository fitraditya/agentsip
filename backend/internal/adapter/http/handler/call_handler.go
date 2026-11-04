package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/adapter/http/httpx"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/usecase/call"
)

// CallHandler exposes read access to call records (usecase/call.Service) —
// the accept/reject decisions themselves are CallDecisionHandler's job, a
// separate port with a separate authorization story (see its doc comment).
type CallHandler struct {
	svc call.Service
}

func NewCallHandler(svc call.Service) *CallHandler {
	return &CallHandler{svc: svc}
}

func (h *CallHandler) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Get("/{callID}", h.get)
}

const defaultCallListLimit = 50

type callResponse struct {
	ID           uuid.UUID  `json:"id"`
	TrunkID      uuid.UUID  `json:"trunk_id"`
	SIPCallID    string     `json:"sip_call_id"`
	CallerNumber string     `json:"caller_number"`
	CalleeNumber string     `json:"callee_number"`
	Direction    string     `json:"direction"`
	Disposition  string     `json:"disposition"`
	StartTime    time.Time  `json:"start_time"`
	AnswerTime   *time.Time `json:"answer_time,omitempty"`
	EndTime      *time.Time `json:"end_time,omitempty"`

	DurationSeconds int     `json:"duration_seconds"`
	RecordingURL    *string `json:"recording_url,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toCallResponse(c *domain.CallRecord) callResponse {
	return callResponse{
		ID:              c.ID,
		TrunkID:         c.TrunkID,
		SIPCallID:       c.SIPCallID,
		CallerNumber:    c.CallerNumber,
		CalleeNumber:    c.CalleeNumber,
		Direction:       string(c.Direction),
		Disposition:     string(c.Disposition),
		StartTime:       c.StartTime,
		AnswerTime:      c.AnswerTime,
		EndTime:         c.EndTime,
		DurationSeconds: c.DurationSeconds,
		RecordingURL:    c.RecordingURL,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

// list is cursor-paginated by ?before=<RFC3339 start_time>&limit=<n>, mirroring
// usecase/call.Page: calls arrive continuously, so an offset would shift
// under a reader paging through live data.
func (h *CallHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := defaultCallListLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "limit must be a positive integer"}))
			return
		}
		limit = n
	}

	calls, err := h.svc.List(r.Context(), call.Page{Limit: limit, Before: r.URL.Query().Get("before")})
	if err != nil {
		httpx.WriteError(w, err)
		return
	}

	resp := make([]callResponse, 0, len(calls))
	for _, c := range calls {
		resp = append(resp, toCallResponse(c))
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"calls": resp})
}

func (h *CallHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "callID"))
	if err != nil {
		httpx.WriteError(w, apperror.ErrValidation.WithDetails(map[string]any{"reason": "invalid call id"}))
		return
	}

	c, err := h.svc.Get(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, err)
		return
	}
	httpx.WriteSuccess(w, http.StatusOK, map[string]any{"call": toCallResponse(c)})
}
