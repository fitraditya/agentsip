package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
)

// knowledgeBaseToolName is the one tool offered to the model when RAG is
// configured. A single, fixed tool rather than something per-trunk: the
// usecase only needs the model to be able to ask "search the knowledge base
// with this query," not to choose among tools.
const knowledgeBaseToolName = "search_knowledge_base"

// RunConfig is everything a single call's conversation loop needs, assembled
// by the caller (adapter/sip) from the resolved trunk — this package has no
// dependency on domain.Trunk itself, only on the values it carries.
type RunConfig struct {
	SystemPrompt     string
	STT              STTSessionConfig
	TTS              TTSSessionConfig
	LLM              LLMConfig
	KnowledgeBaseIDs []uuid.UUID
	// GreetingText is the opening line spoken when call answered.
	// Empty skips greeting.
	GreetingText string
	// GreetingDelaySeconds waits this many seconds after greeting finished
	// before starting STT recognition. Default 0.
	GreetingDelaySeconds int
}

// Service runs the STT -> LLM(+RAG) -> TTS loop for one call. It knows
// nothing about SIP or RTP — AudioIO is the only thing tying it to a
// transport, so the same Service runs identically against adapter/sip or a
// test harness.
type Service struct {
	stt       STT
	tts       TTS
	llm       LLM
	retriever Retriever // nil disables RAG entirely — not every trunk has a knowledge base
	log       logging.Logger
}

func NewService(stt STT, tts TTS, llm LLM, retriever Retriever, log logging.Logger) *Service {
	return &Service{stt: stt, tts: tts, llm: llm, retriever: retriever, log: log}
}

// Run blocks for the lifetime of one call. It returns when ctx is cancelled,
// io.Hangup() closes, or the STT session itself ends (e.g. the provider's
// socket drops).
func (s *Service) Run(ctx context.Context, cfg RunConfig, io AudioIO) error {
	sttSess, err := s.stt.StartSession(ctx, cfg.STT)
	if err != nil {
		return fmt.Errorf("conversation: start stt session: %w", err)
	}
	defer sttSess.Close()

	state := &turnState{messages: []LLMMessage{{Role: "system", Content: cfg.SystemPrompt}}}

	// Pump starts immediately to drain RTP In() and learn remote, but STT
	// forwarding is gated by greetingDone when greeting is configured.
	// If no greeting and no delay, STT starts immediately (integration test).
	greetingDone := make(chan struct{})
	hasGreeting := cfg.GreetingText != "" || cfg.GreetingDelaySeconds > 0
	if !hasGreeting {
		close(greetingDone)
	}
	pumpCtx, pumpCancel := context.WithCancel(ctx)
	defer pumpCancel()
	go func() {
		var framesForwarded uint64
		var framesDroppedDuringGreeting uint64
		for {
			select {
			case frame, ok := <-io.In():
				if !ok {
					s.log.Info().Msg("conversation pump: In() channel closed")
					return
				}
				if hasGreeting {
					// If greeting not done, discard frame (don't send to STT)
					select {
					case <-greetingDone:
						// greeting finished, forward to STT
					default:
						framesDroppedDuringGreeting++
						if framesDroppedDuringGreeting == 1 || framesDroppedDuringGreeting%250 == 0 {
							s.log.Debug().Int("dropped", int(framesDroppedDuringGreeting)).Msg("dropping audio during greeting")
						}
						continue
					}
				}

				framesForwarded++
				if framesForwarded == 1 || framesForwarded%250 == 0 {
					s.log.Info().Int("frame_bytes", len(frame)).Int("total", int(framesForwarded)).Msg("conversation pump: forwarding audio to STT")
				} else {
					s.log.Trace().Int("frame_bytes", len(frame)).Msg("conversation pump: forwarding audio to STT")
				}
				if err := sttSess.SendAudio(frame); err != nil {
					s.log.Warn().Err(err).Msg("conversation pump: SendAudio failed, stopping pump")
					return
				}
			case <-pumpCtx.Done():
				return
			}
		}
	}()

	// Greeting blocking, then optional delay, then close gate to start STT.
	if err := s.playGreetingBlocking(ctx, cfg, io, state); err != nil {
		s.log.Warn().Err(err).Msg("greeting failed")
	}

	if cfg.GreetingDelaySeconds > 0 {
		s.log.Info().Int("delay_seconds", cfg.GreetingDelaySeconds).Msg("greeting delay before STT")
		select {
		case <-time.After(time.Duration(cfg.GreetingDelaySeconds) * time.Second):
		case <-ctx.Done():
			return ctx.Err()
		case <-io.Hangup():
			return nil
		}
	}
	if hasGreeting {
		close(greetingDone)
	}

	for {
		select {
		case <-ctx.Done():
			state.cancelActive()
			return ctx.Err()
		case <-io.Hangup():
			state.cancelActive()
			return nil
		case ev, ok := <-sttSess.Events():
			if !ok {
				// STT session ended (e.g., after final commit). Don't cancel active turn
				// that was just started from last transcript — let it finish speaking.
				s.log.Info().Msg("stt session ended, waiting for active turn to finish")
				state.mu.Lock()
				done := state.done
				state.mu.Unlock()
				if done != nil {
					<-done
				}
				return nil
			}
			if strings.TrimSpace(ev.Text) == "" {
				continue
			}
			// Any speech activity while the assistant is talking is a
			// barge-in. Cancelling on a partial (not waiting for it to
			// become final) matters: waiting would let a full sentence of
			// unwanted assistant audio play out before cutting it off.
			state.cancelActive()
			if ev.Final {
				s.startTurn(ctx, cfg, io, state, ev.Text)
			}
		}
	}
}

// playGreeting is legacy async version, kept for reference. New flow uses
// playGreetingBlocking which plays greeting synchronously before STT starts.
func (s *Service) playGreeting(ctx context.Context, cfg RunConfig, io AudioIO, state *turnState) {
	go func() {
		_ = s.playGreetingBlocking(ctx, cfg, io, state)
	}()
}

// playGreetingBlocking speaks opening synchronously, before STT pump
// starts, so greeting cannot be cut off by barge-in. If cfg.GreetingText
// is empty, greeting is skipped entirely (straight to STT).
func (s *Service) playGreetingBlocking(ctx context.Context, cfg RunConfig, io AudioIO, state *turnState) error {
	greeting := cfg.GreetingText
	if greeting == "" {
		// No greeting configured — skip, goes straight to STT.
		s.log.Info().Msg("greeting skipped (empty)")
		return nil
	}

	state.appendMessage(LLMMessage{Role: "user", Content: "Say the opening greeting for the call."})

	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	state.mu.Lock()
	state.cancel, state.done = cancel, done
	state.mu.Unlock()

	defer func() {
		close(done)
		state.mu.Lock()
		state.cancel, state.done = nil, nil
		state.mu.Unlock()
	}()

	// Use trunk's configured greeting, fallback to default if it somehow got empty after trim.
	text := greeting
	if text == "" {
		text = "Hi, I'm AgentSIP. How can I help you today?"
	}
	if err := s.speakFixedText(turnCtx, cfg, io, text); err != nil {
		if turnCtx.Err() == nil {
			s.log.Warn().Err(err).Msg("greeting failed")
		}
		return err
	}
	return nil
}

func (s *Service) speakFixedText(ctx context.Context, cfg RunConfig, io AudioIO, text string) error {
	ttsSess, err := s.tts.StartSession(ctx, cfg.TTS)
	if err != nil {
		return fmt.Errorf("conversation: start tts for greeting: %w", err)
	}
	defer ttsSess.Close()

	audioDone := make(chan struct{})
	go func() {
		defer close(audioDone)
		for {
			select {
			case frame, ok := <-ttsSess.Audio():
				if !ok {
					return
				}
				select {
				case io.Out() <- frame:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	if err := ttsSess.SendText(text, false); err != nil {
		return fmt.Errorf("conversation: tts send greeting: %w", err)
	}
	if err := ttsSess.SendText("", true); err != nil {
		return fmt.Errorf("conversation: tts flush greeting: %w", err)
	}
	// Empty close to get isFinal true, otherwise server waits 20s and times out
	if err := ttsSess.SendText("", false); err != nil {
		return fmt.Errorf("conversation: tts close greeting: %w", err)
	}
	<-audioDone
	return nil
}

// turnState holds the running conversation history and the handle needed to
// cancel whichever assistant turn is currently in flight. One turn runs at a
// time by construction: starting a new one always cancels and waits out the
// previous one first (see cancelActive), so messages is never written by two
// turn goroutines at once.
type turnState struct {
	mu       sync.Mutex
	messages []LLMMessage
	cancel   context.CancelFunc
	done     chan struct{}
}

func (t *turnState) cancelActive() {
	t.mu.Lock()
	cancel, done := t.cancel, t.done
	t.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
}

func (t *turnState) appendMessage(m LLMMessage) {
	t.mu.Lock()
	t.messages = append(t.messages, m)
	t.mu.Unlock()
}

func (t *turnState) snapshotMessages() []LLMMessage {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]LLMMessage, len(t.messages))
	copy(out, t.messages)
	return out
}

func (s *Service) startTurn(ctx context.Context, cfg RunConfig, io AudioIO, state *turnState, userText string) {
	state.appendMessage(LLMMessage{Role: "user", Content: userText})

	turnCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	state.mu.Lock()
	state.cancel, state.done = cancel, done
	state.mu.Unlock()

	go func() {
		defer close(done)
		defer func() {
			state.mu.Lock()
			state.cancel, state.done = nil, nil
			state.mu.Unlock()
		}()

		reply, err := s.respond(turnCtx, cfg, io, state.snapshotMessages())
		if err != nil {
			if turnCtx.Err() == nil {
				// A real failure, not just this turn being interrupted by a
				// barge-in (which also cancels turnCtx).
				s.log.Error().Err(err).Msg("conversation turn failed")
			}
			return
		}
		state.appendMessage(LLMMessage{Role: "assistant", Content: reply})
	}()
}

// respond runs one assistant turn: stream the LLM's reply straight into a TTS
// session as it arrives (not after the full reply is known — that's the
// latency win of doing this streamed rather than request/response), with at
// most one knowledge-base lookup if the model asks for one.
func (s *Service) respond(ctx context.Context, cfg RunConfig, io AudioIO, messages []LLMMessage) (string, error) {
	ttsSess, err := s.tts.StartSession(ctx, cfg.TTS)
	if err != nil {
		return "", fmt.Errorf("conversation: start tts session: %w", err)
	}
	defer ttsSess.Close()

	audioDone := make(chan struct{})
	go func() {
		defer close(audioDone)
		for {
			select {
			case frame, ok := <-ttsSess.Audio():
				if !ok {
					return
				}
				select {
				case io.Out() <- frame:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		<-audioDone
		if ctx.Err() != nil {
			_ = ttsSess.Cancel()
		} else {
			_ = ttsSess.Close()
		}
	}()

	var assistantText strings.Builder
	toolCall, err := s.streamOneTurn(ctx, cfg, messages, s.toolSpecs(cfg), ttsSess, &assistantText)
	if err != nil {
		return "", err
	}

	if toolCall != nil {
		result, err := s.runTool(ctx, cfg, *toolCall)
		if err != nil {
			result = fmt.Sprintf("tool error: %v", err)
		}
		messages = append(messages,
			LLMMessage{Role: "assistant", Content: assistantText.String()},
			LLMMessage{Role: "tool", Content: result},
		)
		assistantText.Reset()
		// No tools offered on this follow-up call: it cannot ask for another
		// lookup, which is what keeps this a single round-trip rather than an
		// open-ended agentic loop.
		if _, err := s.streamOneTurn(ctx, cfg, messages, nil, ttsSess, &assistantText); err != nil {
			return "", err
		}
	}

	if err := ttsSess.SendText("", true); err != nil {
		return "", fmt.Errorf("conversation: flush tts: %w", err)
	}
	// Empty close to get isFinal true, otherwise server waits 20s and times out
	if err := ttsSess.SendText("", false); err != nil {
		return "", fmt.Errorf("conversation: close tts: %w", err)
	}
	return assistantText.String(), nil
}

func (s *Service) streamOneTurn(ctx context.Context, cfg RunConfig, messages []LLMMessage, tools []ToolSpec, ttsSess TTSSession, assistantText *strings.Builder) (*ToolCall, error) {
	deltas, err := s.llm.StreamChat(ctx, cfg.LLM, messages, tools)
	if err != nil {
		return nil, fmt.Errorf("conversation: llm stream: %w", err)
	}

	var toolCall *ToolCall
	for {
		select {
		case <-ctx.Done():
			return toolCall, ctx.Err()
		case delta, ok := <-deltas:
			if !ok {
				return toolCall, nil
			}
			if delta.ContentDelta != "" {
				assistantText.WriteString(delta.ContentDelta)
				if err := ttsSess.SendText(delta.ContentDelta, false); err != nil {
					return nil, fmt.Errorf("conversation: tts send text: %w", err)
				}
			}
			if delta.ToolCall != nil {
				toolCall = delta.ToolCall
			}
			if delta.Done {
				return toolCall, nil
			}
		}
	}
}

func (s *Service) toolSpecs(cfg RunConfig) []ToolSpec {
	if s.retriever == nil || len(cfg.KnowledgeBaseIDs) == 0 {
		return nil
	}
	return []ToolSpec{{
		Name:                 knowledgeBaseToolName,
		Description:          "Search the knowledge base for information relevant to the user's question.",
		ParametersJSONSchema: `{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`,
	}}
}

func (s *Service) runTool(ctx context.Context, cfg RunConfig, call ToolCall) (string, error) {
	if call.Name != knowledgeBaseToolName {
		return "", fmt.Errorf("conversation: unknown tool %q", call.Name)
	}

	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		return "", fmt.Errorf("conversation: parse tool arguments: %w", err)
	}

	chunks, err := s.retriever.Search(ctx, cfg.KnowledgeBaseIDs, args.Query, 5)
	if err != nil {
		return "", fmt.Errorf("conversation: knowledge base search: %w", err)
	}

	var sb strings.Builder
	for i, c := range chunks {
		fmt.Fprintf(&sb, "[%d] %s\n", i+1, c.Content)
	}
	return sb.String(), nil
}
