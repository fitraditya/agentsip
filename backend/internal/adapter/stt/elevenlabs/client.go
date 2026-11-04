// Package elevenlabs implements usecase/conversation.STT against ElevenLabs'
// realtime speech-to-text WebSocket API (Scribe v2 Realtime).
//
// Wire protocol per ElevenLabs' API reference
// (elevenlabs.io/docs/api-reference/speech-to-text/v-1-speech-to-text-realtime):
// connect to wss://api.elevenlabs.io/v1/speech-to-text/realtime with an
// xi-api-key header, send {"message_type":"input_audio_chunk",
// "audio_base_64":...,"commit":false,"sample_rate":8000} per audio frame, and
// receive {"message_type":"committed_transcript","text":...} once VAD (or a
// manual commit) ends an utterance. This adapter always requests
// audio_format=ulaw_8000: that is raw G.711 u-law, byte-for-byte what
// adapter/sip already extracts from RTP, so no transcoding sits between the
// SIP leg and this adapter.
//
// The exact JSON shape of partial_transcript was not confirmed against the
// live API at the time this was written (ElevenLabs' docs describe its
// behavior but the reference page's schema for it was unavailable) —
// ASSUMPTION: it mirrors committed_transcript's {message_type, text} shape.
// Verify against a real session before depending on partial text content;
// Final (committed_transcript) is unaffected by this and is what this
// adapter's caller actually keys turn-taking on.
package elevenlabs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

const (
	wsURL            = "wss://api.elevenlabs.io/v1/speech-to-text/realtime"
	defaultModel     = "scribe_v2_realtime"
	sampleRateHz     = 8000
	audioFormatULaw8 = "ulaw_8000"
	// flushBytes = 200ms of 8kHz ulaw (160 bytes * 10). Docs recommend
	// 0.1-1s chunks, not 20ms (160 bytes) — tiny chunks cause queue_overflow
	// and waste. We buffer in session and flush 200ms batches.
	flushBytes    = 1600
	flushInterval = 200 * time.Millisecond
)

type STT struct {
	debug bool
}

func New(debug bool) *STT { return &STT{debug: debug} }

func (s *STT) StartSession(ctx context.Context, cfg conversation.STTSessionConfig) (conversation.STTSession, error) {
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	q := url.Values{}
	q.Set("model_id", model)
	q.Set("audio_format", audioFormatULaw8)
	q.Set("commit_strategy", "vad")
	// VAD tuning for telephony: faster commit, avoid long empty segments.
	q.Set("vad_silence_threshold_secs", "0.8")
	// keepalive prevents server closing after 15s with no client msgs
	// (happens during silence when no RTP arrives). Empty partials from
	// keepalive are filtered in conversation/service.go.
	q.Set("keepalive_interval_ms", "1000")

	header := http.Header{"xi-api-key": []string{cfg.APIKey}}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL+"?"+q.Encode(), header)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: dial: %w", err)
	}

	sess := &session{
		conn:   conn,
		events: make(chan conversation.TranscriptEvent, 16),
		debug:  s.debug,
	}
	go sess.readLoop()
	go sess.flushLoop()
	return sess, nil
}

type inputAudioChunk struct {
	MessageType string `json:"message_type"`
	AudioBase64 string `json:"audio_base_64"`
	Commit      bool   `json:"commit"`
	SampleRate  int    `json:"sample_rate"`
}

type serverMessage struct {
	MessageType string          `json:"message_type"`
	Text        string          `json:"text"`
	Raw         json.RawMessage `json:"-"` // keep raw for debug via custom unmarshal below
}

type rawServerMessage struct {
	MessageType string `json:"message_type"`
	Text        string `json:"text"`
	// error fields
	ErrorType string `json:"error_type"`
	Message   string `json:"message"`
	Status    string `json:"status"`
}

type session struct {
	connMu sync.Mutex
	conn   *websocket.Conn
	events chan conversation.TranscriptEvent
	debug  bool

	bufMu sync.Mutex
	buf   []byte
}

func (s *session) sendChunk(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn.WriteJSON(inputAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: base64.StdEncoding.EncodeToString(b),
		Commit:      false,
		SampleRate:  sampleRateHz,
	})
}

func (s *session) SendAudio(frame []byte) error {
	s.bufMu.Lock()
	s.buf = append(s.buf, frame...)
	shouldFlush := len(s.buf) >= flushBytes
	var toSend []byte
	if shouldFlush {
		toSend = append([]byte(nil), s.buf...)
		s.buf = s.buf[:0]
	}
	s.bufMu.Unlock()
	if shouldFlush {
		return s.sendChunk(toSend)
	}
	return nil
}

func (s *session) flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.bufMu.Lock()
		if len(s.buf) == 0 {
			s.bufMu.Unlock()
			continue
		}
		toSend := append([]byte(nil), s.buf...)
		s.buf = s.buf[:0]
		s.bufMu.Unlock()
		if err := s.sendChunk(toSend); err != nil {
			return
		}
	}
}

func (s *session) Events() <-chan conversation.TranscriptEvent { return s.events }

func (s *session) Close() error {
	// flush remaining buffered audio before close
	s.bufMu.Lock()
	leftover := append([]byte(nil), s.buf...)
	s.buf = s.buf[:0]
	s.bufMu.Unlock()
	if len(leftover) > 0 {
		_ = s.sendChunk(leftover)
		// small delay to let server process final chunk before socket close
		time.Sleep(100 * time.Millisecond)
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	return s.conn.Close()
}

func (s *session) readLoop() {
	defer close(s.events)
	for {
		// read raw to log unknown types when debug
		_, raw, err := s.conn.ReadMessage()
		if err != nil {
			if s.debug {
				slog.Info("STT elevenlabs read error, closing", "err", err)
			}
			return
		}
		var msg rawServerMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			if s.debug {
				slog.Info("STT elevenlabs unmarshal fail", "raw", string(raw), "err", err)
			}
			continue
		}
		if s.debug {
			// log full raw for unknown types
			if msg.MessageType != "partial_transcript" && msg.MessageType != "committed_transcript" {
				slog.Info("STT elevenlabs event", "type", msg.MessageType, "text", msg.Text, "raw", string(raw))
			} else {
				slog.Info("STT elevenlabs event", "type", msg.MessageType, "text", msg.Text)
			}
		}
		switch msg.MessageType {
		case "partial_transcript":
			// keepalive empty partials (text=="") filtered upstream, but keep here
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: false}
		case "committed_transcript":
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: true}
		case "committed_transcript_with_timestamps":
			// only arrives if include_timestamps=true, currently not set.
			// Emit as final transcript as well for robustness.
			if msg.Text != "" {
				s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: true}
			}
		case "session_started":
			// handshake, ignore
			continue
		case "input_error", "transcriber_error", "auth_error", "quota_exceeded", "invalid_request", "error", "rate_limited", "queue_overflow", "resource_exhausted", "chunk_size_exceeded", "commit_throttled", "insufficient_audio_activity":
			slog.Error("STT elevenlabs server error", "type", msg.MessageType, "error_type", msg.ErrorType, "msg", msg.Message, "raw", string(raw))
			// don't close immediately, let server close, but log
			continue
		default:
			// unknown future type
			continue
		}
	}
}
