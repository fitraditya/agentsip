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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/websocket"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

const (
	wsURLFmt            = "wss://api.elevenlabs.io/v1/text-to-speech/%s/stream-input"
	defaultModel        = "eleven_flash_v2_5"
	audioFormatULaw8000 = "ulaw_8000"
	audioFormatPCM8000  = "pcm_8000"
)

// ulaw encode table for 14-bit to 8-bit conversion (simplified)
// Using standard G.711 mu-law encode.
func linearToUlaw(pcm int16) byte {
	const (
		bias = 0x84
		clip = 32635
	)

	mu := 0
	sign := byte(0)
	if pcm < 0 {
		pcm = -pcm
		sign = 0x80
	}

	if pcm > clip {
		pcm = clip
	}
	pcm += bias

	mu = 0
	// find exponent
	exp := 7
	for exp > 0 {
		if pcm >= (1<<(exp+3)) {
			break
		}
		exp--
	}
	mantissa := (pcm >> (exp + 3)) & 0x0F
	mu = (exp << 4) | int(mantissa)
	return byte(^(mu | int(sign)))
}

func pcm16ToUlaw(pcm []byte) []byte {
	// pcm is little-endian 16-bit mono
	out := make([]byte, 0, len(pcm)/2)
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int16(pcm[i]) | int16(pcm[i+1])<<8
		out = append(out, linearToUlaw(s))
	}
	return out
}

type TTS struct {
	debug bool
}

func New(debug bool) *TTS { return &TTS{debug: debug} }

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
	// ulaw_8000 is native telephony, lowest latency, no transcode.
	// Previous pcm_8000 returned empty audio for short greeting due to chunk_length_schedule default 120.
	q.Set("output_format", audioFormatULaw8000)

	wsURL := fmt.Sprintf(wsURLFmt, url.PathEscape(cfg.VoiceID)) + "?" + q.Encode()
	header := http.Header{"xi-api-key": []string{cfg.APIKey}}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts: dial: %w", err)
	}

	init := textMessage{
		Text: " ",
		VoiceSettings: &voiceSettings{Stability: 0.5, SimilarityBoost: 0.8},
		GenerationConfig: &generationConfig{ChunkLengthSchedule: []int{50, 80, 120, 160}},
	}
	if err := conn.WriteJSON(init); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("elevenlabs tts: send init message: %w", err)
	}

	sess := &session{conn: conn, audio: make(chan []byte, 32), debug: t.debug, isUlaw: true}
	go sess.readLoop()
	return sess, nil
}

type voiceSettings struct {
	Stability       float64 `json:"stability"`
	SimilarityBoost float64 `json:"similarity_boost"`
}

type generationConfig struct {
	ChunkLengthSchedule []int `json:"chunk_length_schedule,omitempty"`
}

type textMessage struct {
	Text             string            `json:"text"`
	Flush            bool              `json:"flush,omitempty"`
	VoiceSettings    *voiceSettings    `json:"voice_settings,omitempty"`
	GenerationConfig *generationConfig `json:"generation_config,omitempty"`
}

type audioMessage struct {
	Audio   string `json:"audio"`
	IsFinal bool   `json:"isFinal"`
}

type session struct {
	conn   *websocket.Conn
	audio  chan []byte
	debug  bool
	isUlaw bool
}

func (s *session) SendText(chunk string, flush bool) error {
	if chunk != "" && !flush {
		if chunk[len(chunk)-1] != ' ' {
			chunk = chunk + " "
		}
	}
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
		_, rawBytes, err := s.conn.ReadMessage()
		if err != nil {
			if s.debug {
				logging.For("agentsip", "tts").Info().Err(err).Msg("read error")
			}
			return
		}
		var msg audioMessage
		if err := json.Unmarshal(rawBytes, &msg); err != nil {
			if s.debug {
				trunc := string(rawBytes)
				if len(trunc) > 200 {
					trunc = trunc[:200]
				}
				logging.For("agentsip", "tts").Warn().Err(err).Str("raw", trunc).Msg("unmarshal error")
			}
			// try generic error check
			var errMap map[string]any
			if json.Unmarshal(rawBytes, &errMap) == nil {
				if s.debug {
					trunc := fmt.Sprintf("%v", errMap)
					if len(trunc) > 200 {
						trunc = trunc[:200]
					}
					logging.For("agentsip", "tts").Debug().Str("map", trunc).Msg("generic message")
				}
			}
			continue
		}
		logging.For("agentsip", "tts").Info().Bool("isFinal", msg.IsFinal).Bool("hasAudio", msg.Audio != "").Int("audioLen", len(msg.Audio)).Msg("audio received")
		if msg.Audio != "" {
			raw, err := base64.StdEncoding.DecodeString(msg.Audio)
			if err != nil {
				logging.For("agentsip", "tts").Error().Err(err).Msg("base64 decode error")
				continue
			}
			var ulaw []byte
			if s.isUlaw {
				ulaw = raw
			} else {
				ulaw = pcm16ToUlaw(raw)
				if s.debug {
					logging.For("agentsip", "tts").Debug().Int("pcmBytes", len(raw)).Int("ulawBytes", len(ulaw)).Msg("transcoded")
				}
			}
			select {
			case s.audio <- ulaw:
			default:
				if s.debug {
					logging.For("agentsip", "tts").Debug().Msg("dropping audio frame, channel full")
				}
			}
		}
		if msg.IsFinal {
			return
		}
	}
}
