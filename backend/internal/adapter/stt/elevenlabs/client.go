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
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"

	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

const (
	wsURL            = "wss://api.elevenlabs.io/v1/speech-to-text/realtime"
	defaultModel     = "scribe_v2_realtime"
	sampleRateHz     = 8000
	audioFormatULaw8 = "ulaw_8000"
)

type STT struct{}

func New() *STT { return &STT{} }

func (s *STT) StartSession(ctx context.Context, cfg conversation.STTSessionConfig) (conversation.STTSession, error) {
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	q := url.Values{}
	q.Set("model_id", model)
	q.Set("audio_format", audioFormatULaw8)
	q.Set("commit_strategy", "vad")

	header := http.Header{"xi-api-key": []string{cfg.APIKey}}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL+"?"+q.Encode(), header)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs stt: dial: %w", err)
	}

	sess := &session{conn: conn, events: make(chan conversation.TranscriptEvent, 16)}
	go sess.readLoop()
	return sess, nil
}

type inputAudioChunk struct {
	MessageType string `json:"message_type"`
	AudioBase64 string `json:"audio_base_64"`
	Commit      bool   `json:"commit"`
	SampleRate  int    `json:"sample_rate"`
}

type serverMessage struct {
	MessageType string `json:"message_type"`
	Text        string `json:"text"`
}

type session struct {
	conn   *websocket.Conn
	events chan conversation.TranscriptEvent
}

func (s *session) SendAudio(frame []byte) error {
	return s.conn.WriteJSON(inputAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: base64.StdEncoding.EncodeToString(frame),
		Commit:      false,
		SampleRate:  sampleRateHz,
	})
}

func (s *session) Events() <-chan conversation.TranscriptEvent { return s.events }

func (s *session) Close() error { return s.conn.Close() }

func (s *session) readLoop() {
	defer close(s.events)
	for {
		var msg serverMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return
		}
		switch msg.MessageType {
		case "partial_transcript":
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: false}
		case "committed_transcript":
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: true}
		default:
			// session_started, committed_transcript_with_timestamps, error
			// events, etc. — not needed for v1's plain-text transcript flow.
		}
	}
}
