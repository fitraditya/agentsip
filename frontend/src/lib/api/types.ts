/**
 * Domain types for the agentsip backend REST API.
 *
 * Field names are kept as exact snake_case to mirror the backend JSON 1:1 —
 * no camelCase mapping layer. This keeps the client a thin, predictable
 * pass-through: what you see here is exactly what arrives over the wire.
 */

// ---------------------------------------------------------------------------
// Shared / enums
// ---------------------------------------------------------------------------

/** `domain.ProviderKind` */
export type ProviderKind = "elevenlabs" | "openai_compatible" | "pgvector";

/** `domain.AnswerMode` */
export type AnswerMode = "auto" | "webhook";

/** `domain.DocumentStatus` */
export type DocumentStatus = "PENDING" | "PROCESSING" | "READY" | "FAILED";

/** `domain.CallDirection` */
export type CallDirection = "INBOUND" | "OUTBOUND";

/** `domain.CallDisposition` */
export type CallDisposition =
  | "IN_PROGRESS"
  | "ANSWERED"
  | "BUSY"
  | "NO_ANSWER"
  | "FAILED"
  | "CANCELLED";

// ---------------------------------------------------------------------------
// Auth
// ---------------------------------------------------------------------------

export interface User {
  id: string;
  email: string;
  email_verified: boolean;
  created_at: string;
}

/** Stored verbatim in sessionStorage — see `src/lib/api/session.ts`. */
export interface Session {
  token: string;
  expires_at: string;
}

export interface UpdateEmailRequest {
  email: string;
  current_password: string;
}

export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

export interface LoginResult {
  user: User;
  session: Session;
}

// ---------------------------------------------------------------------------
// Trunks
// ---------------------------------------------------------------------------

export interface SipAuthRequest {
  allowed_ips: string[];
  digest_username: string;
  /** Write-only. Omit/leave unset to keep unchanged on create; on update,
   * PUT is a full replace, so an empty string here *clears* it. */
  digest_password?: string;
}

export interface SipAuthResponse {
  allowed_ips: string[];
  digest_username: string;
  digest_configured: boolean;
}

/** Read-only, resolved-at-read-time info the backend joins in alongside a
 * trunk's {stt,llm,tts}_provider_id — NOT the provider's user-facing `name`
 * (the backend doesn't duplicate that here); map provider_id -> name using
 * `sttProvidersApi.list()` / `llmProvidersApi.list()` / `ttsProvidersApi.list()`
 * client-side if you need a friendly label. */
export interface TrunkSttInfo {
  provider: ProviderKind;
  model: string;
  api_key_configured: boolean;
}
export interface TrunkLlmInfo {
  provider: ProviderKind;
  base_url: string;
  model: string;
  api_key_configured: boolean;
}
export interface TrunkTtsInfo {
  provider: ProviderKind;
  voice_id: string;
  model: string;
  api_key_configured: boolean;
}

/** Shared shape for POST /trunks and PUT /trunks/{id} — PUT is a full
 * replace, not a merge/PATCH. Callers must resend every field every time.
 * stt/llm/tts_provider_id must each reference an existing STTProvider/
 * LLMProvider/TTSProvider (create those first — see sttProvidersApi etc.);
 * an unknown ID comes back as a VALIDATION_FAILED ApiError. */
export interface TrunkRequest {
  name: string;
  sip_auth: SipAuthRequest;
  stt_provider_id: string;
  llm_provider_id: string;
  tts_provider_id: string;
  system_prompt: string;
  knowledge_base_ids: string[];
  /** Optional; 0/omitted defaults to 1 server-side. */
  max_concurrent_calls?: number;
  /** Optional; defaults to "auto" server-side. */
  answer_mode?: AnswerMode;
  /** Only used in "auto" mode. */
  ring_delay_seconds?: number;
  /** Only used in "webhook" mode. */
  webhook_url?: string;
  /** Only used in "webhook" mode; defaults to 30 server-side. */
  webhook_timeout_seconds?: number;
}

/** Response shape for create/list/get/update — secrets never come back. */
export interface Trunk {
  id: string;
  name: string;
  sip_auth: SipAuthResponse;
  stt_provider_id: string;
  stt: TrunkSttInfo;
  llm_provider_id: string;
  llm: TrunkLlmInfo;
  tts_provider_id: string;
  tts: TrunkTtsInfo;
  system_prompt: string;
  knowledge_base_ids: string[];
  max_concurrent_calls: number;
  answer_mode: AnswerMode;
  ring_delay_seconds: number;
  /** Omitted by the backend when empty/zero. */
  webhook_url?: string;
  webhook_timeout_seconds?: number;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// STT / LLM / TTS Providers — standalone, reusable (same role Knowledge
// Bases already play for RAG): create one once, reference it by ID from any
// number of trunks.
// ---------------------------------------------------------------------------

export interface SttProviderRequest {
  name: string;
  provider: ProviderKind;
  /** Write-only secret; never echoed back. */
  api_key?: string;
  model: string;
}

export interface SttProvider {
  id: string;
  name: string;
  provider: ProviderKind;
  api_key_configured: boolean;
  model: string;
  created_at: string;
  updated_at: string;
}

export interface LlmProviderRequest {
  name: string;
  provider: ProviderKind;
  base_url: string;
  api_key?: string;
  model: string;
}

export interface LlmProvider {
  id: string;
  name: string;
  provider: ProviderKind;
  base_url: string;
  api_key_configured: boolean;
  model: string;
  created_at: string;
  updated_at: string;
}

export interface TtsProviderRequest {
  name: string;
  provider: ProviderKind;
  api_key?: string;
  voice_id: string;
  model: string;
}

export interface TtsProvider {
  id: string;
  name: string;
  provider: ProviderKind;
  api_key_configured: boolean;
  voice_id: string;
  model: string;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// Knowledge Bases
// ---------------------------------------------------------------------------

export interface EmbeddingConfigRequest {
  provider: ProviderKind;
  base_url?: string;
  api_key?: string;
  model: string;
}

export interface EmbeddingConfigResponse {
  provider: ProviderKind;
  base_url: string;
  model: string;
  api_key_configured: boolean;
}

export interface VectorStoreConfigRequest {
  /** "pgvector" if bringing your own Postgres. */
  provider: ProviderKind;
  dsn: string;
}

export interface VectorStoreConfigResponse {
  provider: ProviderKind;
  custom_dsn_set: boolean;
}

export interface KnowledgeBaseRequest {
  name: string;
  embedding: EmbeddingConfigRequest;
  /** Omit entirely to use the platform-managed vector store. */
  vector_store?: VectorStoreConfigRequest;
}

export interface KnowledgeBase {
  id: string;
  name: string;
  embedding: EmbeddingConfigResponse;
  vector_store: VectorStoreConfigResponse;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeBaseDocumentRequest {
  /** Just a label (filename/URL) — not actually fetched. */
  source_uri: string;
  text: string;
}

export interface KnowledgeBaseDocument {
  id: string;
  knowledge_base_id: string;
  source_uri: string;
  status: DocumentStatus;
  failure_reason: string | null;
}

// ---------------------------------------------------------------------------
// Calls
// ---------------------------------------------------------------------------

export interface Call {
  id: string;
  trunk_id: string;
  sip_call_id: string;
  caller_number: string;
  callee_number: string;
  direction: CallDirection;
  disposition: CallDisposition;
  start_time: string;
  /** Omitted until the call is answered. */
  answer_time?: string;
  /** Omitted until the call ends. */
  end_time?: string;
  duration_seconds: number;
  /** Omitted when there is no recording. */
  recording_url?: string;
  created_at: string;
  updated_at: string;
}

export interface ListCallsParams {
  /** Default 50 server-side. */
  limit?: number;
  /** Opaque paging cursor from a previous page — treat as a black box. */
  before?: string;
}
