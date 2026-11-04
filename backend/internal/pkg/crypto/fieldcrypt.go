// Package crypto provides field-level encryption for secrets stored in the
// database (trunk provider API keys, SIP digest passwords). These values must
// never be persisted in plaintext (secrets-management risk area).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// FieldCipher encrypts individual string fields with AES-256-GCM. Each call to
// Encrypt uses a fresh random nonce, stored alongside the ciphertext, so the
// same plaintext never produces the same output twice.
type FieldCipher struct {
	aead cipher.AEAD
}

// NewFieldCipher builds a FieldCipher from a 32-byte AES-256 key. The key
// comes from config (e.g. an env var), never from a literal in source.
func NewFieldCipher(key []byte) (*FieldCipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("crypto: key must be 32 bytes for AES-256, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("crypto: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("crypto: new gcm: %w", err)
	}
	return &FieldCipher{aead: aead}, nil
}

// Encrypt returns base64(nonce || ciphertext). An empty plaintext encrypts to
// an empty string, so an unset optional field round-trips without needing
// special-casing at every call site.
func (c *FieldCipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("crypto: read nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt is the inverse of Encrypt.
func (c *FieldCipher) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("crypto: decode: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("crypto: ciphertext too short")
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("crypto: open: %w", err)
	}
	return string(plaintext), nil
}
