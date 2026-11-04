---
topic: api-surface
status: ready
classification: INTERNAL
related: [request-flow]
code:
  - backend/internal/adapter/http/router.go
  - backend/internal/adapter/http/handler
  - backend/internal/adapter/http/httpx
verified_commit: no-git
---

# API Surface

## TL;DR

Handwritten REST JSON API, no OpenAPI/Swagger/proto contract file. Every response is wrapped
in a uniform `{"success": true/false, ...}` envelope. No version prefix (no `/v1`) anywhere
in the router.

## Endpoint catalogue

| Method | Path | Auth | Handler |
|---|---|---|---|
| GET | `/healthz` | unauthenticated | `handler.Health` |
| POST | `/register` | unauthenticated | `AuthHandler.register` (`auth_handler.go:102`) |
| POST | `/login` | unauthenticated | `AuthHandler.login` (`:120`) |
| POST | `/logout` | unauthenticated route, self-authenticates | `AuthHandler.logout` (`:139`) |
| POST / GET | `/verify-email` | unauthenticated | `AuthHandler.verifyEmail` (`:180`) |
| POST | `/resend-verification` | unauthenticated | `AuthHandler.resendVerification` (`:194`) |
| POST | `/password-reset/request` | unauthenticated | `AuthHandler.requestPasswordReset` (`:212`) |
| POST | `/password-reset/confirm` | unauthenticated | `AuthHandler.confirmPasswordReset` (`:230`) |
| GET | `/me` | unauthenticated route, self-authenticates (functionally requires a session) | `AuthHandler.me` (`:244`) |
| PUT | `/me/email` | unauthenticated route, self-authenticates | `AuthHandler.updateEmail` (`:263`) |
| PUT | `/me/password` | unauthenticated route, self-authenticates | `AuthHandler.changePassword` (`:288`) |
| POST / GET / GET{id} / PUT{id} / DELETE{id} | `/stt-providers[/{id}]` | authenticated | `STTProviderHandler.{create,list,get,update,delete}` |
| POST / GET / GET{id} / PUT{id} / DELETE{id} | `/llm-providers[/{id}]` | authenticated | `LLMProviderHandler.{create,list,get,update,delete}` |
| POST / GET / GET{id} / PUT{id} / DELETE{id} | `/tts-providers[/{id}]` | authenticated | `TTSProviderHandler.{create,list,get,update,delete}` |
| POST / GET / GET{id} / PUT{id} / DELETE{id} | `/trunks[/{trunkID}]` | authenticated | `TrunkHandler.{create,list,get,update,delete}` |
| POST | `/knowledge-bases` | authenticated | `KnowledgeBaseHandler.create` |
| GET | `/knowledge-bases` | authenticated | `KnowledgeBaseHandler.list` |
| GET | `/knowledge-bases/{knowledgeBaseID}` | authenticated | `KnowledgeBaseHandler.get` |
| POST | `/knowledge-bases/{knowledgeBaseID}/documents` | authenticated | `KnowledgeBaseHandler.ingestDocument` (paste-text, JSON) |
| POST | `/knowledge-bases/{knowledgeBaseID}/documents/upload` | authenticated | `KnowledgeBaseHandler.uploadDocument` (multipart file) |
| GET | `/knowledge-bases/{knowledgeBaseID}/documents` | authenticated | `KnowledgeBaseHandler.listDocuments` |
| GET | `/calls` | authenticated | `CallHandler.list` |
| GET | `/calls/{callID}` | authenticated | `CallHandler.get` |
| POST | `/calls/{pendingCallID}/accept` | authenticated | `CallDecisionHandler.accept` — webhook-mode callback target, see note below |
| POST | `/calls/{pendingCallID}/reject` | authenticated | `CallDecisionHandler.reject` |

**Open question (not resolved by exploration):** `/calls/{pendingCallID}/accept` and
`/reject` are literally mounted inside the same `RequireAuth`-protected group as everything
else, but the handler's own doc comment describes them as webhook callback targets analogous
to OpenAI's realtime accept/reject API — i.e. a design conceptually meant for an *external
system* to call back, which would normally need its own, different auth scheme rather than
an admin dashboard session token. Whether this is intentional or a gap wasn't determinable
from the code alone.

## Contract source

None — handwritten JSON envelope. No OpenAPI/Swagger/`.proto` file exists anywhere in the
repository.

## Response envelope

**Success:** `{"success": true, "data": {...}}` (`backend/internal/adapter/http/httpx/httpx.go:26-29,42-46`).
`data` is always a JSON object, never a bare array, so the envelope can grow without a
breaking change — e.g. `{"trunks": [...]}`, `{"user": {...}}`.

**Error:** `{"success": false, "error_code": "...", "error_message": "...", "error_details": {...}}`
(`httpx.go:31-36,51-65`). `error_details` is omitted entirely when there's nothing structured
to add. Any error that isn't already an `*apperror.Error` is mapped to a generic
`{"INTERNAL", "internal server error", 500}` — raw internal error strings are never leaked.

**Request bodies** for create/update are wrapped the same way: `{"data": {...}}`, decoded via
the generic `httpx.DecodeEnvelope[T]` (`httpx.go:87-96`).

## See Also
- [../request-flow/README.md](../request-flow/README.md)
