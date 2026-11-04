// Package keygen mints and digests bearer tokens (session tokens, email
// verification/password-reset tokens). Deliberately tiny and
// framework-agnostic: the usecase layer depends on it, nothing here depends
// on the usecase layer.
package keygen

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// SessionPrefix marks a string as a human-login session token
// (usecase/auth) — identifiable at a glance in logs and secret scanners.
const SessionPrefix = "asis_"

// VerifyEmailTokenPrefix and PasswordResetTokenPrefix mark the two kinds of
// single-use emailed tokens (usecase/auth, domain.EmailToken) — the same
// identifiable-in-logs reasoning as SessionPrefix: a token leaked into a log
// or a browser history should say at a glance what it's for.
const (
	VerifyEmailTokenPrefix   = "asiv_"
	PasswordResetTokenPrefix = "asir_"
)

// secretBytes is the entropy behind every generated token. 256 bits is what
// lets Hash be a plain digest instead of a password KDF — the token itself
// is the only secret, there is no low-entropy input to slow an offline
// guesser down for.
const secretBytes = 32

// GenerateWithPrefix returns a new bearer token under the given prefix and
// the digest to store instead of it. The plaintext is the caller's only
// copy — it cannot be recovered from the digest.
func GenerateWithPrefix(tokenPrefix string) (plaintext, hash string, err error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("keygen: read entropy: %w", err)
	}

	plaintext = tokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, Hash(plaintext), nil
}

// Hash digests a presented key for lookup, as hex. Authentication compares
// digests, never the keys themselves, so the database never holds a usable
// credential.
func Hash(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
