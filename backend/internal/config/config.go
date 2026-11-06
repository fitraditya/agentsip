// Package config loads process configuration from the environment.
package config

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	DatabaseURL string `env:"DATABASE_URL,required"`

	// CORSAllowedOrigins restricts which browser origins may call the admin
	// API cross-origin (e.g. a web frontend served from a different
	// host/port) — see middleware.CORS's doc comment. Comma-separated;
	// unset/empty allows any origin, which is safe here specifically
	// because every route authenticates via an Authorization: Bearer
	// header, never a cookie, so there is nothing for a third-party page to
	// ride along on.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`

	// FieldEncryptionKeyB64 is a base64-encoded 32-byte AES-256 key used to
	// encrypt trunk secrets (provider API keys, SIP digest passwords) at
	// rest. Required — there is no plaintext fallback, since that would
	// silently store live credentials unencrypted.
	FieldEncryptionKeyB64 string `env:"FIELD_ENCRYPTION_KEY,required"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	// LogJSON emits structured JSON rather than human-readable text. On by
	// default — matches obrelin's default and is what a log aggregator wants.
	LogJSON bool `env:"LOG_JSON" envDefault:"true"`
	// LogParameters controls phone-number masking in logs (see
	// internal/pkg/logging's doc comment) — "off" (default), "masked", or
	// "full".
	LogParameters string `env:"LOG_PARAMETERS" envDefault:"off"`

	// Debug logs every pipeline step (STT transcript, LLM deltas, audio
	// received from websockets) at INFO — for diagnosing why a call has no
	// audio. Never enable in production: it prints transcripts and response
	// text to the log.
	Debug bool `env:"AGENTSIP_DEBUG" envDefault:"false"`

	SIPListenAddr    string `env:"SIP_LISTEN_ADDR" envDefault:":5060"`
	SIPAdvertiseHost string `env:"SIP_ADVERTISE_HOST,required"`
	RTPBindIP        string `env:"RTP_BIND_IP" envDefault:"0.0.0.0"`
	// RTPPortMin/RTPPortMax bound the RTP sockets a call can be allocated on
	// (internal/adapter/sip.RTPPortAllocator), rather than letting the OS
	// pick an ephemeral port per call — same reasoning and same defaults as
	// obrelin's MEDIA_PORT_MIN/MEDIA_PORT_MAX: a real network's firewall
	// rules want a fixed range to open, not "whatever the kernel handed out
	// this time."
	RTPPortMin int `env:"RTP_PORT_MIN" envDefault:"16384"`
	RTPPortMax int `env:"RTP_PORT_MAX" envDefault:"32767"`

	// VectorStoreDatabaseURL is the Postgres instance the pgvector adapter
	// connects to, on its own connection pool so a burst of knowledge-base
	// queries can never exhaust the pool the SIP-facing trunk/call path
	// depends on. Empty means "use DatabaseURL" — same database, still a
	// separate pool — but it can point at an entirely different Postgres
	// instance (or, later, a different VectorStore adapter altogether)
	// without any other config changing.
	VectorStoreDatabaseURL string `env:"VECTORSTORE_DATABASE_URL"`

	// SeedAdminEmail/SeedAdminPassword seed exactly one admin user on first
	// boot, if and only if the database currently has zero users — they are
	// otherwise ignored (including on every subsequent boot once any user
	// exists). Both are optional: leaving either unset simply skips seeding,
	// since there is no platform-admin route left to bootstrap an operator
	// into once this deployment already has at least one user.
	SeedAdminEmail    string `env:"SEED_ADMIN_EMAIL"`
	SeedAdminPassword string `env:"SEED_ADMIN_PASSWORD"`

	// ShutdownDrainTimeout is how long a shutdown waits for calls already in
	// progress to end before hanging up whatever is left (matching obrelin's
	// Decision 56). Zero skips the wait entirely and drops every live call.
	//
	// 30 seconds is the same compromise obrelin makes: long enough that most
	// short calls finish, far short of a long one — an unbounded wait would
	// let one forgotten call hold a deploy indefinitely, which in practice
	// teaches operators to skip the drain.
	ShutdownDrainTimeout time.Duration `env:"SHUTDOWN_DRAIN_TIMEOUT" envDefault:"30s"`

	// SMTP sends the email-verification and password-reset emails
	// (usecase/auth). An empty host disables sending entirely — registration
	// and password reset still work without it, they just don't email a
	// link; see internal/adapter/mail's doc comment. Matches obrelin's own
	// SMTP_* variables field-for-field, voicemail delivery's reasoning
	// applies here unchanged.
	//
	// SMTPStartTLS defaults ON. Sending a password and a login link in clear
	// text across a network should be a choice an operator makes
	// deliberately, not one they get by leaving a variable unset.
	SMTPHost     string `env:"SMTP_HOST"`
	SMTPPort     int    `env:"SMTP_PORT" envDefault:"587"`
	SMTPUsername string `env:"SMTP_USERNAME"`
	SMTPPassword string `env:"SMTP_PASSWORD"`
	SMTPFrom     string `env:"SMTP_FROM"`
	SMTPStartTLS bool   `env:"SMTP_STARTTLS" envDefault:"true"`
	// SMTPInsecureSkipVerify exists for a self-hosted relay with a private
	// certificate authority. Named the way it is so nobody sets it by
	// accident.
	SMTPInsecureSkipVerify bool `env:"SMTP_INSECURE_SKIP_VERIFY"`

	// PublicBaseURL, if set, is prefixed to email-verification/password-reset
	// tokens to build a clickable link (e.g. PublicBaseURL + "/verify-email
	// ?token=..."), pointing at whatever web frontend handles those pages.
	// Empty sends the raw token in the email instead — the API endpoints
	// behave identically either way, this only changes what the email looks
	// like.
	PublicBaseURL string `env:"PUBLIC_BASE_URL"`

	// STT tuning for high-latency VPS (Indonesia -> US). Defaults are for
	// APAC high-latency; US VPS can use larger values for better accuracy.
	STTFlushBytes          int           `env:"STT_FLUSH_BYTES" envDefault:"320"`
	STTFlushInterval       time.Duration `env:"STT_FLUSH_INTERVAL" envDefault:"80ms"`
	STTMinCommitBytes      int           `env:"STT_MIN_COMMIT_BYTES" envDefault:"640"`
	STTManualCommitSilence time.Duration `env:"STT_COMMIT_SILENCE" envDefault:"500ms"`
	STTSpeechThreshold     int           `env:"STT_SPEECH_THRESHOLD" envDefault:"150"`
}

// VectorStoreDSN returns VectorStoreDatabaseURL, falling back to DatabaseURL
// when unset.
func (c *Config) VectorStoreDSN() string {
	if c.VectorStoreDatabaseURL != "" {
		return c.VectorStoreDatabaseURL
	}
	return c.DatabaseURL
}

func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}
	// Same shape of half-configuration as obrelin's own check: a host with
	// nothing to send from produces a relay rejecting every message while
	// nobody is told. The password is never reported, only whether a sender
	// exists.
	if cfg.SMTPHost != "" && cfg.SMTPFrom == "" {
		return nil, fmt.Errorf("config: SMTP_HOST is set but SMTP_FROM is not; " +
			"leave SMTP_HOST empty to disable email verification/password-reset delivery")
	}
	return cfg, nil
}

// EmailEnabled reports whether this deployment can send email at all.
func (c *Config) EmailEnabled() bool { return c.SMTPHost != "" }

// FieldEncryptionKey decodes FieldEncryptionKeyB64 into the raw key bytes
// crypto.NewFieldCipher expects.
func (c *Config) FieldEncryptionKey() ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(c.FieldEncryptionKeyB64)
	if err != nil {
		return nil, fmt.Errorf("config: FIELD_ENCRYPTION_KEY is not valid base64: %w", err)
	}
	return key, nil
}
