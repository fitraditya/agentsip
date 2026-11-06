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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

const (
	wsURL            = "wss://api.elevenlabs.io/v1/speech-to-text/realtime"
	defaultModel     = "scribe_v2_realtime"
	sampleRateHz     = 8000
	audioFormatPCM16 = "pcm_8000" // native 8k PCM, no upsample artifacts
	// Legacy ulaw format kept for reference
	audioFormatULaw8 = "ulaw_8000"
	// Tuned for high-latency APAC VPS (Indonesia -> ElevenLabs US 34.8.184.191).
	// US VPS works with 3200B, Indonesia needs even smaller due to 300-500ms RTT.
	// flush 320B = 20ms (1 RTP frame), commit 640B = 40ms allows quick commit
	// even when WriteJSON blocks, caller doesn't hang up waiting for commit.
	flushBytes            = 320  // 20ms @8k PCM16 (1 RTP frame)
	flushInterval         = 80 * time.Millisecond
	minCommitBytes        = 640 // 40ms @8k PCM16, was 3200 for US, 9600 originally
	manualCommitSilence   = 500 * time.Millisecond
	speechEnergyThreshold = 150
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

type Config struct {
	FlushBytes          int
	FlushInterval       time.Duration
	MinCommitBytes      int
	ManualCommitSilence time.Duration
	SpeechThreshold     int
}

type STT struct {
	debug  bool
	config Config
}

func New(debug bool) *STT {
	return NewWithConfig(debug, Config{
		FlushBytes:          320,
		FlushInterval:       80 * time.Millisecond,
		MinCommitBytes:      640,
		ManualCommitSilence: 500 * time.Millisecond,
		SpeechThreshold:     150,
	})
}

func NewWithConfig(debug bool, cfg Config) *STT {
	// Apply defaults if zero
	if cfg.FlushBytes == 0 {
		cfg.FlushBytes = 320
	}
	if cfg.FlushInterval == 0 {
		cfg.FlushInterval = 80 * time.Millisecond
	}
	if cfg.MinCommitBytes == 0 {
		cfg.MinCommitBytes = 640
	}
	if cfg.ManualCommitSilence == 0 {
		cfg.ManualCommitSilence = 500 * time.Millisecond
	}
	if cfg.SpeechThreshold == 0 {
		cfg.SpeechThreshold = 150
	}
	return &STT{debug: debug, config: cfg}
}

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
		config: s.config,
	}
	if s.debug {
		// Dump raw PCM for offline analysis when AGENTSIP_DEBUG=true
		// File: /tmp/stt_<session_id>_<timestamp>.pcm (16-bit 8k mono)
		fname := filepath.Join(os.TempDir(), fmt.Sprintf("stt_%s_%d.pcm", uuid.New().String()[:8], time.Now().Unix()))
		if f, err := os.Create(fname); err == nil {
			sess.dumpFile = f
			logging.For("agentsip", "stt").Info().Str("path", fname).Str("format", "pcm_s16le_8000_mono").Msg("dump file")
		}
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
	config Config

	bufMu sync.Mutex
	buf   []byte

	lastHighEnergy   time.Time
	bytesSinceCommit int
	speechSeen       bool

	dumpFile *os.File
}

func (s *session) sendChunk(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	if s.debug {
		logging.For("agentsip", "stt").Debug().Int("bytes", len(b)).Msg("sendChunk")
	}
	// dump to file for offline analysis
	if s.dumpFile != nil {
		_, _ = s.dumpFile.Write(b)
	}
	// Count bytes optimistically before network block — on VPS link to
	// 34.8.184.191 latency is 500ms+, old code only counted after WriteJSON
	// success, so bytesSinceCommit grew 1k/sec not 16k/sec and caller hung
	// up before minCommitBytes reached.
	s.bufMu.Lock()
	s.bytesSinceCommit += len(b)
	s.bufMu.Unlock()

	s.connMu.Lock()
	defer s.connMu.Unlock()
	err := s.conn.WriteJSON(inputAudioChunk{
		MessageType: "input_audio_chunk",
		AudioBase64: base64.StdEncoding.EncodeToString(b),
		Commit:      false,
		SampleRate:  sampleRateHz,
	})
	if err != nil {
		// rollback on failure
		s.bufMu.Lock()
		s.bytesSinceCommit -= len(b)
		if s.bytesSinceCommit < 0 {
			s.bytesSinceCommit = 0
		}
		s.bufMu.Unlock()
	}
	return err
}

func (s *session) sendCommit() error {
	s.bufMu.Lock()
	if s.bytesSinceCommit < s.config.MinCommitBytes {
		if s.debug {
			logging.For("agentsip", "stt").Debug().Int("bytes", s.bytesSinceCommit).Int("need", s.config.MinCommitBytes).Msg("commit skipped, not enough audio")
		}
		s.bufMu.Unlock()
		return nil
	}
	s.bufMu.Unlock()

	if s.debug {
		logging.For("agentsip", "stt").Debug().Int("bytesSinceCommit", s.bytesSinceCommit).Msg("sendCommit manual")
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
	// frame is raw ulaw_8000 from RTP (20ms = 160 bytes). Transcode to pcm_8000.
	pcm := decodeUlawToPCM16(frame)
	energy := pcmEnergy(pcm)
	if energy > s.config.SpeechThreshold {
		s.bufMu.Lock()
		s.lastHighEnergy = time.Now()
		s.speechSeen = true
		s.bufMu.Unlock()
	}

	s.bufMu.Lock()
	s.buf = append(s.buf, pcm...)
	shouldFlush := len(s.buf) >= s.config.FlushBytes
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
	ticker := time.NewTicker(s.config.FlushInterval)
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
			if err := s.sendChunk(toSend); err != nil {
				if s.debug {
					logging.For("agentsip", "stt").Debug().Err(err).Msg("flushLoop send error")
				}
				return
			}
		}

		// Manual commit: if we saw speech and then silence, commit.
		// Need at least 0.3s audio else ElevenLabs returns commit_throttled.
		if hasSpeech && !lastHigh.IsZero() && time.Since(lastHigh) > s.config.ManualCommitSilence {
			if bytesSinceCommit >= s.config.MinCommitBytes {
				s.bufMu.Lock()
				s.lastHighEnergy = time.Time{}
				s.bufMu.Unlock()
				if s.debug {
					logging.For("agentsip", "stt").Debug().Int("bytesSinceCommit", bytesSinceCommit).Msg("commit after silence")
				}
				_ = s.sendCommit()
			} else if s.debug {
				logging.For("agentsip", "stt").Debug().Int("bytesSinceCommit", bytesSinceCommit).Msg("commit delayed, not enough audio")
			}
		}
	}
}

func (s *session) Events() <-chan conversation.TranscriptEvent { return s.events }

func (s *session) Close() error {
	// flush remaining buffered audio before close and force final commit
	s.bufMu.Lock()
	leftover := append([]byte(nil), s.buf...)
	s.buf = s.buf[:0]
	hasSpeech := s.speechSeen
	bytesSince := s.bytesSinceCommit
	s.bufMu.Unlock()

	if len(leftover) > 0 {
		_ = s.sendChunk(leftover)
		time.Sleep(100 * time.Millisecond)
	}

	// Final commit on hangup to ensure last utterance is transcribed,
	// even if silence threshold not yet reached. Only if we saw speech.
	if hasSpeech && bytesSince+len(leftover) >= s.config.MinCommitBytes {
		if s.debug {
			logging.For("agentsip", "stt").Info().Int("bytes", bytesSince+len(leftover)).Msg("final commit on close")
		}
		_ = s.sendCommit()
		time.Sleep(500 * time.Millisecond)
	}

	if s.dumpFile != nil {
		_ = s.dumpFile.Close()
		if s.debug {
			logging.For("agentsip", "stt").Info().Str("path", s.dumpFile.Name()).Msg("dump closed")
		}
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
				logging.For("agentsip", "stt").Info().Err(err).Msg("read error, closing")
			}
			return
		}
		var msg rawServerMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			if s.debug {
				trunc := string(raw)
				if len(trunc) > 200 {
					trunc = trunc[:200]
				}
				logging.For("agentsip", "stt").Debug().Str("raw", trunc).Err(err).Msg("unmarshal fail")
			}
			continue
		}
		switch msg.MessageType {
		case "partial_transcript":
			if s.debug {
				logging.For("agentsip", "stt").Debug().Str("type", msg.MessageType).Str("text", msg.Text).Msg("event")
			}
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: false}
		case "committed_transcript":
			logging.For("agentsip", "stt").Info().Str("type", msg.MessageType).Str("text", msg.Text).Msg("committed_transcript")
			s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: true}
		case "committed_transcript_with_timestamps":
			if msg.Text != "" {
				logging.For("agentsip", "stt").Info().Str("type", msg.MessageType).Str("text", msg.Text).Msg("committed_transcript_with_timestamps")
				s.events <- conversation.TranscriptEvent{Text: msg.Text, Final: true}
			}
		case "session_started":
			logging.For("agentsip", "stt").Info().Str("type", msg.MessageType).Str("text", msg.Text).Msg("session_started")
			continue
		case "input_error", "transcriber_error", "auth_error", "quota_exceeded", "invalid_request", "error", "rate_limited", "queue_overflow", "resource_exhausted", "chunk_size_exceeded", "commit_throttled", "insufficient_audio_activity":
			func() {
				trunc := string(raw)
				if len(trunc) > 500 {
					trunc = trunc[:500]
				}
				logging.For("agentsip", "stt").Error().Str("type", msg.MessageType).Str("error_type", msg.ErrorType).Str("msg", msg.Message).Str("raw", trunc).Msg("server error")
			}()
			continue
		default:
			if s.debug {
				trunc := string(raw)
				if len(trunc) > 200 {
					trunc = trunc[:200]
				}
				logging.For("agentsip", "stt").Debug().Str("type", msg.MessageType).Str("text", msg.Text).Str("raw", trunc).Msg("unknown event")
			}
			continue
		}
	}
}
