# Glossary

Single source of truth for terminology used across `.ai/docs/`. Use the **bolded form** when
writing or editing any doc. Add an entry here before introducing a new term.

---

## 1. Domain Concepts

| Term | Definition |
|---|---|
| **Trunk** | The admin-plane configuration resource for one SIP trunk: SIP auth rules, a reference to one STT/LLM/TTS provider each (by ID), system prompt, answer mode. Model class `domain.Trunk`. Max 3 per deployment. |
| **Provider** (STT / LLM / TTS) | A standalone, reusable, named config for one external speech-to-text, language-model, or text-to-speech service, referenced by ID from a trunk — never inlined into it. Model classes `domain.STTProvider`/`LLMProvider`/`TTSProvider`. |
| **Knowledge Base** | A RAG resource: an embedding-provider config + a vector-store config (platform-managed or bring-your-own Postgres), holding **Documents** that get chunked and embedded. Model class `domain.KnowledgeBase`. |
| **Document** | One ingested piece of text inside a Knowledge Base, with a `status` and optional `failure_reason`. Model class `domain.Document`. |
| **Chunk** | A piece of a Document's text plus its embedding vector, stored in the `chunks` table for similarity search. Model class `domain.Chunk`. |
| **Call** (Call Record) | One SIP call's lifecycle record: trunk, caller/callee, disposition, timing. Model class `domain.CallRecord`. |
| **Conversation Turn** | One user/assistant exchange within a call's STT→LLM→TTS loop. Model class `domain.ConversationTurn` — **defined but not currently persisted to Postgres**, see [topics/calls](topics/calls/README.md) Gotchas. |
| **Session** | A bearer-token credential for the admin dashboard login, hashed (never stored plaintext), 24h TTL. Model class `domain.Session`. |
| **Email Token** | A single-use, hashed, purpose-tagged token backing both email verification and password reset. Model class `domain.EmailToken`. |
| **Admission** | In-memory per-trunk concurrent-call counter enforced at INVITE time, before any RTP/DB allocation. |
| **Pending Call** | An in-memory record of a call awaiting a webhook-mode accept/reject decision from an external system. |
| **Drain** | Graceful shutdown: refuse new INVITEs (503), wait for in-flight calls to end, then forcibly BYE anything still live past a deadline. |
| **FK-resolution-at-read-time** | The pattern where a Trunk row stores only provider IDs, and the repository JOINs + resolves the full STT/LLM/TTS config on every read — so the SIP hot path needs zero extra round-trips per call. |

## 2. Data Model (cross-cutting)

| Model | Table | Notes |
|---|---|---|
| `User` | `users` | bcrypt `password_hash` |
| `Session` | `sessions` | only `token_hash` (SHA-256) stored |
| `EmailToken` | `email_tokens` | shared by verify-email + password-reset, differentiated by `purpose` |
| `STTProvider`/`LLMProvider`/`TTSProvider` | `stt_providers`/`llm_providers`/`tts_providers` | `api_key` field-encrypted (AES-256-GCM) |
| `Trunk` | `trunks` | FK to the three provider tables (`ON DELETE RESTRICT`); `sip_digest_password` field-encrypted |
| `CallRecord` | `calls` | FK `trunk_id` → `trunks` (`ON DELETE CASCADE`) |
| `ConversationTurn` | `turns` | table exists; **no writer/reader found** — not actually used yet |
| `KnowledgeBase` | `knowledge_bases` | `embedding_api_key` and `vector_store_dsn` both field-encrypted |
| `Document` | `documents` | FK `knowledge_base_id` → `knowledge_bases` (`ON DELETE CASCADE`) |
| `Chunk` | `chunks` (separate migration set) | no DB-level FKs — may live on a different Postgres instance than the rest of the schema |

## 3. Status Values

| Field | Possible values | Where seen |
|---|---|---|
| `Document.status` | `PENDING`/`PROCESSING`, `READY`, `FAILED` | knowledge-base |
| `CallRecord.Disposition` | `IN_PROGRESS`, `ANSWERED`, `BUSY`, `NO_ANSWER`, `FAILED`, `CANCELLED` | calls (only `IN_PROGRESS`→`ANSWERED` confirmed reachable) |
| `Trunk.AnswerMode` | `auto`, `webhook` | trunks, calls |
| `ConversationTurn.Role` | `USER`, `ASSISTANT` | calls |

## 4. Architecture Patterns

| Term | Definition |
|---|---|
| **Use Case** (usecase layer) | Business-logic package per bounded context (`internal/usecase/<name>`); declares its own `Repository`/`Service` port interfaces, depends only on `domain`. |
| **Adapter** | Concrete implementation of a usecase-declared port (`internal/adapter/<name>`) — HTTP delivery, Postgres repositories, SIP, external provider clients. |
| **Domain** | Pure entity structs with zero outward dependencies (`internal/domain`). |
| **Composition root** | `backend/cmd/engine/main.go` — the single place every layer is wired together. |
| **Response envelope** | This API's uniform JSON wrapper: `{"success": true, "data": {...}}` or `{"success": false, "error_code", "error_message", "error_details"}`. |
| **ComponentCard** | The frontend's standard title+divider card wrapper for a feature-page section (`frontend/src/components/shared/component-card.tsx`). |
| **Dedicated page (vs. modal)** | This frontend's convention: every create/edit flow is a routed page, not a dialog — with one confirmed exception (knowledge-base document ingestion). |

## 5. External Services

> Use the **bolded form** when referencing any of these.

### Infrastructure
| Service | Purpose |
|---|---|
| **Postgres** | Primary datastore for everything except vector chunks, which may optionally live on a separate, bring-your-own Postgres instance (via `pgvector`). |

### Third-party SaaS / provider integrations
| Service | Purpose |
|---|---|
| **ElevenLabs** | STT and TTS provider integration (`adapter/stt/elevenlabs`, `adapter/tts/elevenlabs`) — the only STT/TTS provider kind the frontend currently offers |
| **OpenAI-compatible** | LLM and embedding provider integration (`adapter/llm/openaicompat`, `adapter/embedder/openaicompat`) — the only LLM/embedding provider kind the frontend currently offers; the backend's `ProviderKind` type is wider than what the UI exposes |

### Naming rules
- **ElevenLabs** (one word, capital E and L) — not "Eleven Labs" or "elevenlabs" in prose
  (the latter is fine as a literal enum/config value, e.g. `"provider": "elevenlabs"`).
- **OpenAI-compatible** (hyphenated) when describing the provider kind; `openai_compatible`
  only as the literal snake_case enum value in code/JSON.

## 6. Acronyms

| Acronym | Expansion |
|---|---|
| RAG | Retrieval-Augmented Generation (the knowledge-base feature's purpose) |
| STT | Speech-to-Text |
| TTS | Text-to-Speech |
| LLM | Large Language Model |
| SDP | Session Description Protocol (SIP media negotiation) |
| RTP | Real-time Transport Protocol (the audio media stream) |
| SIP | Session Initiation Protocol |
| DSN | Data Source Name (a database connection string) |
| FK | Foreign Key |
| TTL | Time To Live |
| CSPRNG | Cryptographically Secure Pseudo-Random Number Generator |
