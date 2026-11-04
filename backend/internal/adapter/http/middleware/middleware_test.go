package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/fitraditya/agentsip/internal/adapter/http/middleware"
)

type fakeAuthenticator struct {
	err error
}

func (f *fakeAuthenticator) Authenticate(context.Context, string) error {
	return f.err
}

func newRequireAuthRouter(auth middleware.Authenticator) http.Handler {
	r := chi.NewRouter()
	r.Group(func(tr chi.Router) {
		tr.Use(middleware.RequireAuth(auth))
		tr.Get("/thing", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
	})
	return r
}

func TestRequireAuth_NoAuthorizationHeaderIsUnauthorized(t *testing.T) {
	r := newRequireAuthRouter(&fakeAuthenticator{})

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAuth_ValidCredentialSucceeds(t *testing.T) {
	r := newRequireAuthRouter(&fakeAuthenticator{})

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.Header.Set("Authorization", "Bearer obl_whatever")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequireAuth_AuthenticateErrorPropagates(t *testing.T) {
	r := newRequireAuthRouter(&fakeAuthenticator{err: context.DeadlineExceeded})

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.Header.Set("Authorization", "Bearer obl_whatever")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected a non-200 status when Authenticate fails, got 200")
	}
}
