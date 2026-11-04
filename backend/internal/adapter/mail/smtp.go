// Package mail sends transactional email — email verification and password
// reset links (usecase/auth). Modeled directly on obrelin's
// internal/adapter/mail: deliberately small and deliberately stdlib —
// net/smtp covers submission with STARTTLS and auth, which is the whole of
// what this needs.
package mail

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// ErrNotConfigured means no SMTP host was set. Distinct from a send
// failure: nothing is wrong, this deployment simply does not send email —
// registration and password reset still work, just without the email step.
var ErrNotConfigured = errors.New("mail: no SMTP server is configured")

// Config is what an operator supplies to enable delivery.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	// StartTLS upgrades the connection before authenticating. Defaults ON in
	// the composition root: sending a password and someone's email in clear
	// text across a network should be a choice an operator makes
	// deliberately, not one they get by leaving a variable unset.
	StartTLS bool
	// InsecureSkipVerify exists for a self-hosted relay with a private
	// certificate authority. Named the way it is so nobody sets it by
	// accident.
	InsecureSkipVerify bool
}

func (c Config) configured() bool { return strings.TrimSpace(c.Host) != "" }

// Sender posts messages. One per process; it holds no connection between
// sends, because this traffic (one email per registration/reset request) is
// far too infrequent for a pooled connection to be worth keeping alive.
type Sender struct {
	cfg Config
}

func NewSender(cfg Config) *Sender { return &Sender{cfg: cfg} }

// Configured reports whether this deployment can send at all.
func (s *Sender) Configured() bool { return s != nil && s.cfg.configured() }

// Send posts one plain-text message. Returns ErrNotConfigured rather than
// attempting anything when no SMTP host is set — callers (usecase/auth)
// treat that as "skip the email, the token still works for whoever has it
// directly."
func (s *Sender) Send(to, subject, body string) error {
	if !s.Configured() {
		return ErrNotConfigured
	}
	if strings.TrimSpace(to) == "" {
		return errors.New("mail: no recipient")
	}

	msg := s.compose(to, subject, body)

	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("mail: connecting to %s: %w", addr, err)
	}
	defer func() { _ = client.Close() }()

	if s.cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			// Refused rather than downgraded. An operator who asked for TLS
			// and silently got plaintext would be worse off than one told
			// their relay cannot do it, because they would believe the
			// message was protected in transit.
			return errors.New("mail: the server does not offer STARTTLS and TLS was required")
		}
		if err := client.StartTLS(&tls.Config{
			ServerName:         s.cfg.Host,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: s.cfg.InsecureSkipVerify, //nolint:gosec // operator-chosen, named to be obvious
		}); err != nil {
			return fmt.Errorf("mail: starting TLS: %w", err)
		}
	}

	if s.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)); err != nil {
			return fmt.Errorf("mail: authenticating: %w", err)
		}
	}

	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("mail: sender rejected: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("mail: recipient rejected: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: opening the message body: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("mail: writing the message: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("mail: completing the message: %w", err)
	}
	return client.Quit()
}

func (s *Sender) compose(to, subject, body string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", headerSafe(s.cfg.From))
	fmt.Fprintf(&b, "To: %s\r\n", headerSafe(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", headerSafe(subject)))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return b.Bytes()
}

// headerSafe strips what would let a value forge headers.
//
// CR and LF are the whole attack: an email address containing one would end
// the header it is in and begin another, which is how a message acquires a
// Bcc nobody wrote. The To address is the field that matters here, because
// it is the one a caller (ultimately, whoever filled in the register/login
// form) supplies.
func headerSafe(v string) string {
	return strings.TrimSpace(strings.NewReplacer("\r", "", "\n", "", "\x00", "").Replace(v))
}
