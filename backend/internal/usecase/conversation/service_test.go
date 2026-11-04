package conversation_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

// fakeAudioIO is a conversation.AudioIO the test drives directly, standing in
// for the SIP leg.
type fakeAudioIO struct {
	in     chan []byte
	out    chan []byte
	hangup chan struct{}
}

func newFakeAudioIO() *fakeAudioIO {
	return &fakeAudioIO{in: make(chan []byte), out: make(chan []byte, 64), hangup: make(chan struct{})}
}

func (f *fakeAudioIO) In() <-chan []byte       { return f.in }
func (f *fakeAudioIO) Out() chan<- []byte      { return f.out }
func (f *fakeAudioIO) Hangup() <-chan struct{} { return f.hangup }

// fakeSTT hands back a session whose Events() channel the test owns, so the
// test can script exactly when partial/final transcripts arrive.
type fakeSTT struct {
	events chan conversation.TranscriptEvent
}

func (f *fakeSTT) StartSession(context.Context, conversation.STTSessionConfig) (conversation.STTSession, error) {
	return &fakeSTTSession{events: f.events}, nil
}

type fakeSTTSession struct {
	events chan conversation.TranscriptEvent
}

func (s *fakeSTTSession) SendAudio([]byte) error                      { return nil }
func (s *fakeSTTSession) Events() <-chan conversation.TranscriptEvent { return s.events }
func (s *fakeSTTSession) Close() error                                { return nil }

// fakeTTS records every chunk sent to it and reports when Cancel is called —
// the two things the barge-in test needs to assert on.
type fakeTTS struct {
	mu        sync.Mutex
	sentText  []string
	cancelled int
}

func (f *fakeTTS) StartSession(context.Context, conversation.TTSSessionConfig) (conversation.TTSSession, error) {
	return &fakeTTSSession{owner: f, audio: make(chan []byte)}, nil
}

func (f *fakeTTS) textLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.sentText))
	copy(out, f.sentText)
	return out
}

func (f *fakeTTS) cancelCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cancelled
}

type fakeTTSSession struct {
	owner *fakeTTS
	audio chan []byte
	once  sync.Once
}

func (s *fakeTTSSession) SendText(chunk string, flush bool) error {
	s.owner.mu.Lock()
	s.owner.sentText = append(s.owner.sentText, chunk)
	s.owner.mu.Unlock()
	return nil
}
func (s *fakeTTSSession) Audio() <-chan []byte { return s.audio }
func (s *fakeTTSSession) Cancel() error {
	s.owner.mu.Lock()
	s.owner.cancelled++
	s.owner.mu.Unlock()
	s.once.Do(func() { close(s.audio) })
	return nil
}
func (s *fakeTTSSession) Close() error {
	s.once.Do(func() { close(s.audio) })
	return nil
}

// fakeLLM lets each test script exactly how the model responds, including
// blocking mid-stream to create a window for a barge-in to land in.
type fakeLLM struct {
	handler func(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error)
}

func (f *fakeLLM) StreamChat(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error) {
	return f.handler(ctx, cfg, messages, tools)
}

func deltaChan(deltas ...conversation.LLMDelta) <-chan conversation.LLMDelta {
	ch := make(chan conversation.LLMDelta, len(deltas))
	for _, d := range deltas {
		ch <- d
	}
	close(ch)
	return ch
}

func discardLogger() logging.Logger {
	return logging.Nop()
}

// TestRun_BasicTurn proves a final transcript drives one LLM call whose
// content streams straight into TTS as it arrives, finishing with the
// documented flush-on-end-of-turn SendText("", true).
func TestRun_BasicTurn(t *testing.T) {
	stt := &fakeSTT{events: make(chan conversation.TranscriptEvent, 4)}
	tts := &fakeTTS{}
	llm := &fakeLLM{handler: func(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error) {
		// The greeting turn prepends a user message; the scripted turn has
		// "hello" as its latest user message (index 2).
		if len(messages) < 3 || messages[2].Content != "hello" {
			t.Errorf("expected user message 'hello' at index 2, got %+v", messages)
		}
		return deltaChan(
			conversation.LLMDelta{ContentDelta: "Hi"},
			conversation.LLMDelta{ContentDelta: " there"},
			conversation.LLMDelta{Done: true},
		), nil
	}}

	svc := conversation.NewService(stt, tts, llm, nil, discardLogger())
	io := newFakeAudioIO()

	runDone := make(chan error, 1)
	go func() { runDone <- svc.Run(t.Context(), conversation.RunConfig{SystemPrompt: "be helpful"}, io) }()

	// Wait for the greeting turn to finish before sending the scripted
	// transcript — playGreeting fires on Run, racing with the test otherwise.
	deadline := time.After(2 * time.Second)
	for {
		found := false
		for _, s := range tts.textLog() {
			if s == "Hi, I'm AgentSIP. How can I help you today?" {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatal("greeting never sent")
		case <-time.After(10 * time.Millisecond):
		}
	}

	stt.events <- conversation.TranscriptEvent{Text: "hello", Final: true}

	deadline = time.After(2 * time.Second)
	for {
		log := tts.textLog()
		if len(log) >= 5 && log[len(log)-1] == "" {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for tts text, got %v", log)
		case <-time.After(10 * time.Millisecond):
		}
	}

	got := tts.textLog()
	want := []string{"Hi, I'm AgentSIP. How can I help you today?", "", "Hi", " there", ""}
	if len(got) != len(want) {
		t.Fatalf("tts text log = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tts text log = %v, want %v", got, want)
		}
	}

	close(io.hangup)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after hangup")
	}
}

// TestRun_BargeInCancelsActiveTurn proves that a new final transcript arriving
// while the assistant is mid-reply cancels the in-flight TTS session rather
// than letting it finish — the whole point of barge-in.
func TestRun_BargeInCancelsActiveTurn(t *testing.T) {
	stt := &fakeSTT{events: make(chan conversation.TranscriptEvent, 4)}
	tts := &fakeTTS{}

	firstCallStarted := make(chan struct{})
	releaseFirstCall := make(chan struct{})
	callCount := 0
	var mu sync.Mutex

	llm := &fakeLLM{handler: func(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error) {
		mu.Lock()
		callCount++
		n := callCount
		mu.Unlock()

		ch := make(chan conversation.LLMDelta, 4)
		if n == 1 {
			ch <- conversation.LLMDelta{ContentDelta: "long reply in progress"}
			go func() {
				close(firstCallStarted)
				select {
				case <-releaseFirstCall:
				case <-ctx.Done():
				}
				close(ch)
			}()
		} else {
			ch <- conversation.LLMDelta{ContentDelta: "second reply"}
			ch <- conversation.LLMDelta{Done: true}
			close(ch)
		}
		return ch, nil
	}}

	svc := conversation.NewService(stt, tts, llm, nil, discardLogger())
	io := newFakeAudioIO()

	runDone := make(chan error, 1)
	go func() { runDone <- svc.Run(t.Context(), conversation.RunConfig{SystemPrompt: "be helpful"}, io) }()

	stt.events <- conversation.TranscriptEvent{Text: "first question", Final: true}
	select {
	case <-firstCallStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first llm call never started")
	}

	// Barge in before the first turn's stream ever finishes.
	stt.events <- conversation.TranscriptEvent{Text: "never mind", Final: true}

	deadline := time.After(2 * time.Second)
	for tts.cancelCount() < 1 {
		select {
		case <-deadline:
			t.Fatal("barge-in never cancelled the active tts session")
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(releaseFirstCall) // let the first (now-cancelled) call's goroutine exit cleanly

	deadline = time.After(2 * time.Second)
	for {
		log := tts.textLog()
		found := false
		for _, s := range log {
			if s == "second reply" {
				found = true
			}
		}
		if found {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("second turn's reply never reached tts, log=%v", log)
		case <-time.After(10 * time.Millisecond):
		}
	}

	close(io.hangup)
	select {
	case <-runDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after hangup")
	}
}
