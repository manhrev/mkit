package oauthserver

import (
	"crypto/sha256"
	"encoding/base64"
)

// S256Challenge computes the RFC 7636 §4.2 code_challenge for a
// code_verifier: BASE64URL-ENCODE(SHA256(ASCII(code_verifier))), no padding.
// Used by Exchange to verify a client's code_verifier, and by real clients
// (and this package's tests/dev example, standing in for one) to compute
// the code_challenge sent to Authorize in the first place.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}
