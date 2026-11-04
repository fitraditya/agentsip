// Package sip is the SIP/media leg: it accepts an INVITE, resolves it to a
// trunk, negotiates PCMU-only audio, and hands the call's audio to
// usecase/conversation's STT->LLM->TTS pipeline over a real RTP socket (see
// session.go's rtpAudioIO). The SIP/RTP plumbing itself was proven in
// isolation first (an RTP echo in place of the pipeline) before this package
// took on any AI-pipeline dependency.
package sip

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"

	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/call"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
)

type Config struct {
	// ListenAddr is the SIP UDP bind address, e.g. ":5060".
	ListenAddr string
	// AdvertiseHost is the IP/hostname this process is reachable at — used in
	// the Contact header and in every SDP answer's connection address. Must
	// be a real, externally reachable address in any non-loopback deployment;
	// there is deliberately no attempt to auto-detect it (see obrelin's
	// PublicHost for the same reasoning: guessing wrong here misdirects every
	// in-dialog request and every RTP packet).
	AdvertiseHost string
	// RTPBindIP is the interface RTP sockets are opened on. Defaults to
	// "0.0.0.0" when empty.
	RTPBindIP string
	// RTPPortMin/RTPPortMax bound which ports calls get allocated on
	// (RTPPortAllocator), rather than letting the OS pick an ephemeral one
	// per call — a real network's firewall rules want a fixed range to
	// open. Both zero defaults to 16384-32767, matching
	// config.RTPPortMin/RTPPortMax's own default.
	RTPPortMin int
	RTPPortMax int
}

type Server struct {
	cfg          Config
	ua           *sipgo.UserAgent
	srv          *sipgo.Server
	dialogSrv    *sipgo.DialogServerCache
	trunkSvc     trunk.Service
	callSvc      call.Service
	convoSvc     *conversation.Service
	admission    *Admission
	pendingCalls *PendingCalls
	rtpPorts     *RTPPortAllocator
	activeCalls  *ActiveCalls
	draining     atomic.Bool
	log          logging.Logger
}

// Decisions exposes this server's webhook-mode call decisions
// (usecase/call.Decisions) to other adapters — adapter/http's admin API, for
// its accept/reject endpoints — without them depending on this package
// directly.
func (s *Server) Decisions() *PendingCalls {
	return s.pendingCalls
}

func NewServer(cfg Config, trunkSvc trunk.Service, callSvc call.Service, convoSvc *conversation.Service, log logging.Logger) (*Server, error) {
	if cfg.RTPBindIP == "" {
		cfg.RTPBindIP = "0.0.0.0"
	}
	if cfg.RTPPortMin == 0 && cfg.RTPPortMax == 0 {
		cfg.RTPPortMin, cfg.RTPPortMax = 16384, 32767
	}
	rtpPorts, err := NewRTPPortAllocator(cfg.RTPPortMin, cfg.RTPPortMax)
	if err != nil {
		return nil, err
	}

	ua, err := sipgo.NewUA()
	if err != nil {
		return nil, fmt.Errorf("sip: new user agent: %w", err)
	}
	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("sip: new server: %w", err)
	}
	client, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, fmt.Errorf("sip: new client: %w", err)
	}

	// The Contact header's port matters: in-dialog requests (ACK, BYE) are
	// addressed using the Contact we hand back in the 200 OK, not the port
	// the INVITE arrived on. Leaving Port unset silently sends every ACK/BYE
	// to SIP's default port 5060 instead of wherever this server actually
	// listens, which looks like a hung call (the 200 OK is received fine;
	// the dialog just never confirms).
	_, listenPortStr, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("sip: parse listen addr %q: %w", cfg.ListenAddr, err)
	}
	listenPort, err := strconv.Atoi(listenPortStr)
	if err != nil {
		return nil, fmt.Errorf("sip: listen addr %q has a non-numeric port: %w", cfg.ListenAddr, err)
	}

	contact := sip.ContactHeader{Address: sip.Uri{User: "agentsip", Host: cfg.AdvertiseHost, Port: listenPort}}

	s := &Server{
		cfg:          cfg,
		ua:           ua,
		srv:          srv,
		dialogSrv:    sipgo.NewDialogServerCache(client, contact),
		trunkSvc:     trunkSvc,
		callSvc:      callSvc,
		convoSvc:     convoSvc,
		admission:    NewAdmission(),
		pendingCalls: NewPendingCalls(),
		rtpPorts:     rtpPorts,
		activeCalls:  NewActiveCalls(),
		log:          log,
	}

	srv.OnInvite(s.handleInvite)
	srv.OnAck(s.handleAck)
	srv.OnBye(s.handleBye)

	return s, nil
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	return s.srv.ListenAndServe(ctx, "udp", s.cfg.ListenAddr)
}

func (s *Server) Close() error {
	return s.ua.Close()
}

func (s *Server) handleInvite(req *sip.Request, tx sip.ServerTransaction) {
	ctx := context.Background()
	sipCallID := req.CallID().Value()
	log := s.log.With("sip_call_id", sipCallID)

	dlg, err := s.dialogSrv.ReadInvite(req, tx)
	if err != nil {
		log.Error().Err(err).Msg("read invite")
		return
	}

	// Checked as early as possible — before trunk resolution, admission, or
	// any resource allocation — so a draining process costs a refused call
	// nothing beyond this check.
	if s.IsDraining() {
		s.refuseNewCall(dlg)
		return
	}

	sourceIP, _, err := net.SplitHostPort(req.Source())
	if err != nil {
		sourceIP = req.Source()
	}

	t, err := s.trunkSvc.ResolveInbound(ctx, trunk.InboundMatch{SourceIP: sourceIP})
	if err != nil {
		log.Warn().Str("source_ip", sourceIP).Err(err).Msg("no trunk matched inbound call")
		_ = dlg.Respond(sip.StatusForbidden, "Forbidden", nil)
		return
	}
	log = log.With("trunk_id", t.ID)

	// Admission happens before anything else is allocated for this call — no
	// RTP socket, no CDR row, no provider session — so a refused call costs
	// nothing beyond the check itself.
	release, err := s.admission.Admit(t.ID, t.MaxConcurrentCalls)
	if err != nil {
		log.Warn().Int("max_concurrent_calls", t.MaxConcurrentCalls).Err(err).Msg("trunk at capacity")
		// 503, not 486 "Busy Here": 486 would assert a fact about a callee
		// being busy, which is untrue here — there is no human callee, just
		// this trunk being at capacity. 503 is what a carrier/PBX knows how
		// to fail over on.
		_ = dlg.Respond(sip.StatusServiceUnavailable, "Service Unavailable", nil)
		return
	}

	offer, audio, err := parseAudioOffer(req.Body())
	if err != nil {
		log.Warn().Err(err).Msg("unsupported offer")
		release()
		_ = dlg.Respond(sip.StatusNotAcceptableHere, "Not Acceptable Here", nil)
		return
	}

	callerNumber, calleeNumber := "", ""
	if from := req.From(); from != nil {
		callerNumber = from.Address.User
	}
	if to := req.To(); to != nil {
		calleeNumber = to.Address.User
	}

	// The accept/reject decision happens before any media resource is
	// allocated (no RTP socket yet) — same reasoning as admission above, and
	// why this runs ahead of the RTP/CDR work below rather than after it.
	if !s.decideAnswer(dlg, tx, t, sipCallID, callerNumber, calleeNumber, log, release) {
		return
	}

	conn, rtpPort, err := s.bindRTPSocket()
	if err != nil {
		log.Error().Err(err).Msg("allocate rtp socket")
		release()
		_ = dlg.Respond(sip.StatusInternalServerError, "Internal Server Error", nil)
		return
	}

	answer, err := buildAnswer(offer, audio, s.cfg.AdvertiseHost, rtpPort)
	if err != nil {
		log.Warn().Err(err).Msg("build sdp answer")
		release()
		_ = conn.Close()
		s.rtpPorts.Release(rtpPort)
		_ = dlg.Respond(sip.StatusNotAcceptableHere, "Not Acceptable Here", nil)
		return
	}

	if _, err := s.callSvc.RecordAttempt(ctx, t.ID, sipCallID, callerNumber, calleeNumber, domain.CallDirectionInbound); err != nil {
		log.Error().Err(err).Msg("record call attempt")
		release()
		_ = conn.Close()
		s.rtpPorts.Release(rtpPort)
		_ = dlg.Respond(sip.StatusInternalServerError, "Internal Server Error", nil)
		return
	}

	audioIO := newRTPAudioIO(conn, log)

	// Tracked from here (the call is about to be answered) until the dialog
	// ends, so a graceful shutdown's drain can count it and, if needed, hang
	// it up with a BYE.
	removeActive := s.activeCalls.add(sipCallID, dlg)

	answerTime := time.Now().UTC()
	dlg.OnState(func(state sip.DialogState) {
		if state != sip.DialogStateEnded {
			return
		}
		removeActive()
		release()
		audioIO.closeHangup()
		_ = conn.Close()
		s.rtpPorts.Release(rtpPort)
		if err := s.callSvc.RecordEnd(context.Background(), sipCallID, domain.DispositionAnswered, &answerTime); err != nil {
			log.Error().Err(err).Msg("record call end")
		}
	})

	runCfg := conversation.RunConfig{
		SystemPrompt:     t.SystemPrompt,
		STT:              conversation.STTSessionConfig{APIKey: t.STT.APIKey, Model: t.STT.Model},
		TTS:              conversation.TTSSessionConfig{APIKey: t.TTS.APIKey, Model: t.TTS.Model, VoiceID: t.TTS.VoiceID},
		LLM:              conversation.LLMConfig{BaseURL: t.LLM.BaseURL, APIKey: t.LLM.APIKey, Model: t.LLM.Model},
		KnowledgeBaseIDs: t.KnowledgeBaseIDs,
	}
	// Started right after the 200 OK, not gated on the ACK: some UACs send
	// RTP as soon as they receive the final response, before their own ACK
	// arrives, and there is nothing unsafe about reading before that — the
	// pipeline only ever acts in response to audio actually received.
	go func() {
		if err := s.convoSvc.Run(context.Background(), runCfg, audioIO); err != nil {
			log.Warn().Err(err).Msg("conversation pipeline ended")
		}
	}()

	// RespondSDP blocks until the ACK confirms the dialog (sipgo's design —
	// see DialogServerSession.WriteResponse), which is why the pipeline above
	// is already running before this call: the caller's RTP may start
	// flowing as soon as it sees the 200 OK, before its own ACK arrives.
	if err := dlg.RespondSDP(answer); err != nil {
		log.Error().Err(err).Msg("respond with sdp answer")
		// release and removeActive are both safe to call even if the dialog
		// later, improbably, still reaches Ended and fires the OnState
		// callback above — release is idempotent (see Admission.Admit's
		// sync.Once) and removeActive deleting an already-absent key is a
		// no-op.
		removeActive()
		release()
		audioIO.closeHangup()
		_ = conn.Close()
		s.rtpPorts.Release(rtpPort)
		return
	}
	log.Info().Int("rtp_port", rtpPort).Msg("call answered")
}

// bindRTPSocket allocates a port from the configured range and binds it,
// retrying with a fresh port a bounded number of times if the bind itself
// fails (e.g. something outside this allocator's bookkeeping already holds
// it) — a transient collision shouldn't refuse a call the way exhausting the
// whole range should.
const rtpBindAttempts = 5

func (s *Server) bindRTPSocket() (*net.UDPConn, int, error) {
	var lastErr error
	for range rtpBindAttempts {
		port, err := s.rtpPorts.Allocate()
		if err != nil {
			return nil, 0, err
		}
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP(s.cfg.RTPBindIP), Port: port})
		if err == nil {
			return conn, port, nil
		}
		s.rtpPorts.Release(port)
		lastErr = err
	}
	return nil, 0, fmt.Errorf("sip: bind rtp socket after %d attempts: %w", rtpBindAttempts, lastErr)
}

// decideAnswer runs the accept/reject decision for a call that has already
// passed admission control, for whichever AnswerMode the trunk is in. It
// returns true if the caller should proceed to answer the call, and false if
// this call is already fully handled (a response was sent, or — for a
// CANCEL — sipgo already sent one on its own) and handleInvite should return
// immediately. release is only called here on the reject/cancel paths: the
// caller still owns it on the proceed path, same as every other step in
// handleInvite.
func (s *Server) decideAnswer(dlg *sipgo.DialogServerSession, tx sip.ServerTransaction, t *domain.Trunk, sipCallID, callerNumber, calleeNumber string, log logging.Logger, release release) bool {
	switch t.AnswerMode {
	case domain.AnswerModeWebhook:
		pendingCallID, ch, forget := s.pendingCalls.register()
		defer forget()

		_ = dlg.Respond(sip.StatusRinging, "Ringing", nil)

		if err := notifyIncomingCallWebhook(context.Background(), t.WebhookURL, IncomingCallWebhookPayload{
			PendingCallID: pendingCallID.String(),
			TrunkID:       t.ID.String(),
			SIPCallID:     sipCallID,
			CallerNumber:  callerNumber,
			CalleeNumber:  calleeNumber,
		}); err != nil {
			log.Error().Str("webhook_url", t.WebhookURL).Err(err).Msg("deliver incoming call webhook")
			release()
			_ = dlg.Respond(sip.StatusInternalServerError, "Internal Server Error", nil)
			return false
		}

		timeout := time.Duration(t.WebhookTimeoutSeconds) * time.Second
		proceed, rejectStatus, cancelled := waitWebhookDecision(tx, ch, timeout)
		if proceed {
			return true
		}
		release()
		if !cancelled {
			_ = dlg.Respond(rejectStatus, reasonPhrase(rejectStatus), nil)
		}
		return false

	default: // domain.AnswerModeAuto
		ringDelay := time.Duration(t.RingDelaySeconds) * time.Second
		if ringDelay <= 0 {
			return true
		}
		_ = dlg.Respond(sip.StatusRinging, "Ringing", nil)
		if waitRingDelay(tx, ringDelay) {
			return true
		}
		// Cancelled: sipgo's transaction layer already answered the CANCEL
		// with 487 on its own (see sip/transaction_layer.go), so there is
		// nothing left for this handler to respond with.
		release()
		return false
	}
}

func (s *Server) handleAck(req *sip.Request, tx sip.ServerTransaction) {
	if err := s.dialogSrv.ReadAck(req, tx); err != nil {
		s.log.Warn().Str("sip_call_id", req.CallID().Value()).Err(err).Msg("read ack")
	}
}

func (s *Server) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	if err := s.dialogSrv.ReadBye(req, tx); err != nil {
		s.log.Warn().Str("sip_call_id", req.CallID().Value()).Err(err).Msg("read bye")
	}
}
