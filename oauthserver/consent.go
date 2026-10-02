package oauthserver

import (
	"context"
	"time"
)

// consentTTL is how long a pending consent decision stays open — the gap
// between showing the user "App X wants Y" and them clicking Allow/Deny.
const consentTTL = 5 * time.Minute

// ConsentTicket is what ConsentStore persists between Authorize (issues it,
// for a RequireConsent client) and Decide (consumes it).
type ConsentTicket struct {
	ClientID      string
	UserID        string
	RedirectURI   string
	Scope         string
	State         string
	CodeChallenge string // RFC 7636, S256 method only — carried through to grant on approve
	ExpiresAt     time.Time
}

// ConsentStore holds short-lived, single-use pending consent decisions.
type ConsentStore interface {
	Save(ctx context.Context, consentID string, t ConsentTicket) error
	// Consume atomically gets and deletes consentID — single use, so the
	// same decision can't be replayed.
	Consume(ctx context.Context, consentID string) (ConsentTicket, error)
}
