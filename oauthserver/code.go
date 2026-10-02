package oauthserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// codeTTL is how long an authorization code is valid before exchange —
// short-lived by design, it's meant to be exchanged within seconds of the
// redirect, not stored or reused.
const codeTTL = 2 * time.Minute

// AuthorizationCode is what AuthorizationCodeStore persists between
// Authorize (issues it) and Exchange (consumes it).
type AuthorizationCode struct {
	ClientID      string
	UserID        string
	RedirectURI   string
	Scope         string
	CodeChallenge string // RFC 7636, S256 method only
	ExpiresAt     time.Time
}

// AuthorizationCodeStore holds short-lived, single-use authorization codes.
type AuthorizationCodeStore interface {
	Save(ctx context.Context, code string, ac AuthorizationCode) error
	// Consume atomically gets and deletes code — single use, so a replay
	// (or a second /token call for the same code) is rejected.
	Consume(ctx context.Context, code string) (AuthorizationCode, error)
}

// newCode generates an opaque, single-use authorization code — a random
// secret, not an identifier, so crypto/rand rather than uuid. Also used for
// consentID (same shape: opaque, unguessable, single-use).
func newCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate authorization code: %w", err)
	}

	return hex.EncodeToString(b), nil
}
