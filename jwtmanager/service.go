package jwtmanager

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Service struct {
	privateKey           ed25519.PrivateKey
	publicKey            ed25519.PublicKey
	accessTokenDuration  time.Duration
	refreshTokenDuration time.Duration
	issuer               string
	audience             []string
}

func New(privateKey ed25519.PrivateKey, publicKey ed25519.PublicKey, cfg Config) (*Service, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("jwtmanager: invalid private key length %d, want %d", len(privateKey), ed25519.PrivateKeySize)
	}

	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("jwtmanager: invalid public key length %d, want %d", len(publicKey), ed25519.PublicKeySize)
	}

	return &Service{
		privateKey:           privateKey,
		publicKey:            publicKey,
		accessTokenDuration:  cfg.AccessTokenDuration,
		refreshTokenDuration: cfg.RefreshTokenDuration,
		issuer:               cfg.Issuer,
		audience:             cfg.Audience,
	}, nil
}

// ClaimOption mutates an access token's claims before signing, e.g.
// WithDomain, WithDeviceID.
type ClaimOption func(*Claims)

func WithRoles(roles []string) ClaimOption {
	return func(c *Claims) { c.Roles = roles }
}

// WithAudience overrides a token's aud claim — see Service's default (cfg
// Audience) and WithDelegation's narrower default for the common cases;
// this is for anything else.
func WithAudience(aud ...string) ClaimOption {
	return func(c *Claims) { c.Audience = jwt.ClaimStrings(aud) }
}

func WithDomain(domain string) ClaimOption {
	return func(c *Claims) { c.Domain = domain }
}

func WithDeviceID(deviceID string) ClaimOption {
	return func(c *Claims) { c.DeviceID = deviceID }
}

func WithSessionID(sessionID string) ClaimOption {
	return func(c *Claims) { c.SessionID = sessionID }
}

func WithMetadata(metadata map[string]string) ClaimOption {
	return func(c *Claims) { c.Metadata = metadata }
}

// WithDelegation marks the token as issued to an OAuth client acting on the
// subject's behalf (RFC 9068 client_id + scope claims), replacing any roles
// passed to GenerateAccessToken — a delegated token is authorized by Scope,
// never by the user's own Roles. See Claims.Permissions. Also narrows aud
// to just clientID (overriding Service's default audience) — the token is
// only valid at the resource server clientID itself is, not wherever a
// direct user token would be accepted.
func WithDelegation(clientID, scope string) ClaimOption {
	return func(c *Claims) {
		c.ClientID = clientID
		c.Scope = scope
		c.Roles = nil
		c.Audience = jwt.ClaimStrings{clientID}
	}
}

// duration returns how long a freshly generated token of tokenType stays
// valid — a pure function of tokenType, so generate can derive it instead
// of taking it as a param.
func (s *Service) duration(tokenType TokenType) time.Duration {
	if tokenType == TokenTypeRefresh {
		return s.refreshTokenDuration
	}

	return s.accessTokenDuration
}

func (s *Service) generate(subject string, tokenType TokenType, opts ...ClaimOption) (string, error) {
	now := time.Now()

	claims := &Claims{
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    s.issuer,
			Audience:  jwt.ClaimStrings(s.audience), // default; opts (e.g. WithDelegation) may narrow it
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.duration(tokenType))),
			ID:        uuid.NewString(),
		},
	}

	for _, opt := range opts {
		opt(claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}

	return signed, nil
}

func (s *Service) GenerateAccessToken(subject string, roles []string, opts ...ClaimOption) (string, error) {
	return s.generate(subject, TokenTypeAccess, append([]ClaimOption{WithRoles(roles)}, opts...)...)
}

// GenerateRefreshToken carries only subject + type, least-privilege: smaller
// blast radius if leaked. opts is normally empty; a delegated issuance
// (WithDelegation) passes it through too, so the refresh token itself
// carries ClientID/Scope — otherwise it'd be indistinguishable from a
// plain login refresh token once signed.
func (s *Service) GenerateRefreshToken(subject string, opts ...ClaimOption) (string, error) {
	return s.generate(subject, TokenTypeRefresh, opts...)
}

func (s *Service) Verify(tokenString string, wantType TokenType) (*Claims, error) {
	claims := &Claims{}

	_, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return s.publicKey, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
			return nil, fmt.Errorf("invalid token signature: %w", err)
		}

		return nil, fmt.Errorf("parse token: %w", err)
	}

	if claims.TokenType != wantType {
		return nil, fmt.Errorf("unexpected token type: want %s got %s", wantType, claims.TokenType)
	}

	return claims, nil
}
