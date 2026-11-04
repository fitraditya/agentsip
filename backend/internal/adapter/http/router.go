// Package http wires the admin REST API's routes to their handlers. It
// imports handler constructors but contains no business logic itself.
package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/fitraditya/agentsip/internal/adapter/http/handler"
	"github.com/fitraditya/agentsip/internal/adapter/http/middleware"
	"github.com/fitraditya/agentsip/internal/usecase/auth"
	"github.com/fitraditya/agentsip/internal/usecase/call"
	"github.com/fitraditya/agentsip/internal/usecase/knowledgebase"
	"github.com/fitraditya/agentsip/internal/usecase/llmprovider"
	"github.com/fitraditya/agentsip/internal/usecase/sttprovider"
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
	"github.com/fitraditya/agentsip/internal/usecase/ttsprovider"
)

// Deps is everything NewRouter needs.
type Deps struct {
	Auth          auth.Service
	Trunk         trunk.Service
	KnowledgeBase *knowledgebase.Service
	STTProvider   sttprovider.Service
	LLMProvider   llmprovider.Service
	TTSProvider   ttsprovider.Service
	Call          call.Service
	// CallDecisions backs the webhook-mode accept/reject endpoints
	// (usecase/call.Decisions), implemented by adapter/sip.
	CallDecisions call.Decisions
	// CORSAllowedOrigins restricts which browser origins may call this API
	// (see middleware.CORS's doc comment) — empty allows any origin.
	CORSAllowedOrigins []string
}

func NewRouter(deps Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(middleware.CORS(deps.CORSAllowedOrigins))

	r.Get("/healthz", handler.Health)

	credentials := sessionAuthenticator{sessions: deps.Auth}

	// Unauthenticated: this is how a credential is obtained in the first
	// place, so none of these three can require one.
	handler.NewAuthHandler(deps.Auth).Routes(r)

	r.Group(func(tr chi.Router) {
		tr.Use(middleware.RequireAuth(credentials))

		tr.Route("/stt-providers", handler.NewSTTProviderHandler(deps.STTProvider).Routes)
		tr.Route("/llm-providers", handler.NewLLMProviderHandler(deps.LLMProvider).Routes)
		tr.Route("/tts-providers", handler.NewTTSProviderHandler(deps.TTSProvider).Routes)
		tr.Route("/trunks", handler.NewTrunkHandler(deps.Trunk).Routes)
		tr.Route("/knowledge-bases", handler.NewKnowledgeBaseHandler(deps.KnowledgeBase).Routes)
		tr.Route("/calls", func(cr chi.Router) {
			handler.NewCallHandler(deps.Call).Routes(cr)
			handler.NewCallDecisionHandler(deps.CallDecisions).Routes(cr)
		})
	})

	return r
}
