// Package password hashes and verifies human-chosen login passwords.
// Deliberately tiny and framework-agnostic, same shape as pkg/keygen:
// usecase/auth depends on it, nothing here depends on the usecase layer.
//
// bcrypt, not a plain digest — a login password is drawn from a far smaller
// effective space than keygen's 256 bits of CSPRNG output, so the defence
// has to be a slow, tunable KDF rather than a fast hash. Matches obrelin's
// own reasoning for its voicemail PIN hashing (internal/pkg/crypto/pin.go),
// which makes the same case for a human-chosen secret.
package password

import "golang.org/x/crypto/bcrypt"

// cost is bcrypt's default work factor — tuned for a password typed once at
// a login form, where a user waiting on it notices tens of milliseconds far
// less than an offline attacker trying millions of guesses does.
const cost = bcrypt.DefaultCost

// Hash returns a bcrypt hash of a plaintext password.
func Hash(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), cost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Verify reports whether a plaintext password matches its hash.
func Verify(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
