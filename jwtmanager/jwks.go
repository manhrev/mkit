package jwtmanager

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
)

// JWK is an Ed25519 public key (RFC 8037 OKP) in RFC 7517 JWK Set format —
// just enough fields for a verifier to check an EdDSA signature, no private
// material.
type JWK struct {
	Kty string `json:"kty"` // "OKP" — Octet Key Pair, RFC 8037
	Crv string `json:"crv"` // "Ed25519"
	X   string `json:"x"`   // base64url(raw public key bytes), no padding
	Alg string `json:"alg"` // "EdDSA"
	Use string `json:"use"` // "sig"
	Kid string `json:"kid"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the service's public key as a single-entry JWK Set for a
// /.well-known/jwks.json endpoint. Kid matches the kid header on every token
// this Service signs (see keyID).
func (s *Service) JWKS() JWKS {
	return JWKS{
		Keys: []JWK{{
			Kty: "OKP",
			Crv: "Ed25519",
			X:   base64.RawURLEncoding.EncodeToString(s.publicKey),
			Alg: "EdDSA",
			Use: "sig",
			Kid: s.kid,
		}},
	}
}

// keyID derives a kid from the public key itself (truncated SHA-256,
// base64url) rather than configuring one, since there's only ever one key
// today — add key rotation (multiple configured keys, each with its own kid)
// if that changes. Deterministic, so a verifier caching the JWKS keeps
// matching across restarts.
func keyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)

	return base64.RawURLEncoding.EncodeToString(sum[:8])
}
