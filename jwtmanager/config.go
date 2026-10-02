package jwtmanager

import "time"

type Config struct {
	AccessTokenDuration  time.Duration
	RefreshTokenDuration time.Duration
	Issuer               string
	// Audience is the default aud claim for tokens this Service issues —
	// this issuer's own identity, for a direct (non-delegated) token. A
	// delegated token (WithDelegation) overrides this to just the client
	// it was issued to. Empty means no aud claim is set.
	Audience []string
}
