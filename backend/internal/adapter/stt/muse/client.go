package muse

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"

	"github.com/fitraditya/agentsip/internal/pkg/audio"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

const (
	wsURL        = "wss://api.meta.ai/v1/asr/realtime"
	defaultModel = "muse-voice-transcribe-1.0"
)

type STT struct{}

func New() *STT { return &STT{} }

func (s *STT) StartSession(ctx context.Context, cfg conversation.STTSessionConfig) (conversation.STTSession, error) {
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("muse stt: dial: %w", err)
	}

	handshake := map[string]any{
		"authorization": map[string]string{"accessToken": "Bearer " + cfg.APIKey},
		"audioEncoding": "PCM_16KHZ",
		"model":         model,
		"mode":          "ENDPOINTING",
		"partialMode":   "CUMULATIVE",
	}
	if err := conn.WriteJSON(handshake); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("muse stt: handshake: %w", err)
	}

	var ack map[string]any
	if err := conn.ReadJSON(&ack); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("muse stt: handshake ack: %w", err)
	}
	if _, ok := ack["sessionId"]; !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("muse stt: handshake failed: %v", ack)
	}

	sess := &session{conn: conn, events: make(chan conversation.TranscriptEvent, 16)}
	go sess.readLoop()
	return sess, nil
}

type serverMessage struct {
	Type       string `json:"type"`
	Transcript string `json:"transcript"`
	Final      *bool  `json:"final"`
}

type session struct {
	conn   *websocket.Conn
	events chan conversation.TranscriptEvent
}

func (s *session) SendAudio(frame []byte) error {
	pcm := audio.Ulaw8kToPCM16k(frame)
	return s.conn.WriteMessage(websocket.BinaryMessage, pcm)
}

func (s *session) Events() <-chan conversation.TranscriptEvent { return s.events }

func (s *session) Close() error {
	_ = s.conn.WriteJSON(map[string]string{"type": "endStream"})
	return s.conn.Close()
}

func (s *session) readLoop() {
	defer close(s.events)
	for {
		msgType, data, err := s.conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.BinaryMessage {
			continue
		}
		var msg serverMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "transcript":
			if msg.Transcript == "" {
				continue
			}
			isFinal := msg.Final != nil && *msg.Final
			s.events <- conversation.TranscriptEvent{Text: msg.Transcript, Final: isFinal}
		case "speechComplete":
			if msg.Transcript == "" {
				continue
			}
			s.events <- conversation.TranscriptEvent{Text: msg.Transcript, Final: true}
		case "error":
			return
		}
	}
}
