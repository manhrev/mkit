package jwtmanager

import (
	"encoding/base64"
	"testing"
)

func TestJWKS(t *testing.T) {
	s := testService(t)

	jwks := s.JWKS()

	if len(jwks.Keys) != 1 {
		t.Fatalf("len(Keys) = %d, want 1", len(jwks.Keys))
	}

	key := jwks.Keys[0]

	if key.Kty != "OKP" {
		t.Errorf("Kty = %q, want OKP", key.Kty)
	}

	if key.Crv != "Ed25519" {
		t.Errorf("Crv = %q, want Ed25519", key.Crv)
	}

	if key.Alg != "EdDSA" {
		t.Errorf("Alg = %q, want EdDSA", key.Alg)
	}

	x, err := base64.RawURLEncoding.DecodeString(key.X)
	if err != nil {
		t.Fatalf("decode X: %v", err)
	}

	if string(x) != string(s.publicKey) {
		t.Error("X does not decode back to the service's public key")
	}

	if key.Kid == "" {
		t.Error("Kid is empty")
	}

	if again := s.JWKS().Keys[0].Kid; again != key.Kid {
		t.Errorf("Kid is not deterministic: %q then %q", key.Kid, again)
	}
}
