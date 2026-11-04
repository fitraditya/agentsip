-- 0001_init.up.sql: agentsip's lite, single-deployment schema — users,
-- sessions, email verification/password reset tokens, STT/LLM/TTS provider
-- configs, trunks, calls, turns, and knowledge base/document metadata.
-- Chunk storage (with
-- its vector column) is deliberately a separate migration sequence under
-- db/migrations_vectorstore/ — see that directory's header for why.
--
-- There is no tenant dimension anywhere in this schema: this is a
-- single-deployment engine. Run with the golang-migrate CLI against
-- DATABASE_URL (see Makefile's migrate-up target) — this is not applied
-- automatically by the engine binary.

-- email is globally unique: login identifies a user by email alone.
CREATE TABLE users (
    id                uuid PRIMARY KEY,
    email             text NOT NULL UNIQUE,
    password_hash     text NOT NULL, -- bcrypt; see internal/pkg/password
    email_verified_at timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Only token_hash (sha256 hex of the plaintext bearer token) is ever stored
-- — the plaintext is never recoverable from it.
CREATE TABLE sessions (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

-- Partial index: Authenticate only ever looks up among sessions that are
-- still live.
CREATE INDEX sessions_active_hash_idx ON sessions (token_hash) WHERE revoked_at IS NULL;
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- purpose distinguishes a verification token from a password-reset token in
-- the same table — they share every other column, and Authenticate-style
-- lookups already need to filter by purpose the same way sessions filters
-- by revoked_at.
CREATE TABLE email_tokens (
    id         uuid PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    text NOT NULL, -- 'verify_email' or 'password_reset'
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz
);

-- Partial index, same reasoning as sessions_active_hash_idx: a lookup only
-- ever wants a token that hasn't been used yet.
CREATE INDEX email_tokens_active_hash_idx ON email_tokens (token_hash) WHERE used_at IS NULL;
CREATE INDEX email_tokens_user_id_idx ON email_tokens (user_id);

-- STT/LLM/TTS provider configs: standalone, named, reusable (same role
-- knowledge_bases already plays for RAG) — create one once, reference it by
-- ID from any number of trunks, instead of duplicating the same API
-- key/model on every trunk. Name is a user-facing label only.
CREATE TABLE stt_providers (
    id       uuid PRIMARY KEY,
    name     text NOT NULL,
    provider text NOT NULL,
    api_key  text NOT NULL DEFAULT '', -- field-encrypted ciphertext
    model    text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE llm_providers (
    id       uuid PRIMARY KEY,
    name     text NOT NULL,
    provider text NOT NULL,
    base_url text NOT NULL,
    api_key  text NOT NULL DEFAULT '', -- field-encrypted ciphertext
    model    text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tts_providers (
    id       uuid PRIMARY KEY,
    name     text NOT NULL,
    provider text NOT NULL,
    api_key  text NOT NULL DEFAULT '', -- field-encrypted ciphertext
    voice_id text NOT NULL DEFAULT '',
    model    text NOT NULL DEFAULT '',

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE trunks (
    id   uuid PRIMARY KEY,
    name text NOT NULL,

    sip_allowed_ips      text[] NOT NULL DEFAULT '{}',
    sip_digest_username  text NOT NULL DEFAULT '',
    sip_digest_password  text NOT NULL DEFAULT '', -- field-encrypted ciphertext, never plaintext

    -- RESTRICT, not CASCADE: deleting a still-referenced provider config
    -- must fail loudly (ErrConflict, see {stt,llm,tts}provider_repo.go's
    -- Delete) rather than silently break whatever trunk relied on it.
    stt_provider_id uuid NOT NULL REFERENCES stt_providers(id) ON DELETE RESTRICT,
    llm_provider_id uuid NOT NULL REFERENCES llm_providers(id) ON DELETE RESTRICT,
    tts_provider_id uuid NOT NULL REFERENCES tts_providers(id) ON DELETE RESTRICT,

    system_prompt      text NOT NULL DEFAULT '',
    knowledge_base_ids uuid[] NOT NULL DEFAULT '{}',

    -- Enforced in-memory by adapter/sip (Admission), not by a CHECK here:
    -- concurrency is about calls in flight on this process right now, which
    -- only the SIP layer knows. This column is just where the configured
    -- limit is persisted. 1 matches domain.DefaultMaxConcurrentCalls — a new
    -- trunk starts at one concurrent call; raising it is a plan decision,
    -- not a default.
    max_concurrent_calls int NOT NULL DEFAULT 1,

    -- 'auto' answers immediately (or after ring_delay_seconds, to simulate
    -- normal ring time); 'webhook' defers the accept/reject decision to
    -- webhook_url, waiting up to webhook_timeout_seconds for a decision
    -- through the admin API's accept/reject endpoints before rejecting on
    -- its own. See domain.AnswerMode.
    answer_mode             text NOT NULL DEFAULT 'auto',
    ring_delay_seconds      int  NOT NULL DEFAULT 0,
    webhook_url             text NOT NULL DEFAULT '',
    webhook_timeout_seconds int  NOT NULL DEFAULT 30,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Defense-in-depth backstop for the application-enforced trunks-per-
-- deployment rule (internal/usecase/trunk.Service.Create). Not a substitute
-- for it: the application check runs first and returns a usable error; this
-- only prevents a race between two concurrent creates from exceeding the
-- limit.
CREATE OR REPLACE FUNCTION enforce_trunk_limit() RETURNS trigger AS $$
BEGIN
    IF (SELECT count(*) FROM trunks) >= 3 THEN
        RAISE EXCEPTION 'this deployment already has the maximum number of trunks';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trunks_enforce_limit
    BEFORE INSERT ON trunks
    FOR EACH ROW EXECUTE FUNCTION enforce_trunk_limit();

CREATE INDEX trunks_stt_provider_id_idx ON trunks (stt_provider_id);
CREATE INDEX trunks_llm_provider_id_idx ON trunks (llm_provider_id);
CREATE INDEX trunks_tts_provider_id_idx ON trunks (tts_provider_id);

CREATE TABLE calls (
    id          uuid PRIMARY KEY,
    trunk_id    uuid NOT NULL REFERENCES trunks(id) ON DELETE CASCADE,
    sip_call_id text NOT NULL,

    caller_number text NOT NULL,
    callee_number text NOT NULL,
    direction     text NOT NULL,
    disposition   text NOT NULL,

    start_time  timestamptz NOT NULL,
    answer_time timestamptz,
    end_time    timestamptz,

    duration_seconds int NOT NULL DEFAULT 0,
    recording_url    text,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX calls_sip_call_id_idx ON calls (sip_call_id);
CREATE INDEX calls_start_time_idx ON calls (start_time DESC, id DESC);

CREATE TABLE turns (
    id         uuid PRIMARY KEY,
    call_id    uuid NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    role       text NOT NULL,
    text       text NOT NULL,
    started_at timestamptz NOT NULL,
    latency_ms int
);

CREATE INDEX turns_call_id_idx ON turns (call_id);

CREATE TABLE knowledge_bases (
    id   uuid PRIMARY KEY,
    name text NOT NULL,

    embedding_provider text NOT NULL,
    embedding_base_url text NOT NULL,
    embedding_api_key  text NOT NULL DEFAULT '', -- field-encrypted ciphertext
    embedding_model    text NOT NULL DEFAULT '',

    -- vector_store_provider empty means "use this deployment's platform-
    -- managed vector store" (config.VectorStoreDatabaseURL); set to
    -- 'pgvector' and vector_store_dsn to a different Postgres connection
    -- string to point this one knowledge base elsewhere. See
    -- internal/adapter/vectorstore/pgvector.
    vector_store_provider text NOT NULL DEFAULT '',
    vector_store_dsn      text NOT NULL DEFAULT '', -- field-encrypted ciphertext

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE documents (
    id                uuid PRIMARY KEY,
    knowledge_base_id uuid NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    source_uri        text NOT NULL,
    status            text NOT NULL,
    failure_reason    text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX documents_knowledge_base_id_idx ON documents (knowledge_base_id);
