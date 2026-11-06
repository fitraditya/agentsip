package domain

import (
	"time"

	"github.com/google/uuid"
)

// MaxTrunks is the v1 business rule: this deployment may provision at most
// this many SIP trunks. Enforced in usecase/trunk, not the database, so it can
// later become plan-dependent without a migration.
const MaxTrunks = 3

// DefaultMaxConcurrentCalls is applied to a trunk that doesn't specify its
// own limit (usecase/trunk.Service.Create). 1 per trunk, per client, by
// default — not a generous starting point to raise later if trouble shows
// up, but the floor every plan starts at: each concurrent call on an AI
// trunk holds a live STT websocket, a TTS websocket, and LLM requests, all
// billable provider usage, not just a phone line. Raising it is a plan
// upgrade, not a config tweak a new trunk gets by default. There is no Plan
// entity yet to attach that to — when one exists, it should own this number
// (or own it per trunk tier), and this constant becomes its fallback for
// a trunk with no plan assigned, not the limit itself.
const DefaultMaxConcurrentCalls = 1

// ProviderKind identifies which concrete adapter backs an STT/LLM/TTS config.
type ProviderKind string

const (
	ProviderElevenLabs   ProviderKind = "elevenlabs"
	ProviderOpenAICompat ProviderKind = "openai_compatible"
	ProviderPGVector     ProviderKind = "pgvector"
)

// STTConfig selects and configures the speech-to-text provider for a trunk.
// APIKey is field-encrypted at rest (internal/pkg/crypto) — it is plaintext only
// in memory, never logged or returned by the admin API after creation.
type STTConfig struct {
	Provider ProviderKind
	APIKey   string
	Model    string
}

// TTSConfig selects and configures the text-to-speech provider for a trunk.
type TTSConfig struct {
	Provider ProviderKind
	APIKey   string
	VoiceID  string
	Model    string
}

// LLMConfig points at an OpenAI-compatible chat completions endpoint. BaseURL
// is what makes this provider-agnostic rather than OpenAI-specific: any
// deployment implementing the same wire protocol (OpenAI itself, Azure OpenAI,
// a self-hosted OpenAI-compatible server) plugs in without a new adapter.
type LLMConfig struct {
	Provider ProviderKind
	BaseURL  string
	APIKey   string
	Model    string
}

// STTProvider, LLMProvider, and TTSProvider are standalone, named, reusable
// provider configs — the same role Knowledge Bases already played for RAG:
// create one once, then reference it by ID from any number of trunks,
// instead of re-entering the same API key/model on every trunk. Name is a
// user-facing label only (e.g. "ElevenLabs — Prod"), not used for lookup.
type STTProvider struct {
	ID   uuid.UUID
	Name string

	Provider ProviderKind
	APIKey   string
	Model    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type LLMProvider struct {
	ID   uuid.UUID
	Name string

	Provider ProviderKind
	BaseURL  string
	APIKey   string
	Model    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type TTSProvider struct {
	ID   uuid.UUID
	Name string

	Provider ProviderKind
	APIKey   string
	VoiceID  string
	Model    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// AnswerMode decides how a trunk's agent accepts or rejects an inbound call
// once it has passed admission control.
type AnswerMode string

const (
	// AnswerModeAuto answers immediately once idle — optionally after
	// RingDelaySeconds, to simulate normal ring time rather than picking up
	// instantly — and rejects outright when the trunk is at capacity. This
	// is the default: no external dependency, no extra latency unless asked
	// for.
	AnswerModeAuto AnswerMode = "auto"
	// AnswerModeWebhook defers the accept/reject decision to an external
	// service: WebhookURL is notified of the incoming call, and the call
	// waits (ringing) for that service to call back through the admin API's
	// accept/reject endpoints (usecase/call.Decisions) — the same shape as
	// OpenAI's realtime.call.incoming webhook plus its accept/reject API.
	// Unlike OpenAI's version, this is not how every call is handled; it's
	// an opt-in per-trunk mode for when the decision to take a call depends
	// on something outside this platform (e.g. a CRM check).
	AnswerModeWebhook AnswerMode = "webhook"
)

// DefaultWebhookTimeoutSeconds is how long a webhook-mode call waits for an
// accept/reject decision before being rejected automatically — matching
// OpenAI's reasoning that an unanswered decision must not hang a call
// forever, just with a concrete default where their docs leave the behavior
// unspecified.
const DefaultWebhookTimeoutSeconds = 30

// SIPAuth is how an inbound INVITE is matched to this trunk. AllowedIPs is the
// carrier/SBC's source IP allowlist; the digest credentials are an alternative
// (or additional) authentication path for carriers that register/authenticate
// rather than relying on static IPs. DigestPassword is field-encrypted at rest.
type SIPAuth struct {
	AllowedIPs     []string
	DigestUsername string
	DigestPassword string
}

// Trunk is a SIP connection plus the AI agent configuration that
// answers calls on it. There is deliberately no separate "Agent" entity: since
// each trunk owns its own STT/LLM/TTS selection, a trunk already is the unit
// an agent config would otherwise duplicate.
//
// STT/LLM/TTS reference a standalone STTProvider/LLMProvider/TTSProvider by
// ID (so the same provider config can be shared across trunks), but the
// repository resolves that reference into the embedded STT/LLM/TTS config
// values at read time — adapter/sip's hot call path reads t.STT.APIKey etc.
// directly and has no reason to know providers are a separate table.
type Trunk struct {
	ID   uuid.UUID
	Name string

	SIPAuth SIPAuth

	STTProviderID uuid.UUID
	LLMProviderID uuid.UUID
	TTSProviderID uuid.UUID

	// STT/LLM/TTS are resolved from {STT,LLM,TTS}ProviderID by the
	// repository, not persisted on this row — never set these directly
	// except from a repo's scan function.
	STT STTConfig
	LLM LLMConfig
	TTS TTSConfig

	// SystemPrompt is the instruction given to the LLM at the start of every
	// call on this trunk.
	SystemPrompt string

	// KnowledgeBaseIDs are the knowledge bases this trunk's agent may search
	// via RAG retrieval. A knowledge base can be shared across
	// multiple trunks, so this is a reference list, not ownership.
	KnowledgeBaseIDs []uuid.UUID

	// MaxConcurrentCalls bounds how many calls this trunk will answer at
	// once. Enforced in-memory by adapter/sip (internal/adapter/sip's
	// Admission), not by the database — concurrency is about calls actually
	// in flight on this process right now, which only the SIP layer knows.
	MaxConcurrentCalls int

	// AnswerMode, RingDelaySeconds, WebhookURL, and WebhookTimeoutSeconds
	// govern how a call that passed admission control gets accepted or
	// rejected — see AnswerMode's doc comment. RingDelaySeconds and the
	// webhook fields are only meaningful for their respective mode; a trunk
	// in "auto" mode with a WebhookURL set simply never uses it.
	AnswerMode            AnswerMode
	RingDelaySeconds      int
	WebhookURL            string
	WebhookTimeoutSeconds int

	// GreetingText is the opening line spoken when a call is answered.
	// Empty means skip greeting and go straight to listening (STT).
	// When non-empty, the engine TTSes this text first, then optionally
	// waits GreetingDelaySeconds before starting STT.
	GreetingText string
	// GreetingDelaySeconds is how many seconds to wait after greeting
	// finished before starting speech recognition. Default 0.
	GreetingDelaySeconds int

	CreatedAt time.Time
	UpdatedAt time.Time
}
