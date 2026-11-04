// Command engine is agentsip's single binary: the admin REST API, the SIP/RTP
// leg, the STT->LLM->TTS conversation pipeline (ElevenLabs STT/TTS, any
// OpenAI-compatible LLM for v1), and RAG (OpenAI-compatible embeddings,
// pgvector).
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgvectorpgx "github.com/pgvector/pgvector-go/pgx"

	embedderopenaicompat "github.com/fitraditya/agentsip/internal/adapter/embedder/openaicompat"
	adapterhttp "github.com/fitraditya/agentsip/internal/adapter/http"
	llmopenaicompat "github.com/fitraditya/agentsip/internal/adapter/llm/openaicompat"
	"github.com/fitraditya/agentsip/internal/adapter/mail"
	"github.com/fitraditya/agentsip/internal/adapter/repository/postgres"
	adaptersip "github.com/fitraditya/agentsip/internal/adapter/sip"
	"github.com/fitraditya/agentsip/internal/adapter/stt"
	sttelevenlabs "github.com/fitraditya/agentsip/internal/adapter/stt/elevenlabs"
	sttmuse "github.com/fitraditya/agentsip/internal/adapter/stt/muse"
	ttselevenlabs "github.com/fitraditya/agentsip/internal/adapter/tts/elevenlabs"
	vectorstorepgvector "github.com/fitraditya/agentsip/internal/adapter/vectorstore/pgvector"
	"github.com/fitraditya/agentsip/internal/config"
	"github.com/fitraditya/agentsip/internal/pkg/crypto"
	"github.com/fitraditya/agentsip/internal/pkg/logging"
	"github.com/fitraditya/agentsip/internal/usecase/auth"
	"github.com/fitraditya/agentsip/internal/usecase/call"
	"github.com/fitraditya/agentsip/internal/usecase/conversation"
	"github.com/fitraditya/agentsip/internal/usecase/knowledgebase"
	"github.com/fitraditya/agentsip/internal/usecase/llmprovider"
	"github.com/fitraditya/agentsip/internal/usecase/sttprovider"
	"github.com/fitraditya/agentsip/internal/usecase/trunk"
	"github.com/fitraditya/agentsip/internal/usecase/ttsprovider"
)

func main() {
	if err := run(); err != nil {
		logging.For("agentsip", "engine").Fatal().Err(err).Msg("fatal")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logging.Configure(cfg.LogLevel, cfg.LogJSON, logging.ParameterMode(cfg.LogParameters))
	log := logging.For("agentsip", "engine")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	// Sharding, matching obrelin's Decision 9/33: one engine process per
	// database. Refuses to start rather than run a second instance against
	// the state a first instance already owns — Admission's per-trunk
	// counters and PendingCalls' webhook registry are both in-process and
	// would misbehave (double-counted capacity, "not found" webhook
	// decisions) if two instances served the same configuration.
	engineLock, err := postgres.AcquireEngineLock(ctx, cfg.DatabaseURL)
	if err != nil {
		if errors.Is(err, postgres.ErrEngineAlreadyRunning) {
			return fmt.Errorf("refusing to start: %w (one engine process per database)", err)
		}
		return fmt.Errorf("acquiring the engine lock: %w", err)
	}
	defer func() {
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer releaseCancel()
		_ = engineLock.Release(releaseCtx)
	}()

	// Its own pool — see config.VectorStoreDSN's doc comment — even when it
	// resolves to the same Postgres instance as the main pool above.
	vectorPoolCfg, err := pgxpool.ParseConfig(cfg.VectorStoreDSN())
	if err != nil {
		return fmt.Errorf("parse vector store dsn: %w", err)
	}
	vectorPoolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgvectorpgx.RegisterTypes(ctx, conn)
	}
	vectorPool, err := pgxpool.NewWithConfig(ctx, vectorPoolCfg)
	if err != nil {
		return fmt.Errorf("connect vector store postgres: %w", err)
	}
	defer vectorPool.Close()

	fieldKey, err := cfg.FieldEncryptionKey()
	if err != nil {
		return err
	}
	cipher, err := crypto.NewFieldCipher(fieldKey)
	if err != nil {
		return fmt.Errorf("build field cipher: %w", err)
	}

	// Left nil when EmailEnabled is false — auth.Service treats a nil Mailer
	// as "send is best-effort and currently off," not an error. Built as a
	// plain *mail.Sender, not stored in an auth.Mailer-typed var first: an
	// interface holding a nil *Sender is itself non-nil, which would silence
	// that exact fallback.
	var mailer auth.Mailer
	if cfg.EmailEnabled() {
		mailer = mail.NewSender(mail.Config{
			Host:               cfg.SMTPHost,
			Port:               cfg.SMTPPort,
			Username:           cfg.SMTPUsername,
			Password:           cfg.SMTPPassword,
			From:               cfg.SMTPFrom,
			StartTLS:           cfg.SMTPStartTLS,
			InsecureSkipVerify: cfg.SMTPInsecureSkipVerify,
		})
	}
	authSvc := auth.NewService(auth.Deps{
		Users:         postgres.NewUserRepository(pool),
		Sessions:      postgres.NewSessionRepository(pool),
		EmailTokens:   postgres.NewEmailTokenRepository(pool),
		Mailer:        mailer,
		PublicBaseURL: cfg.PublicBaseURL,
	})

	trunkRepo := postgres.NewTrunkRepository(pool, cipher)
	trunkSvc := trunk.NewService(trunkRepo)

	sttProviderSvc := sttprovider.NewService(postgres.NewSTTProviderRepository(pool, cipher))
	llmProviderSvc := llmprovider.NewService(postgres.NewLLMProviderRepository(pool, cipher))
	ttsProviderSvc := ttsprovider.NewService(postgres.NewTTSProviderRepository(pool, cipher))

	callRepo := postgres.NewCallRepository(pool)
	callSvc := call.NewService(callRepo)

	kbRepo := postgres.NewKnowledgeBaseRepository(pool, cipher)
	// Platform-managed by default (vectorPool); a knowledge base configured
	// with its own domain.VectorStoreConfig.DSN gets its own pool instead,
	// opened lazily and tracked here for shutdown.
	vectorStore := vectorstorepgvector.New(vectorPool)
	defer vectorStore.Close()
	kbSvc := knowledgebase.NewService(
		kbRepo,
		knowledgebase.NewSimpleChunker(),
		embedderopenaicompat.New(),
		vectorStore,
	)

	// One adapter instance per port, shared across every trunk: trunk-
	// specific config (API keys, model, voice) flows through per-call
	// *Config/*SessionConfig values, not through adapter construction. There
	// is exactly one provider per port for v1 (ElevenLabs STT/TTS, any
	// OpenAI-compatible LLM).
	convoSvc := conversation.NewService(
		stt.NewRouter(sttelevenlabs.New(), sttmuse.New()),
		ttselevenlabs.New(),
		llmopenaicompat.New(),
		kbSvc,
		log,
	)

	sipServer, err := adaptersip.NewServer(adaptersip.Config{
		ListenAddr:    cfg.SIPListenAddr,
		AdvertiseHost: cfg.SIPAdvertiseHost,
		RTPBindIP:     cfg.RTPBindIP,
		RTPPortMin:    cfg.RTPPortMin,
		RTPPortMax:    cfg.RTPPortMax,
	}, trunkSvc, callSvc, convoSvc, log)
	if err != nil {
		return fmt.Errorf("build sip server: %w", err)
	}
	defer sipServer.Close()

	// CallDecisions backs the webhook-mode accept/reject endpoints — the
	// same registry handleInvite waits on, so a decision made through the
	// HTTP API actually reaches the call waiting on it.
	router := adapterhttp.NewRouter(adapterhttp.Deps{
		Auth:               authSvc,
		Trunk:              trunkSvc,
		KnowledgeBase:      kbSvc,
		STTProvider:        sttProviderSvc,
		LLMProvider:        llmProviderSvc,
		TTSProvider:        ttsProviderSvc,
		Call:               callSvc,
		CallDecisions:      sipServer.Decisions(),
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
	})

	// Seeds exactly one admin user on first boot when the database currently
	// has zero users; otherwise a no-op. Bootstrap itself handles both empty
	// SeedAdminEmail/SeedAdminPassword and a non-empty database silently, so
	// there is no redundant check here.
	if err := authSvc.Bootstrap(ctx, cfg.SeedAdminEmail, cfg.SeedAdminPassword); err != nil {
		return fmt.Errorf("bootstrap seed admin: %w", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// The SIP transport gets its own context, independent of the shutdown
	// signal's ctx: graceful drain (below) needs the transport to stay up
	// while it refuses new calls and waits for active ones to finish, so
	// ListenAndServe must not be torn down the instant a signal arrives.
	sipCtx, sipCancel := context.WithCancel(context.Background())

	errCh := make(chan error, 2)
	go func() {
		log.Info().Str("addr", cfg.HTTPAddr).Msg("http server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http server: %w", err)
		}
	}()
	go func() {
		log.Info().Str("addr", cfg.SIPListenAddr).Msg("sip server listening")
		if err := sipServer.ListenAndServe(sipCtx); err != nil && !errors.Is(err, net.ErrClosed) {
			errCh <- fmt.Errorf("sip server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("shutting down")
	case err := <-errCh:
		sipCancel()
		return err
	}

	// Graceful shutdown, matching obrelin's drain (Decision 56): stop
	// accepting new calls, let the calls already up finish on their own
	// (bounded by ShutdownDrainTimeout), and hang up whatever is left with a
	// BYE rather than silence. The SIP transport is closed only afterwards —
	// every step above needs it up.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), cfg.ShutdownDrainTimeout)
	cutShort := sipServer.Drain(drainCtx)
	drainCancel()
	if cutShort > 0 {
		log.Warn().Int("calls", cutShort).Msg("shutdown ended calls that were still in progress")
	}
	sipCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
