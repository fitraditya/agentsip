// Package logging is this project's logging surface, over
// github.com/obrel/go-lib/pkg/log — the same shared logger obrelin uses, and
// for the same reason: it puts this service's output in the same shape as
// every other Mekari service (one format, one level convention, one Sentry
// hook), and these two services are operated together. go-lib's own
// interface is printf-only; this package keeps the structured, fluent shape
// call sites actually want, with fields going to logrus underneath (what
// go-lib wraps).
//
// # Redaction
//
// agentsip handles phone numbers (caller/callee), SIP credentials, and
// provider API keys (STT/LLM/TTS/embedding), so — same as obrelin —
// redaction is not a debug-mode concern:
//
//   - Secret always renders as a placeholder, at every level, with no flag to
//     turn it off. There is no legitimate reason to put a credential in a
//     log, so there is no switch.
//   - Number masks by default. LOG_PARAMETERS=full unmasks it, a deliberate,
//     auditable choice for a bounded troubleshooting window, not a default.
package logging

import (
	"strings"
	"time"

	golog "github.com/obrel/go-lib/pkg/log"
	"github.com/sirupsen/logrus"
)

// ParameterMode controls how much detail reaches the log.
type ParameterMode string

const (
	// ParametersOff is the default: no parameter detail beyond what a
	// message states.
	ParametersOff ParameterMode = "off"
	// ParametersMasked logs parameters with personal data masked — enough to
	// correlate a call across components without reproducing the number.
	ParametersMasked ParameterMode = "masked"
	// ParametersFull logs identifiers unmasked. Credentials and keys are
	// still redacted; that part has no switch. Intended for a bounded
	// troubleshooting window, not a default, since it puts personal data
	// into a log that is probably shipped somewhere else.
	ParametersFull ParameterMode = "full"
)

// redacted is what a secret renders as. A fixed string rather than a length
// or a prefix: both leak information about the value, and neither helps
// anyone debug.
const redacted = "[redacted]"

var parameterMode = ParametersOff
var currentLevel golog.Level = golog.InfoLevel
var traceVerbose bool // true when LOG_LEVEL=4/trace for per-packet RTP spam

// Configure sets up the shared logger. Called once at startup.
func Configure(level string, jsonFormat bool, params ParameterMode) {
	currentLevel = parseLevel(level)
	// level 4 / trace enables per-packet spam
	traceVerbose = isTraceLevel(level)
	golog.SetLevel(currentLevel)
	if jsonFormat {
		golog.SetFormat(golog.JSONFormat)
	} else {
		golog.SetFormat(golog.TextFormat)
	}
	parameterMode = params
}

// ParametersEnabled reports whether parameter detail should be logged at
// all. Call sites check it before doing work purely to build a debug field.
func ParametersEnabled() bool { return parameterMode != ParametersOff }

// IsTraceEnabled reports whether per-packet verbose logs should be emitted (level 4/trace)
func IsTraceEnabled() bool { return traceVerbose }

func isTraceLevel(level string) bool {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "4", "trace", "5":
		return true
	default:
		return false
	}
}

func parseLevel(level string) golog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "4", "trace":
		return golog.DebugLevel // most verbose, includes trace
	case "3", "debug":
		return golog.DebugLevel
	case "1", "warn", "warning":
		return golog.WarnLevel
	case "0", "error":
		return golog.ErrorLevel
	case "fatal":
		return golog.FatalLevel
	case "2", "info", "":
		fallthrough
	default:
		// numeric 2 or info or empty defaults to info
		// numeric handling: 0=error,1=warn,2=info,3=debug,4=trace
		switch level {
		case "0":
			return golog.ErrorLevel
		case "1":
			return golog.WarnLevel
		case "2":
			return golog.InfoLevel
		case "3":
			return golog.DebugLevel
		case "4":
			return golog.DebugLevel
		}
		return golog.InfoLevel
	}
}

// Logger writes log entries. The zero value is usable and discards nothing —
// use Nop for a logger that discards.
type Logger struct {
	entry *logrus.Entry
}

// For returns a logger tagged with the package and scope it belongs to,
// which is what makes an entry attributable to a place in the code rather
// than just a moment in time.
func For(pkg, scope string) Logger {
	base := golog.For(pkg, scope)
	if entry, ok := base.(*logrus.Entry); ok {
		return Logger{entry: entry}
	}
	// go-lib returns a *logrus.Entry today. Falling back rather than
	// asserting keeps a change in its internals from panicking this process
	// at startup.
	return Logger{entry: logrus.NewEntry(golog.Standard()).WithFields(logrus.Fields{
		"package": pkg,
		"scope":   scope,
	})}
}

// Nop returns a logger that writes nowhere, for tests that are not
// asserting on output.
func Nop() Logger {
	discard := logrus.New()
	discard.SetOutput(nopWriter{})
	discard.SetLevel(logrus.PanicLevel)
	return Logger{entry: logrus.NewEntry(discard)}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }

func (l Logger) base() *logrus.Entry {
	if l.entry == nil {
		return logrus.NewEntry(golog.Standard())
	}
	return l.entry
}

// With returns a logger carrying an extra field on every subsequent entry —
// for a value that identifies a whole unit of work, such as a call, rather
// than one message.
func (l Logger) With(key string, value any) Logger {
	return Logger{entry: l.base().WithField(key, value)}
}

func (l Logger) Trace() *Event {
	if !traceVerbose {
		return nil
	}
	return newEvent(l.base(), logrus.DebugLevel)
}
func (l Logger) Debug() *Event { return newEvent(l.base(), logrus.DebugLevel) }
func (l Logger) Info() *Event  { return newEvent(l.base(), logrus.InfoLevel) }
func (l Logger) Warn() *Event  { return newEvent(l.base(), logrus.WarnLevel) }
func (l Logger) Error() *Event { return newEvent(l.base(), logrus.ErrorLevel) }
func (l Logger) Fatal() *Event { return newEvent(l.base(), logrus.FatalLevel) }

// Event accumulates fields for one entry, written when Msg is called.
// Nothing is emitted until then, so an Event that is built and dropped costs
// nothing.
type Event struct {
	entry  *logrus.Entry
	level  logrus.Level
	fields logrus.Fields
}

func newEvent(entry *logrus.Entry, level logrus.Level) *Event {
	return &Event{entry: entry, level: level, fields: logrus.Fields{}}
}

func (e *Event) set(key string, value any) *Event {
	if e == nil {
		return e
	}
	e.fields[key] = value
	return e
}

func (e *Event) Str(key, value string) *Event           { return e.set(key, value) }
func (e *Event) Int(key string, v int) *Event           { return e.set(key, v) }
func (e *Event) Int64(key string, v int64) *Event       { return e.set(key, v) }
func (e *Event) Bool(key string, v bool) *Event         { return e.set(key, v) }
func (e *Event) Dur(key string, v time.Duration) *Event { return e.set(key, v.String()) }

// Err attaches an error. A nil error is dropped rather than logged as
// "<nil>", so a shared path that sometimes has one does not produce a field
// that reads like a failure.
func (e *Event) Err(err error) *Event {
	if err == nil {
		return e
	}
	return e.set("error", err.Error())
}

// Secret records that a value exists without recording the value. Always
// redacted, at every level, with no flag — a credential in a log is a
// disclosure, and a debug switch is not a reason for one.
func (e *Event) Secret(key string) *Event { return e.set(key, redacted) }

// Number attaches a phone number or SIP user, masked unless
// LOG_PARAMETERS=full.
//
// Masked rather than omitted because correlating a call across components
// needs *something* stable, and the leading and trailing characters are
// enough to match lines by eye without reproducing the subscriber's number.
func (e *Event) Number(key, value string) *Event {
	if parameterMode == ParametersFull {
		return e.set(key, value)
	}
	return e.set(key, MaskNumber(value))
}

// Param attaches detail that only matters while debugging. Dropped entirely
// unless parameter logging is on, so the common case costs nothing.
func (e *Event) Param(key string, value any) *Event {
	if parameterMode == ParametersOff {
		return e
	}
	return e.set(key, value)
}

// Msg writes the entry.
func (e *Event) Msg(msg string) {
	if e == nil {
		return
	}
	e.entry.WithFields(e.fields).Log(e.level, msg)
	e.exitIfFatal()
}

// exitIfFatal ends the process after a fatal entry.
//
// logrus puts the exit in Entry.Fatal, NOT in Entry.Log(FatalLevel, ...) —
// Fatal is Log followed by Logger.Exit(1). This wrapper emits through Log,
// so without this, "fatal" would be a severity label and nothing else.
func (e *Event) exitIfFatal() {
	if e.level == logrus.FatalLevel {
		e.entry.Logger.Exit(1)
	}
}

// MaskNumber hides the subscriber part of a phone number or SIP user,
// keeping enough to correlate entries: the leading two characters and the
// final two.
//
// A short value is masked entirely — an extension like "1001" has no part
// that is safe to show and still meaningless on its own.
func MaskNumber(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 6 {
		return strings.Repeat("*", len(v))
	}
	return v[:2] + strings.Repeat("*", len(v)-4) + v[len(v)-2:]
}
