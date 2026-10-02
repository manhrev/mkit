// Package oauthserver implements the OAuth2 Authorization Code grant
// (RFC 6749 §4.1), Client Credentials grant (RFC 6749 §4.4), and refreshing
// a delegated access token (RFC 6749 §6) for internal, confidential
// clients: no PKCE (clients hold a client_secret),
// no dynamic client registration (ClientStore is expected to be a small
// static list). Token issuance itself is delegated to a TokenIssuer
// (authservice.Service satisfies it).
//
// Consent is per-client (Client.RequireConsent): a client that doesn't
// require it is auto-approved once the user is authenticated (first-party
// trusted). A client that does gets a two-step Authorize→Decide dance
// instead of an immediate code — see Authorize's doc comment. There's no
// consent *screen* here (no HTML/frontend in this codebase at all) — this
// only implements the API side a frontend would call to render one.
//
// See client.go, code.go, consent.go for the Client/AuthorizationCode/
// ConsentTicket types and their stores, and issuer.go for TokenIssuer.
package oauthserver

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/manhrev/mkit/error/serviceerr"
)

// AuthorizeResult is what Authorize returns: either a redirect (grant went
// straight through) or a pending consent that needs a Decide call first —
// never both.
type AuthorizeResult struct {
	RedirectURL     string // set when ConsentRequired is false
	ConsentRequired bool
	ConsentID       string // set when ConsentRequired is true
}

type Service struct {
	auth     TokenIssuer
	clients  ClientStore
	codes    AuthorizationCodeStore
	consents ConsentStore
}

func New(auth TokenIssuer, clients ClientStore, codes AuthorizationCodeStore, consents ConsentStore) *Service {
	return &Service{auth: auth, clients: clients, codes: codes, consents: consents}
}

// Authorize validates client_id/redirect_uri/scope/PKCE for an already-
// authenticated user (userID from a prior ValidateAccessToken call).
// code_challenge_method must be "S256" — PKCE is mandatory on every call,
// not just for public clients (see RFC 7636, OAuth 2.1 guidance). For a
// client with RequireConsent false, this grants immediately (same as
// before consent existed) and returns a redirect URL carrying a fresh
// authorization code + state. For a RequireConsent client, it instead
// stashes a pending ConsentTicket and returns its ConsentID — the caller
// must then call Decide with the user's approve/deny to actually get a
// code (or a denial redirect). scope is capped against client.Scopes here,
// at request time, either way — the eventual code (and the token it's
// exchanged for) never carries more than what was requested.
//
// Note: this package doesn't itself establish who the user is — no cookie
// session, no browser login page. The caller (the HTTP controller) is
// expected to have already verified the user via a bearer access token.
// That means this only works when whatever calls Authorize already holds
// that token (a script/fetch call), not a real browser top-level redirect
// (which can't carry a custom header) — see the controller for the full
// caveat.
func (s *Service) Authorize(ctx context.Context, clientID, redirectURI, scope, state, userID, codeChallenge, codeChallengeMethod string) (AuthorizeResult, error) {
	client, err := s.clients.Get(ctx, clientID)
	if err != nil {
		return AuthorizeResult{}, serviceerr.NewInvalidArgument(err).SetMessage("Unknown client_id.")
	}

	if !slices.Contains(client.RedirectURIs, redirectURI) {
		// Deliberately NOT redirecting on this error — redirecting to an
		// unregistered URI is exactly the open-redirect this check exists
		// to prevent.
		return AuthorizeResult{}, serviceerr.NewInvalidArgument(fmt.Errorf("redirect_uri not registered for client")).
			SetMessage("redirect_uri does not match a registered URI for this client.")
	}

	if err := checkScope(client, scope); err != nil {
		return AuthorizeResult{}, err
	}

	if codeChallengeMethod != "S256" || codeChallenge == "" {
		// "plain" is a valid RFC 7636 method (for clients that can't hash)
		// but meaningfully weaker, and out of scope here — S256 only.
		return AuthorizeResult{}, serviceerr.NewInvalidArgument(fmt.Errorf("code_challenge_method must be S256")).
			SetMessage(`code_challenge (with code_challenge_method=S256) is required.`)
	}

	if !client.RequireConsent {
		redirectURL, err := s.grant(ctx, clientID, redirectURI, scope, state, userID, codeChallenge)
		if err != nil {
			return AuthorizeResult{}, err
		}

		return AuthorizeResult{RedirectURL: redirectURL}, nil
	}

	consentID, err := newCode()
	if err != nil {
		return AuthorizeResult{}, serviceerr.NewInternal(err)
	}

	if err := s.consents.Save(ctx, consentID, ConsentTicket{
		ClientID:      clientID,
		UserID:        userID,
		RedirectURI:   redirectURI,
		Scope:         scope,
		State:         state,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(consentTTL),
	}); err != nil {
		return AuthorizeResult{}, serviceerr.NewInternal(err)
	}

	return AuthorizeResult{ConsentRequired: true, ConsentID: consentID}, nil
}

// Decide resolves a pending consent (from a RequireConsent client's
// Authorize call): approve issues a code exactly like a non-consent grant
// would, deny returns an RFC 6749 §4.1.2.1 access_denied redirect instead.
// userID must match the ticket's — one user can't decide another's pending
// consent, even holding a valid consentID (which is otherwise just an
// opaque, unguessable secret, same as an authorization code).
func (s *Service) Decide(ctx context.Context, consentID string, approve bool, userID string) (redirectURL string, err error) {
	ticket, err := s.consents.Consume(ctx, consentID)
	if err != nil {
		return "", serviceerr.NewUnauthenticated(fmt.Errorf("invalid or already-used consent: %w", err)).
			SetMessage("Invalid, expired, or already-decided consent request.")
	}

	if ticket.UserID != userID {
		return "", serviceerr.NewPermissionDenied(fmt.Errorf("consent ticket belongs to a different user")).
			SetMessage("This consent request does not belong to you.")
	}

	if time.Now().After(ticket.ExpiresAt) {
		return "", serviceerr.NewUnauthenticated(fmt.Errorf("consent ticket expired")).
			SetMessage("Consent request expired.")
	}

	if !approve {
		return ticket.RedirectURI + "?error=access_denied&state=" + ticket.State, nil
	}

	return s.grant(ctx, ticket.ClientID, ticket.RedirectURI, ticket.Scope, ticket.State, ticket.UserID, ticket.CodeChallenge)
}

// grant issues a fresh authorization code and returns the redirect URL
// carrying it + state — the actual "authorization succeeded" step, shared
// by the no-consent-needed path and Decide's approve path.
func (s *Service) grant(ctx context.Context, clientID, redirectURI, scope, state, userID, codeChallenge string) (redirectURL string, err error) {
	code, err := newCode()
	if err != nil {
		return "", serviceerr.NewInternal(err)
	}

	if err := s.codes.Save(ctx, code, AuthorizationCode{
		ClientID:      clientID,
		UserID:        userID,
		RedirectURI:   redirectURI,
		Scope:         scope,
		CodeChallenge: codeChallenge,
		ExpiresAt:     time.Now().Add(codeTTL),
	}); err != nil {
		return "", serviceerr.NewInternal(err)
	}

	return redirectURI + "?code=" + code + "&state=" + state, nil
}

// Exchange validates client credentials + the code + PKCE code_verifier,
// consumes the code (single use), and issues an access+refresh pair for
// the code's user.
func (s *Service) Exchange(ctx context.Context, clientID, clientSecret, code, redirectURI, codeVerifier string) (access, refresh string, err error) {
	client, err := s.clients.Get(ctx, clientID)
	if err != nil || client.Secret != clientSecret {
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("invalid client credentials")).
			SetMessage("Invalid client_id or client_secret.")
	}

	ac, err := s.codes.Consume(ctx, code)
	if err != nil {
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("invalid or already-used code: %w", err)).
			SetMessage("Invalid, expired, or already-used authorization code.")
	}

	if ac.ClientID != clientID || ac.RedirectURI != redirectURI {
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("code was not issued to this client/redirect_uri")).
			SetMessage("Authorization code does not match this client/redirect_uri.")
	}

	if time.Now().After(ac.ExpiresAt) {
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("authorization code expired")).
			SetMessage("Authorization code expired.")
	}

	if S256Challenge(codeVerifier) != ac.CodeChallenge {
		// Same bucket as a bad client_secret — both mean "you're not the
		// party this code/request was issued to."
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("code_verifier does not match code_challenge")).
			SetMessage("Invalid code_verifier.")
	}

	return s.auth.IssueForClient(ctx, ac.UserID, ac.ClientID, ac.Scope)
}

// ClientCredentials implements RFC 6749 §4.4 (Client Credentials Grant): a
// client authenticates with its own client_secret and gets a token for
// itself directly — no user, no redirect, no consent, no refresh token
// (nothing to refresh; the client just calls this again with its secret).
// For machine-to-machine callers where there's no user to act on behalf of.
func (s *Service) ClientCredentials(ctx context.Context, clientID, clientSecret, scope string) (access string, err error) {
	client, err := s.clients.Get(ctx, clientID)
	if err != nil || client.Secret != clientSecret {
		return "", serviceerr.NewUnauthenticated(fmt.Errorf("invalid client credentials")).
			SetMessage("Invalid client_id or client_secret.")
	}

	if err := checkScope(client, scope); err != nil {
		return "", err
	}

	return s.auth.IssueAccessForClient(ctx, clientID, scope)
}

// RefreshToken implements RFC 6749 §6 (Refreshing an Access Token) for a
// delegated refresh token issued by Exchange: the client re-authenticates
// with its client_secret, gets back a rotated access+refresh pair carrying
// the same client/scope as before — never more.
func (s *Service) RefreshToken(ctx context.Context, clientID, clientSecret, refreshToken string) (access, refresh string, err error) {
	client, err := s.clients.Get(ctx, clientID)
	if err != nil || client.Secret != clientSecret {
		return "", "", serviceerr.NewUnauthenticated(fmt.Errorf("invalid client credentials")).
			SetMessage("Invalid client_id or client_secret.")
	}

	return s.auth.RefreshForClient(ctx, clientID, refreshToken)
}
