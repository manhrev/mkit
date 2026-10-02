package oauthserver

import "context"

// TokenIssuer issues tokens for a grant this package has approved — the
// only slice of authservice.Service this package needs, so it depends on
// this instead of the concrete type.
type TokenIssuer interface {
	// IssueForClient issues an access+refresh pair delegated to a client
	// acting on userID's behalf (Authorization Code grant).
	IssueForClient(ctx context.Context, userID, clientID, scope string) (access, refresh string, err error)
	// IssueAccessForClient issues an access-only token for a client acting
	// as itself, no user involved (Client Credentials grant).
	IssueAccessForClient(ctx context.Context, clientID, scope string) (access string, err error)
	// RefreshForClient rotates a delegated refresh token (from
	// IssueForClient), reissuing access+refresh for the client/scope it
	// already carried — never more (RFC 6749 §6).
	RefreshForClient(ctx context.Context, clientID, refreshToken string) (access, refresh string, err error)
}
