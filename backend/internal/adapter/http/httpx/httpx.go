// Package httpx holds the wire format shared by every handler package:
// request/response envelope, error-to-status mapping, and request context
// extraction. Separate from internal/adapter/http (the router) so handler
// packages can depend on it without an import cycle back through the router.
//
// Envelope convention — matched to obrelin's (Decision 25), deliberately:
// these are sibling services, and one client library should be able to talk
// to both.
//
//	request:  {"data": {...}}
//	success:  {"success": true,  "data": {...}}
//	error:    {"success": false, "error_code": "...", "error_message": "...", "error_details": {...}}
//
// error_details is omitted entirely when there's nothing structured to add.
package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/fitraditya/agentsip/internal/pkg/apperror"
)

type successEnvelope struct {
	Success bool `json:"success"`
	Data    any  `json:"data"`
}

type errorEnvelope struct {
	Success      bool           `json:"success"`
	ErrorCode    string         `json:"error_code"`
	ErrorMessage string         `json:"error_message"`
	ErrorDetails map[string]any `json:"error_details,omitempty"`
}

// WriteSuccess wraps data in {"success": true, "data": ...}. data should
// itself be a JSON object (a resource, or a keyed list like
// {"trunks": [...]}) — never a bare array, so the envelope can grow (e.g.
// pagination cursors) without a breaking change.
func WriteSuccess(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(successEnvelope{Success: true, Data: data})
}

// WriteError maps any error to the uniform envelope. A non-*apperror.Error is
// treated as an unexpected internal failure — its raw message is never
// leaked to the client, only logged by the caller.
func WriteError(w http.ResponseWriter, err error) {
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		appErr = apperror.New("INTERNAL", "internal server error", http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(appErr.HTTPStatus)
	_ = json.NewEncoder(w).Encode(errorEnvelope{
		Success:      false,
		ErrorCode:    appErr.Code,
		ErrorMessage: appErr.Message,
		ErrorDetails: appErr.Details,
	})
}

// BearerToken extracts the credential from an Authorization header. The
// scheme is matched case-insensitively per RFC 7235, which defines it as a
// case-insensitive token. Shared by middleware.RequireAuth and any handler
// (e.g. logout) that needs the raw presented token rather than an
// already-authenticated identity.
func BearerToken(r *http.Request) (string, bool) {
	const scheme = "bearer "

	header := r.Header.Get("Authorization")
	if len(header) < len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}

	token := strings.TrimSpace(header[len(scheme):])
	return token, token != ""
}

// DecodeEnvelope unmarshals a request body of the form {"data": {...}} into
// T. Every handler's create/update path uses this instead of decoding the
// body directly.
func DecodeEnvelope[T any](r *http.Request) (T, error) {
	var envelope struct {
		Data T `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
		var zero T
		return zero, err
	}
	return envelope.Data, nil
}
