// Package conversation will own the live STT->LLM->TTS orchestration for a
// call, including RAG retrieval and barge-in. Only the ports are defined here
// so far: the service depends on the SIP/media leg and the provider adapters,
// none of which exist yet (next build slice). Declaring the ports now lets
// those adapters be built and type-checked against a stable contract.
package conversation

import (
	"context"

	"github.com/google/uuid"
)

// AudioIO is the narrow interface the SIP/media adapter implements per call.
// This package never touches RTP/SIP directly — it would run identically
// against a test harness or a different transport.
type AudioIO interface {
	// In streams raw PCM frames decoded from the call's negotiated codec.
	In() <-chan []byte
	// Out is where synthesized audio is written to be sent back out.
	Out() chan<- []byte
	// Hangup is closed when the call tears down from the SIP side (BYE,
	// timeout, transport error).
	Hangup() <-chan struct{}
}

// TranscriptEvent is the provider-agnostic form of an STT result. Final is
// true for a committed/end-of-utterance transcript (whatever the provider
// calls it — e.g. ElevenLabs' committed_transcript) and is this package's
// end-of-turn signal; providers with their own built-in VAD/endpointing need
// no special-casing here.
type TranscriptEvent struct {
	Text  string
	Final bool
}

type STTSessionConfig struct {
	APIKey string
	Model  string
}

type STTSession interface {
	// SendAudio streams one frame of input audio to the provider.
	SendAudio(frame []byte) error
	// Events delivers transcript updates as they arrive. Closed when the
	// session ends (provider error or explicit Close).
	Events() <-chan TranscriptEvent
	Close() error
}

type STT interface {
	StartSession(ctx context.Context, cfg STTSessionConfig) (STTSession, error)
}

type TTSSessionConfig struct {
	APIKey  string
	VoiceID string
	Model   string
}

type TTSSession interface {
	// SendText streams one chunk of text to synthesize. flush forces a
	// synthesis boundary rather than waiting for more text to buffer.
	SendText(chunk string, flush bool) error
	// Audio delivers synthesized audio frames as they're produced.
	Audio() <-chan []byte
	// Cancel aborts synthesis immediately — the barge-in hook: called when
	// the caller starts speaking while this session is still playing out.
	Cancel() error
	Close() error
}

type TTS interface {
	StartSession(ctx context.Context, cfg TTSSessionConfig) (TTSSession, error)
}

type LLMMessage struct {
	Role    string // "system", "user", "assistant", "tool"
	Content string
}

// LLMDelta is one streamed increment of a chat completion.
type LLMDelta struct {
	ContentDelta string
	// ToolCall is set when the model invokes a tool (e.g. knowledge-base
	// search) instead of — or in addition to — emitting content.
	ToolCall *ToolCall
	Done     bool
}

type ToolCall struct {
	Name      string
	Arguments string // raw JSON arguments, parsed by the tool's handler
}

type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type LLM interface {
	// StreamChat sends the conversation so far and streams the model's
	// reply. Tools, when non-empty, are offered to the model as callable
	// functions (used for RAG retrieval — see Retriever).
	StreamChat(ctx context.Context, cfg LLMConfig, messages []LLMMessage, tools []ToolSpec) (<-chan LLMDelta, error)
}

type ToolSpec struct {
	Name        string
	Description string
	// ParametersJSONSchema is the tool's parameters, as a JSON Schema
	// object, in the shape the LLM provider's function-calling API expects.
	ParametersJSONSchema string
}

// Retriever is the RAG search port, implemented directly by
// usecase/knowledgebase.Service — not by an adapter — since retrieval is
// itself a usecase (ingestion and search share the same Embedder/VectorStore
// ports) rather than infrastructure. knowledgeBaseIDs is required, not
// optional, so a caller cannot search without explicitly naming the scope.
type Retriever interface {
	Search(ctx context.Context, knowledgeBaseIDs []uuid.UUID, query string, topK int) ([]RetrievedChunk, error)
}

type RetrievedChunk struct {
	Content string
	Score   float32
}
