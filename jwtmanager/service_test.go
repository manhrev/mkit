package jwtmanager

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func testService(t *testing.T) *Service {
	t.Helper()

	priv, err := LoadPrivateKey("testdata/priv.pem")
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	pub, err := LoadPublicKey("testdata/pub.pem")
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	s, err := New(priv, pub, Config{
		AccessTokenDuration:  time.Hour,
		RefreshTokenDuration: 24 * time.Hour,
		Issuer:               "gorest-test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return s
}

func TestAccessTokenRoundtrip(t *testing.T) {
	s := testService(t)

	tok, err := s.GenerateAccessToken("user-1", []string{"admin"})
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := s.Verify(tok, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.Subject != "user-1" {
		t.Errorf("Subject = %q, want user-1", claims.Subject)
	}

	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", claims.Roles)
	}

	if claims.TokenType != TokenTypeAccess {
		t.Errorf("TokenType = %q, want %q", claims.TokenType, TokenTypeAccess)
	}
}

func TestWithDelegation(t *testing.T) {
	s := testService(t)

	tok, err := s.GenerateAccessToken("user-1", []string{"admin"}, WithDelegation("internal-service", "read:resource"))
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := s.Verify(tok, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !claims.IsDelegated() {
		t.Error("IsDelegated() = false, want true")
	}

	if claims.ClientID != "internal-service" {
		t.Errorf("ClientID = %q, want internal-service", claims.ClientID)
	}

	if len(claims.Roles) != 0 {
		t.Errorf("Roles = %v, want empty — WithDelegation must clear roles passed to GenerateAccessToken", claims.Roles)
	}

	if got := claims.Permissions(); len(got) != 1 || got[0] != "read:resource" {
		t.Errorf("Permissions() = %v, want [read:resource]", got)
	}

	if !claims.HasAudience("internal-service") {
		t.Errorf("Audience = %v, want [internal-service] — WithDelegation must narrow aud to just the client", claims.Audience)
	}

	if claims.HasAudience("gorest-test") {
		t.Error("HasAudience(issuer) = true, want false — delegated token must not carry the default audience")
	}
}

func TestDefaultAudience(t *testing.T) {
	priv, err := LoadPrivateKey("testdata/priv.pem")
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	pub, err := LoadPublicKey("testdata/pub.pem")
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	s, err := New(priv, pub, Config{
		AccessTokenDuration:  time.Hour,
		RefreshTokenDuration: 24 * time.Hour,
		Issuer:               "gorest-test",
		Audience:             []string{"gorest-test"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tok, err := s.GenerateAccessToken("user-1", []string{"admin"})
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := s.Verify(tok, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !claims.HasAudience("gorest-test") {
		t.Errorf("Audience = %v, want to include configured default gorest-test", claims.Audience)
	}
}

func TestWithAudienceOverride(t *testing.T) {
	s := testService(t)

	tok, err := s.GenerateAccessToken("user-1", []string{"admin"}, WithAudience("some-other-service"))
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := s.Verify(tok, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if !claims.HasAudience("some-other-service") {
		t.Errorf("Audience = %v, want [some-other-service]", claims.Audience)
	}
}

func TestPermissionsNonDelegated(t *testing.T) {
	s := testService(t)

	tok, err := s.GenerateAccessToken("user-1", []string{"admin", "editor"})
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := s.Verify(tok, TokenTypeAccess)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if claims.IsDelegated() {
		t.Error("IsDelegated() = true, want false for a direct user token")
	}

	got := claims.Permissions()
	if len(got) != 2 || got[0] != "admin" || got[1] != "editor" {
		t.Errorf("Permissions() = %v, want [admin editor]", got)
	}
}

func TestRefreshTokenRejectedAsAccess(t *testing.T) {
	s := testService(t)

	tok, err := s.GenerateRefreshToken("user-1")
	if err != nil {
		t.Fatalf("GenerateRefreshToken: %v", err)
	}

	if _, err := s.Verify(tok, TokenTypeAccess); err == nil {
		t.Fatal("Verify(want=access) on refresh token: expected error, got nil")
	}
}

func TestWrongKeyRejected(t *testing.T) {
	s := testService(t)

	otherPriv, err := LoadPrivateKey("testdata/other_priv.pem")
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	pub, err := LoadPublicKey("testdata/pub.pem") // mismatched pair
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	other, err := New(otherPriv, pub, Config{
		AccessTokenDuration:  time.Hour,
		RefreshTokenDuration: 24 * time.Hour,
		Issuer:               "gorest-test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tok, err := other.GenerateAccessToken("user-1", nil)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	if _, err := s.Verify(tok, TokenTypeAccess); err == nil {
		t.Fatal("Verify with mismatched key: expected error, got nil")
	}
}

func TestAlgNoneRejected(t *testing.T) {
	s := testService(t)

	claims := &Claims{
		Identity:  Identity{Roles: []string{"admin"}},
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign alg=none token: %v", err)
	}

	if _, err := s.Verify(tok, TokenTypeAccess); err == nil {
		t.Fatal("Verify on alg=none token: expected error, got nil")
	}
}

func TestExpiredTokenRejected(t *testing.T) {
	s := testService(t)

	tok, err := s.generate("user-1", TokenTypeAccess, func(c *Claims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := s.Verify(tok, TokenTypeAccess); err == nil {
		t.Fatal("Verify on expired token: expected error, got nil")
	}
}
