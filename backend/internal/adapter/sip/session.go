package sip

import (
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/pion/rtp"

	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
)

// samplesPerFrame is 20ms of 8kHz u-law audio (one byte per sample) — the
// conventional RTP packetization interval telephony endpoints expect. TTS
// audio arrives in whatever chunk sizes the provider streams it in, so
// rtpAudioIO re-slices it into this frame size before sending, rather than
// forwarding provider chunks as RTP packets directly.
const samplesPerFrame = 160

const frameInterval = 20 * time.Millisecond

// rtpPayloadTypePCMU is RFC 3551's static PCMU assignment, as a wire-format
// value (contrast payloadTypePCMU in sdp.go, which is the same number as the
// SDP format string "0").
const rtpPayloadTypePCMU uint8 = 0

// rtpAudioIO implements usecase/conversation.AudioIO over one call's RTP
// socket. It is the only place in this codebase that turns RTP packets into
// the raw audio bytes usecase/conversation works with and back — everything
// upstream of it (the conversation pipeline, the STT/TTS adapters) is
// transport-agnostic.
//
// Audio in both directions is raw G.711 u-law with no framing: In() delivers
// exactly what was in each received RTP packet's payload, and bytes written
// to Out() are u-law samples, not RTP packets. This works because every piece
// of this pipeline — SIP's PCMU-only SDP answer (sdp.go), ElevenLabs' STT
// audio_format=ulaw_8000, and its TTS output_format=ulaw_8000 — was chosen to
// speak the same format, so nothing in between ever transcodes.
type rtpAudioIO struct {
	conn *net.UDPConn
	log  logging.Logger

	in     chan []byte
	out    chan []byte
	hangup chan struct{}
	once   sync.Once

	remoteMu sync.Mutex
	remote   *net.UDPAddr
}

func newRTPAudioIO(conn *net.UDPConn, log logging.Logger) *rtpAudioIO {
	a := &rtpAudioIO{
		conn:   conn,
		log:    log,
		in:     make(chan []byte, 32),
		out:    make(chan []byte, 32),
		hangup: make(chan struct{}),
	}
	go a.readLoop()
	go a.writeLoop()
	return a
}

var _ conversation.AudioIO = (*rtpAudioIO)(nil)

func (a *rtpAudioIO) In() <-chan []byte       { return a.in }
func (a *rtpAudioIO) Out() chan<- []byte      { return a.out }
func (a *rtpAudioIO) Hangup() <-chan struct{} { return a.hangup }

// closeHangup unblocks usecase/conversation.Service.Run's wait on Hangup().
// Idempotent because both the BYE handler and a transport-level failure can
// each try to tear the call down.
func (a *rtpAudioIO) closeHangup() {
	a.once.Do(func() { close(a.hangup) })
}

func (a *rtpAudioIO) setRemote(addr *net.UDPAddr) {
	a.remoteMu.Lock()
	a.remote = addr
	a.remoteMu.Unlock()
}

func (a *rtpAudioIO) getRemote() *net.UDPAddr {
	a.remoteMu.Lock()
	defer a.remoteMu.Unlock()
	return a.remote
}

// readLoop turns received RTP packets into raw audio frames on In(). Like
// the step-2 echo test this replaces, it learns the remote RTP address from
// where packets actually arrive (symmetric RTP) rather than trusting the
// SDP-declared address, since NAT'd callers often send from a different
// address/port than they offered.
func (a *rtpAudioIO) readLoop() {
	buf := make([]byte, 1500)
	var packetsReceived uint64
	for {
		n, addr, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			a.log.Info().Err(err).Msg("rtp read error, closing hangup")
			a.closeHangup()
			return
		}
		prevRemote := a.getRemote()
		isNewRemote := prevRemote == nil || prevRemote.String() != addr.String()
		a.setRemote(addr)
		if isNewRemote {
			a.log.Info().Str("remote", addr.String()).Msg("rtp learned remote")
		}

		var pkt rtp.Packet
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			a.log.Warn().Err(err).Int("bytes", n).Str("from", addr.String()).Msg("dropping non-RTP packet")
			continue
		}

		packetsReceived++
		// Log first packet and every 50th to avoid spam, plus Debug for every
		if packetsReceived == 1 || packetsReceived%250 == 0 {
			a.log.Info().Int("payload_bytes", len(pkt.Payload)).Int("seq", int(pkt.SequenceNumber)).Int("total", int(packetsReceived)).Str("from", addr.String()).Msg("rtp packet received")
		} else {
			a.log.Debug().Int("payload_bytes", len(pkt.Payload)).Int("seq", int(pkt.SequenceNumber)).Msg("rtp packet received")
		}

		select {
		case a.in <- pkt.Payload:
			a.log.Debug().Int("payload_bytes", len(pkt.Payload)).Msg("rtp forwarded to STT pipeline")
		default:
			// The conversation pipeline (ultimately, an STT websocket) isn't
			// keeping up. Dropping this frame is better than blocking the
			// socket read loop: blocking here would just back up as kernel
			// UDP receive buffer loss instead, with worse timing behavior.
			a.log.Warn().Msg("dropping audio frame: pipeline not keeping up")
		}
	}
}

// writeLoop paces outgoing audio into frameInterval-sized RTP packets. TTS
// providers stream audio in whatever chunk sizes they choose, not
// conveniently pre-split into 20ms frames, so incoming bytes are queued and
// drained on a ticker rather than packetized and sent as soon as they arrive
// — sending bursts of packets back-to-back the moment a provider flushes a
// chunk would overflow the receiving phone's jitter buffer.
func (a *rtpAudioIO) writeLoop() {
	var seq uint16
	var timestamp uint32
	ssrc := rand.Uint32()

	var pending []byte
	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case chunk, ok := <-a.out:
			if !ok {
				return
			}
			pending = append(pending, chunk...)

		case <-ticker.C:
			remote := a.getRemote()
			if remote == nil || len(pending) == 0 {
				continue
			}
			n := min(samplesPerFrame, len(pending))
			frame := pending[:n]
			pending = pending[n:]

			pkt := &rtp.Packet{
				Header: rtp.Header{
					Version:        2,
					PayloadType:    rtpPayloadTypePCMU,
					SequenceNumber: seq,
					Timestamp:      timestamp,
					SSRC:           ssrc,
				},
				Payload: frame,
			}
			seq++
			timestamp += uint32(n)

			raw, err := pkt.Marshal()
			if err != nil {
				a.log.Warn().Err(err).Msg("marshal outgoing rtp packet")
				continue
			}
			if _, err := a.conn.WriteToUDP(raw, remote); err != nil {
				return
			}
		}
	}
}
