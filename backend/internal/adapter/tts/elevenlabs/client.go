// Package elevenlabs implements usecase/conversation.TTS against ElevenLabs'
// text-to-speech streaming WebSocket API.
//
// Wire protocol per ElevenLabs' developer guide
// (elevenlabs.io/docs/developers/websockets): connect to
// wss://api.elevenlabs.io/v1/text-to-speech/{voice_id}/stream-input with an
// xi-api-key header, send an initialize message ({"text":" ",
// "voice_settings":{...}}) before any real text, then {"text":"<chunk>"} per
// piece of text and {"text":"","flush":true} (or just send more text) to
// control synthesis boundaries; an empty {"text":""} closes the stream.
// Responses are {"audio":"<base64>","isFinal":bool}. This adapter always
// requests output_format=ulaw_8000 — raw G.711 u-law, matching adapter/sip's
// RTP payload byte-for-byte, so audio flows TTS -> RTP with no transcoding.
//
// output_format=ulaw_8000 is not confirmed against ElevenLabs' own API
// reference for this specific endpoint (their docs only state the parameter
// accepts "any type," with no enumerated values on the page available at
// implementation time) — ASSUMPTION based on it being a documented value
// elsewhere in ElevenLabs' API and the standard choice for telephony
// integrations. Verify against a real session before relying on it; if
// unsupported here, the fallback is default (MP3) output transcoded in
// pkg/audio, not a code change in this file.
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
	wsURLFmt            = "wss://api.elevenlabs.io/v1/text-to-speech/%s/stream-input"
	defaultModel        = "eleven_flash_v2_5"
	audioFormatULaw8000 = "ulaw_8000"
)

type TTS struct{}

func New() *TTS { return &TTS{} }

func (t *TTS) StartSession(ctx context.Context, cfg conversation.TTSSessionConfig) (conversation.TTSSession, error) {
	if cfg.VoiceID == "" {
		return nil, fmt.Errorf("elevenlabs tts: voice_id is required")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}

	q := url.Values{}
	q.Set("model_id", model)
	q.Set("output_format", audioFormatULaw8000)

	wsURL := fmt.Sprintf(wsURLFmt, url.PathEscape(cfg.VoiceID)) + "?" + q.Encode()
	header := http.Header{"xi-api-key": []string{cfg.APIKey}}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: dial: %w", err)
	}

	init := textMessage{Text: " ", VoiceSettings: &voiceSettings{Stability: 0.5, SimilarityBoost: 0.8}}
	if err := conn.WriteJSON(init); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("elevenlabs tts: send init message: %w", err)
	}

	sess := &session{conn: conn, audio: make(chan []byte, 16)}
	go sess.readLoop()
	return sess, nil
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

type textMessage struct {
	Text          string         `json:"text"`
	Flush         bool           `json:"flush,omitempty"`
	VoiceSettings *voiceSettings `json:"voice_settings,omitempty"`
}

type audioMessage struct {
	Audio   string `json:"audio"`
	IsFinal bool   `json:"isFinal"`
}

type session struct {
	conn  *websocket.Conn
	audio chan []byte
}

func (s *session) SendText(chunk string, flush bool) error {
	return s.conn.WriteJSON(textMessage{Text: chunk, Flush: flush})
}

func (s *session) Audio() <-chan []byte { return s.audio }

// Cancel is the barge-in hook: ElevenLabs' protocol has no documented
// graceful "stop synthesizing," so this hard-closes the socket. The caller
// (usecase/conversation) treats a closed Audio channel as the session ending,
// which this also causes.
func (s *session) Cancel() error {
	return s.conn.Close()
}

func (s *session) Close() error {
	// An empty text message is the documented way to end the stream after
	// whatever was sent finishes generating, rather than cutting it off.
	_ = s.conn.WriteJSON(textMessage{Text: ""})
	return s.conn.Close()
}

func (s *session) readLoop() {
	defer close(s.audio)
	for {
		var msg audioMessage
		if err := s.conn.ReadJSON(&msg); err != nil {
			return
		}
		if msg.Audio != "" {
			raw, err := base64.StdEncoding.DecodeString(msg.Audio)
			if err == nil {
				s.audio <- raw
			}
		}
		if msg.IsFinal {
			return
		}
	}
}
