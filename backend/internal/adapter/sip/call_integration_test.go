package sip_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo"
	sipmsg "github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	sdpv3 "github.com/pion/sdp/v3"

	adaptersip "github.com/fitraditya/agentsip/internal/adapter/sip"
	"github.com/fitraditya/agentsip/internal/domain"
	"github.com/fitraditya/agentsip/internal/pkg/apperror"
	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/call"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
)

// fakeTrunkRepo and fakeCallRepo back real trunk.Service/call.Service
// instances, so this test exercises the same usecase code the production
// binary runs, with only the Postgres layer swapped for memory.

type fakeTrunkRepo struct {
	t *domain.Trunk
}

func (r *fakeTrunkRepo) Create(context.Context, *domain.Trunk) error { return nil }
func (r *fakeTrunkRepo) Get(_ context.Context, _ uuid.UUID) (*domain.Trunk, error) {
	return r.t, nil
}
func (r *fakeTrunkRepo) List(context.Context) ([]*domain.Trunk, error) { return nil, nil }
func (r *fakeTrunkRepo) Count(context.Context) (int, error)            { return 0, nil }
func (r *fakeTrunkRepo) Update(context.Context, *domain.Trunk) error   { return nil }
func (r *fakeTrunkRepo) Delete(context.Context, uuid.UUID) error       { return nil }
func (r *fakeTrunkRepo) FindByInboundMatch(_ context.Context, match trunk.InboundMatch) (*domain.Trunk, error) {
	if slices.Contains(r.t.SIPAuth.AllowedIPs, match.SourceIP) {
		return r.t, nil
	}
	return nil, errNotFound
}

var errNotFound = &notFoundErr{}

type notFoundErr struct{}

func (*notFoundErr) Error() string { return "not found" }

type fakeCallRepo struct {
	mu    sync.Mutex
	calls map[string]*domain.CallRecord
}

func newFakeCallRepo() *fakeCallRepo {
	return &fakeCallRepo{calls: map[string]*domain.CallRecord{}}
}

func (r *fakeCallRepo) Create(_ context.Context, c *domain.CallRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[c.SIPCallID] = c
	return nil
}
func (r *fakeCallRepo) GetByID(context.Context, uuid.UUID) (*domain.CallRecord, error) {
	return nil, errNotFound
}
func (r *fakeCallRepo) GetBySIPCallID(_ context.Context, sipCallID string) (*domain.CallRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.calls[sipCallID]
	if !ok {
		return nil, errNotFound
	}
	return c, nil
}
func (r *fakeCallRepo) List(context.Context, call.Page) ([]*domain.CallRecord, error) {
	return nil, nil
}
func (r *fakeCallRepo) UpdateDisposition(_ context.Context, id uuid.UUID, disposition domain.CallDisposition, answerTime, endTime *time.Time, durationSeconds int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.calls {
		if c.ID == id {
			c.Disposition = disposition
			c.AnswerTime = answerTime
			c.EndTime = endTime
			c.DurationSeconds = durationSeconds
			return nil
		}
	}
	return errNotFound
}

// fakeSTT/fakeTTS/fakeLLM stand in for the ElevenLabs/OpenAI-compatible
// adapters, so this test proves the SIP<->RTP<->conversation wiring (the
// thing this package owns) without touching a real network. The pipeline's
// own turn-taking/barge-in logic is covered separately by
// internal/usecase/conversation's own tests — this test's job is narrower:
// does a received RTP frame actually reach STT, and does TTS-produced audio
// actually come back out as RTP.

type fakeSTT struct {
	events      chan conversation.TranscriptEvent
	audioFrames chan []byte
}

func (f *fakeSTT) StartSession(context.Context, conversation.STTSessionConfig) (conversation.STTSession, error) {
	return &fakeSTTSession{f}, nil
}

type fakeSTTSession struct{ f *fakeSTT }

func (s *fakeSTTSession) SendAudio(frame []byte) error {
	cp := append([]byte(nil), frame...)
	select {
	case s.f.audioFrames <- cp:
	default:
	}
	return nil
}
func (s *fakeSTTSession) Events() <-chan conversation.TranscriptEvent { return s.f.events }
func (s *fakeSTTSession) Close() error                                { return nil }

type fakeTTS struct {
	started      chan struct{}
	started_once sync.Once
	audio        chan []byte
	sentMu       sync.Mutex
	sent         []string
}

func (f *fakeTTS) StartSession(context.Context, conversation.TTSSessionConfig) (conversation.TTSSession, error) {
	f.started_once.Do(func() { close(f.started) })
	return &fakeTTSSession{f}, nil
}

type fakeTTSSession struct{ f *fakeTTS }

func (s *fakeTTSSession) SendText(chunk string, flush bool) error {
	s.f.sentMu.Lock()
	s.f.sent = append(s.f.sent, chunk)
	s.f.sentMu.Unlock()
	return nil
}
func (s *fakeTTSSession) Audio() <-chan []byte { return s.f.audio }
func (s *fakeTTSSession) Cancel() error        { return nil }
func (s *fakeTTSSession) Close() error         { return nil }

type fakeLLM struct {
	proceed chan struct{}
}

func (f *fakeLLM) StreamChat(ctx context.Context, cfg conversation.LLMConfig, messages []conversation.LLMMessage, tools []conversation.ToolSpec) (<-chan conversation.LLMDelta, error) {
	ch := make(chan conversation.LLMDelta, 1)
	go func() {
		select {
		case <-f.proceed:
		case <-ctx.Done():
		}
		ch <- conversation.LLMDelta{Done: true}
		close(ch)
	}()
	return ch, nil
}

// TestCallRoundTrip proves the full wiring this and the previous build slice
// set out to validate: a real INVITE negotiates a real SDP answer; audio
// received over RTP reaches the STT adapter; a final transcript drives the
// conversation pipeline; audio the TTS adapter produces is sent back out as
// real RTP packets; and a BYE finalizes the call record as ANSWERED. STT,
// TTS, and the LLM are fakes — the SIP/RTP plumbing and the
// usecase/conversation wiring between them are real.
func TestCallRoundTrip(t *testing.T) {
	const listenAddr = "127.0.0.1:26061"

	trunkID := uuid.New()
	fakeTrunk := &fakeTrunkRepo{t: &domain.Trunk{
		ID:      trunkID,
		SIPAuth: domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:     domain.TTSConfig{VoiceID: "test-voice"},
	}}
	callRepo := newFakeCallRepo()

	trunkSvc := trunk.NewService(fakeTrunk)
	callSvc := call.NewService(callRepo)

	stt := &fakeSTT{events: make(chan conversation.TranscriptEvent, 4), audioFrames: make(chan []byte, 16)}
	tts := &fakeTTS{started: make(chan struct{}), audio: make(chan []byte, 4)}
	llm := &fakeLLM{proceed: make(chan struct{})}
	convoSvc := conversation.NewService(stt, tts, llm, nil, logging.Nop())

	log := logging.Nop()
	srv, err := adaptersip.NewServer(adaptersip.Config{
		ListenAddr:    listenAddr,
		AdvertiseHost: "127.0.0.1",
		RTPBindIP:     "127.0.0.1",
	}, trunkSvc, callSvc, convoSvc, log)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	serverReady := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, sipgo.ListenReadyCtxKey, sipgo.ListenReadyCtxValue(serverReady))

	go func() {
		if err := srv.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			t.Logf("sip server stopped: %v", err)
		}
	}()
	select {
	case <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("sip server never started listening")
	}

	clientRTP, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen client rtp: %v", err)
	}
	defer clientRTP.Close()
	clientRTPPort := clientRTP.LocalAddr().(*net.UDPAddr).Port

	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatalf("new ua: %v", err)
	}
	defer ua.Close()
	client, err := sipgo.NewClient(ua)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	contact := sipmsg.ContactHeader{Address: sipmsg.Uri{User: "caller", Host: "127.0.0.1"}}
	dialogClient := sipgo.NewDialogClientCache(client, contact)

	offer := buildTestOffer(clientRTPPort)
	recipient := sipmsg.Uri{User: "agent", Host: "127.0.0.1"}

	inviteReq := sipmsg.NewRequest(sipmsg.INVITE, recipient)
	inviteReq.SetBody(offer)
	inviteReq.AppendHeader(sipmsg.NewHeader("Content-Type", "application/sdp"))
	inviteReq.SetDestination(listenAddr)

	session, err := dialogClient.WriteInvite(ctx, inviteReq)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	if err := session.WaitAnswer(ctx, sipgo.AnswerOptions{}); err != nil {
		t.Fatalf("wait answer: %v", err)
	}
	if session.InviteResponse.StatusCode != sipmsg.StatusOK {
		t.Fatalf("expected 200 OK, got %d", session.InviteResponse.StatusCode)
	}
	if err := session.Ack(ctx); err != nil {
		t.Fatalf("ack: %v", err)
	}

	answerIP, answerPort := parseTestAnswer(t, session.InviteResponse.Body())
	if answerIP != "127.0.0.1" {
		t.Fatalf("expected answer address 127.0.0.1, got %s", answerIP)
	}

	// Direction 1: caller -> STT. A real RTP packet sent to the negotiated
	// port should have its payload handed to the (fake) STT session.
	// Payload must be >=40 bytes (session.go drops small CNG packets).
	testPayload := []byte("user-said-this-160bytes-payload--------------------------------------" + "----------------------------------------------------------------")
	sent := &rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 0, SequenceNumber: 1, Timestamp: 160, SSRC: 0xCAFE},
		Payload: testPayload,
	}
	raw, err := sent.Marshal()
	if err != nil {
		t.Fatalf("marshal rtp: %v", err)
	}
	if _, err := clientRTP.WriteToUDP(raw, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: answerPort}); err != nil {
		t.Fatalf("send rtp: %v", err)
	}
	select {
	case got := <-stt.audioFrames:
		if string(got) != string(testPayload) {
			t.Fatalf("stt received payload %q, want %q", got, testPayload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stt never received the rtp payload")
	}

	// Drive a final transcript, which starts an assistant turn.
	stt.events <- conversation.TranscriptEvent{Text: "hello agent", Final: true}
	select {
	case <-tts.started:
	case <-time.After(2 * time.Second):
		t.Fatal("tts session was never started for the turn")
	}

	// Direction 2: TTS -> caller. Audio the (fake) TTS session produces
	// should arrive at the caller as a real RTP packet.
	tts.audio <- []byte("assistant-audio")
	_ = clientRTP.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1500)
	n, _, err := clientRTP.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read rtp from server: %v", err)
	}
	var got rtp.Packet
	if err := got.Unmarshal(buf[:n]); err != nil {
		t.Fatalf("unmarshal rtp from server: %v", err)
	}
	if got.PayloadType != 0 {
		t.Fatalf("payload type = %d, want 0 (PCMU)", got.PayloadType)
	}
	if string(got.Payload) != "assistant-audio" {
		t.Fatalf("payload = %q, want %q", got.Payload, "assistant-audio")
	}

	close(llm.proceed) // let the fake LLM finish the turn

	rec, err := callRepo.GetBySIPCallID(ctx, session.InviteRequest.CallID().Value())
	if err != nil {
		t.Fatalf("call record not found: %v", err)
	}
	if rec.Disposition != domain.DispositionInProgress {
		t.Fatalf("expected IN_PROGRESS before bye, got %s", rec.Disposition)
	}

	if err := session.Bye(ctx); err != nil {
		t.Fatalf("bye: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		rec, err = callRepo.GetBySIPCallID(ctx, session.InviteRequest.CallID().Value())
		if err == nil && rec.Disposition == domain.DispositionAnswered {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("call was not finalized as ANSWERED after bye, last state: %+v", rec)
}

func buildTestOffer(rtpPort int) []byte {
	sd := &sdpv3.SessionDescription{
		Origin: sdpv3.Origin{
			Username: "-", SessionID: 1, SessionVersion: 1,
			NetworkType: "IN", AddressType: "IP4", UnicastAddress: "127.0.0.1",
		},
		SessionName: "agentsip-test",
		ConnectionInformation: &sdpv3.ConnectionInformation{
			NetworkType: "IN", AddressType: "IP4", Address: &sdpv3.Address{Address: "127.0.0.1"},
		},
		TimeDescriptions: []sdpv3.TimeDescription{{}},
		MediaDescriptions: []*sdpv3.MediaDescription{{
			MediaName: sdpv3.MediaName{
				Media: "audio", Port: sdpv3.RangedPort{Value: rtpPort},
				Protos: []string{"RTP", "AVP"}, Formats: []string{"0"},
			},
			Attributes: []sdpv3.Attribute{sdpv3.NewAttribute("rtpmap", "0 PCMU/8000")},
		}},
	}
	out, err := sd.Marshal()
	if err != nil {
		panic(err)
	}
	return out
}

func parseTestAnswer(t *testing.T, body []byte) (ip string, port int) {
	t.Helper()
	var sd sdpv3.SessionDescription
	if err := sd.Unmarshal(body); err != nil {
		t.Fatalf("parse answer sdp: %v", err)
	}
	if len(sd.MediaDescriptions) != 1 {
		t.Fatalf("expected exactly one media description in answer, got %d", len(sd.MediaDescriptions))
	}
	return sd.ConnectionInformation.Address.Address, sd.MediaDescriptions[0].MediaName.Port.Value
}

// dialTestCall sends a real INVITE/ACK to listenAddr and returns the
// established dialog session. The caller is responsible for ending it (Bye).
func dialTestCall(t *testing.T, ctx context.Context, dialogClient *sipgo.DialogClientCache, listenAddr string) *sipgo.DialogClientSession {
	t.Helper()

	clientRTP, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("listen client rtp: %v", err)
	}
	t.Cleanup(func() { clientRTP.Close() })
	clientRTPPort := clientRTP.LocalAddr().(*net.UDPAddr).Port

	inviteReq := sipmsg.NewRequest(sipmsg.INVITE, sipmsg.Uri{User: "agent", Host: "127.0.0.1"})
	inviteReq.SetBody(buildTestOffer(clientRTPPort))
	inviteReq.AppendHeader(sipmsg.NewHeader("Content-Type", "application/sdp"))
	inviteReq.SetDestination(listenAddr)

	session, err := dialogClient.WriteInvite(ctx, inviteReq)
	if err != nil {
		t.Fatalf("invite: %v", err)
	}
	return session
}

// TestCallRejectedWhenTrunkAtCapacity proves admission control actually runs
// on the SIP path, not just in Admission's own unit tests: a trunk configured
// for one concurrent call accepts the first INVITE and rejects a second,
// concurrent one with 503 — not 486 "Busy Here", since there is no callee to
// be busy, only this trunk being at capacity (see server.go's handleInvite).
func TestCallRejectedWhenTrunkAtCapacity(t *testing.T) {
	const listenAddr = "127.0.0.1:26062"

	fakeTrunk := &fakeTrunkRepo{t: &domain.Trunk{
		ID:                 uuid.New(),
		SIPAuth:            domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:                domain.TTSConfig{VoiceID: "test-voice"},
		MaxConcurrentCalls: 1,
	}}
	trunkSvc := trunk.NewService(fakeTrunk)
	callSvc := call.NewService(newFakeCallRepo())

	stt := &fakeSTT{events: make(chan conversation.TranscriptEvent, 4), audioFrames: make(chan []byte, 16)}
	tts := &fakeTTS{started: make(chan struct{}), audio: make(chan []byte, 4)}
	llm := &fakeLLM{proceed: make(chan struct{})}
	convoSvc := conversation.NewService(stt, tts, llm, nil, logging.Nop())

	log := logging.Nop()
	srv, err := adaptersip.NewServer(adaptersip.Config{
		ListenAddr:    listenAddr,
		AdvertiseHost: "127.0.0.1",
		RTPBindIP:     "127.0.0.1",
	}, trunkSvc, callSvc, convoSvc, log)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	serverReady := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = context.WithValue(ctx, sipgo.ListenReadyCtxKey, sipgo.ListenReadyCtxValue(serverReady))

	go func() {
		if err := srv.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			t.Logf("sip server stopped: %v", err)
		}
	}()
	select {
	case <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("sip server never started listening")
	}

	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatalf("new ua: %v", err)
	}
	defer ua.Close()
	client, err := sipgo.NewClient(ua)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	contact := sipmsg.ContactHeader{Address: sipmsg.Uri{User: "caller", Host: "127.0.0.1"}}
	dialogClient := sipgo.NewDialogClientCache(client, contact)

	first := dialTestCall(t, ctx, dialogClient, listenAddr)
	if err := first.WaitAnswer(ctx, sipgo.AnswerOptions{}); err != nil {
		t.Fatalf("first call wait answer: %v", err)
	}
	if first.InviteResponse.StatusCode != sipmsg.StatusOK {
		t.Fatalf("first call expected 200 OK, got %d", first.InviteResponse.StatusCode)
	}
	if err := first.Ack(ctx); err != nil {
		t.Fatalf("first call ack: %v", err)
	}
	defer first.Bye(ctx)

	second := dialTestCall(t, ctx, dialogClient, listenAddr)
	err = second.WaitAnswer(ctx, sipgo.AnswerOptions{})
	if err == nil {
		t.Fatalf("expected the second call to be refused, got %d", second.InviteResponse.StatusCode)
	}
	var dialogErr *sipgo.ErrDialogResponse
	if !errors.As(err, &dialogErr) {
		t.Fatalf("expected an ErrDialogResponse, got %v (%T)", err, err)
	}
	if dialogErr.Res.StatusCode != sipmsg.StatusServiceUnavailable {
		t.Fatalf("second call status = %d, want %d (Service Unavailable)", dialogErr.Res.StatusCode, sipmsg.StatusServiceUnavailable)
	}
}

// testServer bundles one running sip.Server with everything a test needs to
// dial it and inspect what happened.
type testServer struct {
	dialogClient *sipgo.DialogClientCache
	listenAddr   string
	callRepo     *fakeCallRepo
	stt          *fakeSTT
	tts          *fakeTTS
	llm          *fakeLLM
	decisions    *adaptersip.PendingCalls
}

// startTestServer builds and starts a real sip.Server backed entirely by
// fakes (trunk/call repos, STT/TTS/LLM), for tests that need more than one
// scenario's worth of setup repeated. listenAddr must be unique per test so
// parallel/sequential test runs don't collide on the same UDP port.
func startTestServer(t *testing.T, listenAddr string, tr *domain.Trunk) *testServer {
	t.Helper()

	trunkSvc := trunk.NewService(&fakeTrunkRepo{t: tr})
	callRepo := newFakeCallRepo()
	callSvc := call.NewService(callRepo)

	stt := &fakeSTT{events: make(chan conversation.TranscriptEvent, 4), audioFrames: make(chan []byte, 16)}
	tts := &fakeTTS{started: make(chan struct{}), audio: make(chan []byte, 4)}
	llm := &fakeLLM{proceed: make(chan struct{})}
	convoSvc := conversation.NewService(stt, tts, llm, nil, logging.Nop())

	log := logging.Nop()
	srv, err := adaptersip.NewServer(adaptersip.Config{
		ListenAddr:    listenAddr,
		AdvertiseHost: "127.0.0.1",
		RTPBindIP:     "127.0.0.1",
	}, trunkSvc, callSvc, convoSvc, log)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	t.Cleanup(func() { srv.Close() })

	serverReady := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	ctx = context.WithValue(ctx, sipgo.ListenReadyCtxKey, sipgo.ListenReadyCtxValue(serverReady))

	go func() {
		if err := srv.ListenAndServe(ctx); err != nil && ctx.Err() == nil {
			t.Logf("sip server stopped: %v", err)
		}
	}()
	select {
	case <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("sip server never started listening")
	}

	ua, err := sipgo.NewUA()
	if err != nil {
		t.Fatalf("new ua: %v", err)
	}
	t.Cleanup(func() { ua.Close() })
	client, err := sipgo.NewClient(ua)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	contact := sipmsg.ContactHeader{Address: sipmsg.Uri{User: "caller", Host: "127.0.0.1"}}

	return &testServer{
		dialogClient: sipgo.NewDialogClientCache(client, contact),
		listenAddr:   listenAddr,
		callRepo:     callRepo,
		stt:          stt,
		tts:          tts,
		llm:          llm,
		decisions:    srv.Decisions(),
	}
}

// TestCallRingDelayDelaysAnswerAndSendsRinging proves a trunk's
// RingDelaySeconds actually delays the final response and that a 180
// Ringing is sent first — the "simulate normal waiting time" behavior.
func TestCallRingDelayDelaysAnswerAndSendsRinging(t *testing.T) {
	ts := startTestServer(t, "127.0.0.1:26063", &domain.Trunk{
		ID:                 uuid.New(),
		SIPAuth:            domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:                domain.TTSConfig{VoiceID: "test-voice"},
		MaxConcurrentCalls: 1,
		AnswerMode:         domain.AnswerModeAuto,
		RingDelaySeconds:   1,
	})

	ctx := t.Context()
	session := dialTestCall(t, ctx, ts.dialogClient, ts.listenAddr)

	var sawRinging bool
	start := time.Now()
	err := session.WaitAnswer(ctx, sipgo.AnswerOptions{OnResponse: func(res *sipmsg.Response) error {
		if res.StatusCode == sipmsg.StatusRinging {
			sawRinging = true
		}
		return nil
	}})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("wait answer: %v", err)
	}
	if !sawRinging {
		t.Fatal("expected a 180 Ringing before the final response")
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("answered after %v, want at least ~1s (ring_delay_seconds)", elapsed)
	}
	_ = session.Ack(ctx)
	_ = session.Bye(ctx)
}

// TestCallWebhookAcceptAnswersTheCall proves the webhook accept/reject path
// wires all the way from the HTTP-facing usecase/call.Decisions port
// (here called directly, as adapter/http's handler would call it) through to
// an actual 200 OK on the SIP side.
func TestCallWebhookAcceptAnswersTheCall(t *testing.T) {
	var webhookBody adaptersip.IncomingCallWebhookPayload
	webhookReceived := make(chan struct{})
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&webhookBody)
		close(webhookReceived)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	ts := startTestServer(t, "127.0.0.1:26064", &domain.Trunk{
		ID:                    uuid.New(),
		SIPAuth:               domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:                   domain.TTSConfig{VoiceID: "test-voice"},
		MaxConcurrentCalls:    1,
		AnswerMode:            domain.AnswerModeWebhook,
		WebhookURL:            webhookSrv.URL,
		WebhookTimeoutSeconds: 10,
	})

	ctx := t.Context()
	session := dialTestCall(t, ctx, ts.dialogClient, ts.listenAddr)

	select {
	case <-webhookReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook was never delivered")
	}
	pendingCallID, err := uuid.Parse(webhookBody.PendingCallID)
	if err != nil {
		t.Fatalf("webhook payload had no valid pending_call_id: %+v", webhookBody)
	}
	if err := ts.decisions.Accept(pendingCallID); err != nil {
		t.Fatalf("accept: %v", err)
	}

	if err := session.WaitAnswer(ctx, sipgo.AnswerOptions{}); err != nil {
		t.Fatalf("wait answer: %v", err)
	}
	if session.InviteResponse.StatusCode != sipmsg.StatusOK {
		t.Fatalf("expected 200 OK after accept, got %d", session.InviteResponse.StatusCode)
	}
	_ = session.Ack(ctx)
	_ = session.Bye(ctx)

	// A second decision on the same call must be refused — the first one
	// already resolved it.
	if err := ts.decisions.Reject(pendingCallID, 0); !errors.Is(err, apperror.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for a decision on an already-resolved call, got %v", err)
	}
}

// TestCallWebhookRejectsExplicitly proves an explicit Reject call delivers
// the requested status code to the caller.
func TestCallWebhookRejectsExplicitly(t *testing.T) {
	var webhookBody adaptersip.IncomingCallWebhookPayload
	webhookReceived := make(chan struct{})
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&webhookBody)
		close(webhookReceived)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	ts := startTestServer(t, "127.0.0.1:26065", &domain.Trunk{
		ID:                    uuid.New(),
		SIPAuth:               domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:                   domain.TTSConfig{VoiceID: "test-voice"},
		MaxConcurrentCalls:    1,
		AnswerMode:            domain.AnswerModeWebhook,
		WebhookURL:            webhookSrv.URL,
		WebhookTimeoutSeconds: 10,
	})

	ctx := t.Context()
	session := dialTestCall(t, ctx, ts.dialogClient, ts.listenAddr)

	select {
	case <-webhookReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook was never delivered")
	}
	pendingCallID, err := uuid.Parse(webhookBody.PendingCallID)
	if err != nil {
		t.Fatalf("webhook payload had no valid pending_call_id: %+v", webhookBody)
	}

	if err := ts.decisions.Reject(pendingCallID, 603); err != nil {
		t.Fatalf("reject: %v", err)
	}

	err = session.WaitAnswer(ctx, sipgo.AnswerOptions{})
	var dialogErr *sipgo.ErrDialogResponse
	if !errors.As(err, &dialogErr) {
		t.Fatalf("expected an ErrDialogResponse, got %v (%T)", err, err)
	}
	if dialogErr.Res.StatusCode != 603 {
		t.Fatalf("status = %d, want 603 (the explicit reject code)", dialogErr.Res.StatusCode)
	}
}

// TestCallWebhookTimesOutAndRejects proves a webhook-mode call that nobody
// ever decides on is rejected automatically once WebhookTimeoutSeconds
// elapses — the behavior OpenAI's own docs leave unspecified, so it's worth
// pinning down here.
func TestCallWebhookTimesOutAndRejects(t *testing.T) {
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	ts := startTestServer(t, "127.0.0.1:26066", &domain.Trunk{
		ID:                    uuid.New(),
		SIPAuth:               domain.SIPAuth{AllowedIPs: []string{"127.0.0.1"}},
		TTS:                   domain.TTSConfig{VoiceID: "test-voice"},
		MaxConcurrentCalls:    1,
		AnswerMode:            domain.AnswerModeWebhook,
		WebhookURL:            webhookSrv.URL,
		WebhookTimeoutSeconds: 1,
	})

	ctx := t.Context()
	session := dialTestCall(t, ctx, ts.dialogClient, ts.listenAddr)

	deadline := time.After(5 * time.Second)
	done := make(chan error, 1)
	go func() { done <- session.WaitAnswer(ctx, sipgo.AnswerOptions{}) }()

	select {
	case err := <-done:
		var dialogErr *sipgo.ErrDialogResponse
		if !errors.As(err, &dialogErr) {
			t.Fatalf("expected an ErrDialogResponse, got %v (%T)", err, err)
		}
		if dialogErr.Res.StatusCode != 480 {
			t.Fatalf("status = %d, want 480 (Temporarily Unavailable, the timeout default)", dialogErr.Res.StatusCode)
		}
	case <-deadline:
		t.Fatal("call was never rejected after the webhook timeout elapsed")
	}
}

// NOTE: a CANCEL-during-ring-delay test (hanging up while waitRingDelay is
// sleeping) was attempted here and deliberately removed. It reproducibly
// deadlocked inside sipgo's *client*-side transaction FSM
// (sip.(*baseTx).spinFsmWithResponse), used only as the test's simulated
// caller — our own code (decision.go's waitRingDelay/tx.OnCancel) never
// appears in that deadlock's stack. That's a sipgo client-library
// concurrency issue to chase separately, not evidence against the
// server-side OnCancel wiring, which otherwise follows sipgo's documented
// API and obrelin's own precedent for it. Flagging the gap rather than
// claiming coverage this suite doesn't actually have.
