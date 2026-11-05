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
	audioFormatPCM16 = "pcm_8000" // native 8k PCM, no upsample artifacts
	// Legacy ulaw format kept for reference
	audioFormatULaw8 = "ulaw_8000"
	// 0.3s @8k mono 16-bit = 4800 bytes. Use 80ms flush (1280B) for liveness.
	flushBytes          = 1280
	flushInterval       = 200 * time.Millisecond
	minCommitBytes      = 4800 // 0.3s minimum for commit_throttled guard @8k
	manualCommitSilence = 800 * time.Millisecond
)

// ulaw to linear PCM table generated from standard G.711 mu-law decode.
var ulawDecodeTable [256]int16

func init() {
	for i := 0; i < 256; i++ {
		ulawDecodeTable[i] = ulawToLinear(byte(i))
	}
}

func ulawToLinear(ulaw byte) int16 {
	ulaw = ^ulaw
	sign := ulaw & 0x80
	exponent := (ulaw >> 4) & 0x07
	mantissa := ulaw & 0x0F
	sample := int16(mantissa<<3) + 0x84
	sample <<= exponent
	sample -= 0x84
	if sign != 0 {
		sample = -sample
	}
	return sample
}

// decodeUlawToPCM16 decodes mu-law bytes to little-endian 16-bit PCM at 8k (native, no upsample).
func decodeUlawToPCM16(ulaw []byte) []byte {
	if len(ulaw) == 0 {
		return nil
	}
	out := make([]byte, 0, len(ulaw)*2)
	for _, b := range ulaw {
		s := ulawDecodeTable[b]
		out = append(out, byte(s), byte(s>>8))
	}
	return out
}

// decodeUlawToPCM16_16k is the 8k->16k linear interpolation version (kept for testing pcm_16000 path).
func decodeUlawToPCM16_16k(ulaw []byte) []byte {
	if len(ulaw) == 0 {
		return nil
	}
	decoded := make([]int16, len(ulaw))
	for i, b := range ulaw {
		decoded[i] = ulawDecodeTable[b]
	}
	out := make([]byte, 0, len(ulaw)*4)
	for i := 0; i < len(decoded); i++ {
		s0 := decoded[i]
		var s1 int16
		if i+1 < len(decoded) {
			s1 = decoded[i+1]
		} else {
			s1 = s0
		}
		mid := int16((int32(s0) + int32(s1)) / 2)
		out = append(out, byte(s0), byte(s0>>8))
		out = append(out, byte(mid), byte(mid>>8))
	}
	return out
}

func pcmEnergy(pcm []byte) int {
	if len(pcm) < 2 {
		return 0
	}
	var sum int64
	var count int64
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int16(pcm[i]) | int16(pcm[i+1])<<8
		if s < 0 {
			s = -s
		}
		sum += int64(s)
		count++
	}
	if count == 0 {
		return 0
	}
	return int(sum / count)
}

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
	q.Set("audio_format", audioFormatPCM16)
	q.Set("commit_strategy", "manual")
	q.Set("vad_silence_threshold_secs", "0.5")
	q.Set("vad_threshold", "0.2")
	q.Set("min_speech_duration_ms", "50")
	q.Set("keepalive_interval_ms", "1000")
	q.Set("include_timestamps", "true")
	// Force English — auto-detect was returning Russian "Да./Алло!" for English "hello/test"
	// User reports speaking English but getting Russian. Set explicit language_code.
	q.Set("language_code", "en")
	// Optional: disable filtering that may cause empty commits
	q.Set("no_verbatim", "false")

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

	lastHighEnergy   time.Time
	bytesSinceCommit int
	speechSeen       bool
}

func (s *session) sendChunk(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if s.debug {
		slog.Info("STT elevenlabs sendChunk", "bytes", len(b))
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	err := s.conn.WriteJSON(inputAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: base64.StdEncoding.EncodeToString(b),
		Commit:      false,
		SampleRate:  sampleRateHz,
	})
	if err == nil {
		s.bufMu.Lock()
		s.bytesSinceCommit += len(b)
		s.bufMu.Unlock()
	}
	return err
}

func (s *session) sendCommit() error {
	s.bufMu.Lock()
	if s.bytesSinceCommit < minCommitBytes {
		if s.debug {
			slog.Info("STT elevenlabs commit skipped, not enough audio", "bytes", s.bytesSinceCommit, "need", minCommitBytes)
		}
		s.bufMu.Unlock()
		return nil
	}
	s.bufMu.Unlock()

	if s.debug {
		slog.Info("STT elevenlabs sendCommit manual", "bytesSinceCommit", s.bytesSinceCommit)
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	err := s.conn.WriteJSON(inputAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: "",
		Commit:      true,
		SampleRate:  sampleRateHz,
	})
	if err == nil {
		s.bufMu.Lock()
		s.bytesSinceCommit = 0
		s.speechSeen = false
		s.bufMu.Unlock()
	}
	return err
}

func (s *session) SendAudio(frame []byte) error {
	// frame is raw ulaw_8000 from RTP (20ms = 160 bytes). Transcode to pcm_16000 for better VAD.
	pcm := decodeUlawToPCM16(frame)
	energy := pcmEnergy(pcm)
	if energy > 800 {
		s.bufMu.Lock()
		s.lastHighEnergy = time.Now()
		s.speechSeen = true
		s.bufMu.Unlock()
	}

	s.bufMu.Lock()
	s.buf = append(s.buf, pcm...)
	shouldFlush := len(s.buf) >= flushBytes
	var toSend []byte
	if shouldFlush {
		toSend = append([]byte(nil), s.buf...)
		s.buf = s.buf[:0]
	}
	bufLen := len(s.buf)
	s.bufMu.Unlock()

	if s.debug && len(frame) > 0 {
		if bufLen < 7000 || bufLen%32000 == 0 {
			slog.Info("STT elevenlabs buffered", "ulaw_in", len(frame), "pcm_out", len(pcm), "energy", energy, "buffered", bufLen, "flushing", shouldFlush)
		}
	}
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
		hasBuf := len(s.buf) > 0
		var toSend []byte
		if hasBuf {
			toSend = append([]byte(nil), s.buf...)
			s.buf = s.buf[:0]
		}
		lastHigh := s.lastHighEnergy
		hasSpeech := s.speechSeen
		bytesSinceCommit := s.bytesSinceCommit
		s.bufMu.Unlock()

		if hasBuf {
			if s.debug {
				slog.Info("STT elevenlabs flushLoop sending", "bytes", len(toSend))
			}
			if err := s.sendChunk(toSend); err != nil {
				if s.debug {
					slog.Info("STT elevenlabs flushLoop send error", "err", err)
				}
				return
			}
		}

		// Manual commit: if we saw speech and then silence, commit.
		// Need at least 0.3s audio else ElevenLabs returns commit_throttled.
		if hasSpeech && !lastHigh.IsZero() && time.Since(lastHigh) > manualCommitSilence {
			if bytesSinceCommit >= minCommitBytes {
				s.bufMu.Lock()
				s.lastHighEnergy = time.Time{}
				s.bufMu.Unlock()
				if s.debug {
					slog.Info("STT elevenlabs commit after silence", "bytesSinceCommit", bytesSinceCommit)
				}
				_ = s.sendCommit()
			} else if s.debug {
				slog.Info("STT elevenlabs commit delayed, not enough audio", "bytesSinceCommit", bytesSinceCommit)
			}
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
