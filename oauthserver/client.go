package oauthserver

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/manhrev/mkit/error/serviceerr"
)

// Client is a registered OAuth2 client (another confidential backend
// service, not a browser/SPA — it holds Secret and calls Exchange itself).
type Client struct {
	ID             string
	Secret         string
	RedirectURIs   []string
	Scopes         []string // scopes this client is allowed to request
	RequireConsent bool     // if false, auto-approved once the user is authenticated (first-party trusted)
}

type ClientStore interface {
	Get(ctx context.Context, clientID string) (Client, error)
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
