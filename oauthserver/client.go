package oauthserver

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/manhrev/mkit/error/serviceerr"
)

// Client is a registered OAuth2 client. A confidential client (a backend
// service) holds Secret and sends it to Exchange/RefreshToken. A Public
// client (native/desktop app, RFC 8252) can't keep a secret, so it has none
// and relies on PKCE alone; it can't use ClientCredentials.
type Client struct {
	ID             string
	Secret         string
	Public         bool
	RedirectURIs   []string
	Scopes         []string // scopes this client is allowed to request
	RequireConsent bool     // if false, auto-approved once the user is authenticated (first-party trusted)
}

type ClientStore interface {
	Get(ctx context.Context, clientID string) (Client, error)
}

// checkSecret reports whether secret authenticates client: a Public client
// must send none, a confidential one must match its (non-empty) Secret.
func checkSecret(client Client, secret string) bool {
	if client.Public {
		return secret == ""
	}

	return client.Secret != "" && subtle.ConstantTimeCompare([]byte(client.Secret), []byte(secret)) == 1
}

// redirectURIAllowed reports whether redirectURI matches one of client's
// registered URIs exactly, or — for a loopback IP redirect (RFC 8252 §7.3)
// — matches ignoring the port, since a native app binds a random free port
// per sign-in.
func redirectURIAllowed(client Client, redirectURI string) bool {
	if slices.Contains(client.RedirectURIs, redirectURI) {
		return true
	}

	u, err := url.Parse(redirectURI)
	if err != nil || !isLoopback(u) {
		return false
	}

	for _, reg := range client.RedirectURIs {
		r, err := url.Parse(reg)
		if err == nil && isLoopback(r) && r.Hostname() == u.Hostname() &&
			r.Path == u.Path && r.RawQuery == u.RawQuery && u.Fragment == "" && u.User == nil {
			return true
		}
	}

	return false
}

// isLoopback: http on a loopback IP literal. "localhost" is deliberately
// excluded (RFC 8252 §8.3: it can resolve elsewhere).
func isLoopback(u *url.URL) bool {
	return u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")
}

// checkScope validates that every space-separated scope in scope is one
// client is allowed to request — the eventual token never carries more than
// what was requested here, at grant time, either way.
func checkScope(client Client, scope string) error {
	for sc := range strings.FieldsSeq(scope) {
		if !slices.Contains(client.Scopes, sc) {
			return serviceerr.NewInvalidArgument(fmt.Errorf("scope %q not allowed for client", sc)).
				SetMessage("Requested scope exceeds what this client is allowed.")
		}
	}

	return nil
}
