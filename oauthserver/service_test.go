package oauthserver

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/manhrev/mkit/authservice"
	"github.com/manhrev/mkit/jwtmanager"
)

// --- fakes (authservice deps, same shape as authservice's own tests) ---

type fakeVerifier struct{}

func (fakeVerifier) Verify(_ context.Context, username, password string) (string, error) {
	if username == "alice" && password == "hunter2" {
		return "user-alice", nil
	}

	return "", errors.New("bad credentials")
}

type fakeUserLookup struct{}

func (fakeUserLookup) RolesByUserID(_ context.Context, userID string) ([]string, error) {
	return []string{"admin"}, nil
}

type fakeRefreshStore struct {
	mu      sync.Mutex
	records map[string]string
}

func newFakeRefreshStore() *fakeRefreshStore { return &fakeRefreshStore{records: map[string]string{}} }

func (f *fakeRefreshStore) Save(_ context.Context, jti, userID string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[jti] = userID

	return nil
}

func (f *fakeRefreshStore) Get(_ context.Context, jti string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	userID, ok := f.records[jti]
	if !ok {
		return "", errors.New("not found")
	}

	return userID, nil
}

func (f *fakeRefreshStore) Delete(_ context.Context, jti string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, jti)

	return nil
}

type fakeBlocklist struct{}

func (fakeBlocklist) Block(_ context.Context, jti string, _ time.Time) error { return nil }
func (fakeBlocklist) IsBlocked(_ context.Context, jti string) (bool, error)  { return false, nil }

// --- fakes (oauthserver deps) ---

type fakeClientStore struct {
	clients map[string]Client
}

func (f *fakeClientStore) Get(_ context.Context, clientID string) (Client, error) {
	c, ok := f.clients[clientID]
	if !ok {
		return Client{}, errors.New("unknown client")
	}

	return c, nil
}

type fakeCodeStore struct {
	mu    sync.Mutex
	codes map[string]AuthorizationCode
}

func newFakeCodeStore() *fakeCodeStore { return &fakeCodeStore{codes: map[string]AuthorizationCode{}} }

func (f *fakeCodeStore) Save(_ context.Context, code string, ac AuthorizationCode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.codes[code] = ac

	return nil
}

func (f *fakeCodeStore) Consume(_ context.Context, code string) (AuthorizationCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	ac, ok := f.codes[code]
	if !ok {
		return AuthorizationCode{}, errors.New("unknown code")
	}
	delete(f.codes, code)

	return ac, nil
}

type fakeConsentStore struct {
	mu      sync.Mutex
	tickets map[string]ConsentTicket
}

func newFakeConsentStore() *fakeConsentStore {
	return &fakeConsentStore{tickets: map[string]ConsentTicket{}}
}

func (f *fakeConsentStore) Save(_ context.Context, consentID string, t ConsentTicket) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tickets[consentID] = t

	return nil
}

func (f *fakeConsentStore) Consume(_ context.Context, consentID string) (ConsentTicket, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	t, ok := f.tickets[consentID]
	if !ok {
		return ConsentTicket{}, errors.New("unknown or already-decided consent")
	}
	delete(f.tickets, consentID)

	return t, nil
}

// --- setup ---

const (
	testRedirectURI        = "http://localhost:9090/callback"
	testPartnerRedirectURI = "http://localhost:9091/callback"

	// testVerifier stands in for a real client's randomly-generated PKCE
	// code_verifier — fixed here since tests don't need randomness, just a
	// verifier/challenge pair that's consistent within a test.
	testVerifier = "test-code-verifier-0123456789abcdefghijklmnop"
)

var testChallenge = S256Challenge(testVerifier)

// testService returns a Service wired to a real authservice.Service (as
// its TokenIssuer) plus that authservice.Service itself, so tests can
// verify issued tokens (ValidateAccessToken) without the package needing
// to expose s.auth.
func testService(t *testing.T) (*Service, *authservice.Service) {
	t.Helper()

	priv, err := jwtmanager.LoadPrivateKey("../jwtmanager/testdata/priv.pem")
	if err != nil {
		t.Fatalf("LoadPrivateKey: %v", err)
	}

	pub, err := jwtmanager.LoadPublicKey("../jwtmanager/testdata/pub.pem")
	if err != nil {
		t.Fatalf("LoadPublicKey: %v", err)
	}

	jwtSvc, err := jwtmanager.New(priv, pub, jwtmanager.Config{
		AccessTokenDuration:  time.Hour,
		RefreshTokenDuration: 24 * time.Hour,
		Issuer:               "oauthserver-test",
	})
	if err != nil {
		t.Fatalf("jwtmanager.New: %v", err)
	}

	authSvc := authservice.New(jwtSvc, fakeVerifier{}, fakeUserLookup{}, newFakeRefreshStore(), fakeBlocklist{})

	clients := &fakeClientStore{clients: map[string]Client{
		"internal-service": {
			ID:           "internal-service",
			Secret:       "dev-secret",
			RedirectURIs: []string{testRedirectURI},
			Scopes:       []string{"read:resource"},
		},
		"partner-app": {
			ID:             "partner-app",
			Secret:         "partner-secret",
			RedirectURIs:   []string{testPartnerRedirectURI},
			Scopes:         []string{"read:resource"},
			RequireConsent: true,
		},
	}}

	return New(authSvc, clients, newFakeCodeStore(), newFakeConsentStore()), authSvc
}

// extractCode pulls the "code" query param out of a "url?code=...&state=..."
// redirect target built by Authorize.
func extractCode(t *testing.T, redirectURL string) string {
	t.Helper()

	u, err := url.Parse(redirectURL)
	if err != nil {
		t.Fatalf("parse redirect URL %q: %v", redirectURL, err)
	}

	code := u.Query().Get("code")
	if code == "" {
		t.Fatalf("redirect URL %q has no code param", redirectURL)
	}

	return code
}

// --- tests ---

func TestAuthorizeExchangeRoundtrip(t *testing.T) {
	s, authSvc := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	access, refresh, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, testVerifier)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if access == "" || refresh == "" {
		t.Fatal("Exchange returned empty tokens")
	}

	// the issued token must carry the granted scope, not the user's own
	// (broader) roles — user-alice's fakeUserLookup roles are ["admin"],
	// but only "read:resource" was requested/granted.
	claims, err := authSvc.ValidateAccessToken(ctx, access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if !claims.IsDelegated() {
		t.Error("IsDelegated() = false, want true for an OAuth-issued token")
	}

	if claims.ClientID != "internal-service" {
		t.Errorf("ClientID = %q, want internal-service", claims.ClientID)
	}

	if got := claims.Permissions(); len(got) != 1 || got[0] != "read:resource" {
		t.Errorf("Permissions() = %v, want [read:resource]", got)
	}

	if len(claims.Roles) != 0 {
		t.Errorf("Roles = %v, want empty — delegated token must not carry the user's own roles", claims.Roles)
	}
}

func TestExchangeRejectsReusedCode(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	if _, _, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, testVerifier); err != nil {
		t.Fatalf("first Exchange: %v", err)
	}

	if _, _, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, testVerifier); err == nil {
		t.Fatal("second Exchange with reused code: expected error, got nil")
	}
}

func TestExchangeRejectsWrongSecret(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	if _, _, err := s.Exchange(ctx, "internal-service", "wrong-secret", code, testRedirectURI, testVerifier); err == nil {
		t.Fatal("Exchange with wrong secret: expected error, got nil")
	}
}

func TestExchangeRejectsRedirectURIMismatch(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	if _, _, err := s.Exchange(ctx, "internal-service", "dev-secret", code, "http://evil.example/callback", testVerifier); err == nil {
		t.Fatal("Exchange with mismatched redirect_uri: expected error, got nil")
	}
}

func TestExchangeRejectsExpiredCode(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	// insert an already-expired code directly, bypassing Authorize's TTL
	codes := s.codes.(*fakeCodeStore)
	if err := codes.Save(ctx, "expired-code", AuthorizationCode{
		ClientID:      "internal-service",
		UserID:        "user-alice",
		RedirectURI:   testRedirectURI,
		Scope:         "read:resource",
		CodeChallenge: testChallenge,
		ExpiresAt:     time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, _, err := s.Exchange(ctx, "internal-service", "dev-secret", "expired-code", testRedirectURI, testVerifier); err == nil {
		t.Fatal("Exchange with expired code: expected error, got nil")
	}
}

func TestExchangeRejectsWrongVerifier(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	if _, _, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, "wrong-verifier-that-does-not-match"); err == nil {
		t.Fatal("Exchange with wrong code_verifier: expected error, got nil")
	}
}

func TestAuthorizeRejectsUnknownClient(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.Authorize(ctx, "no-such-client", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256"); err == nil {
		t.Fatal("Authorize with unknown client_id: expected error, got nil")
	}
}

func TestAuthorizeRejectsUnregisteredRedirectURI(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.Authorize(ctx, "internal-service", "http://evil.example/callback", "read:resource", "xyz", "user-alice", testChallenge, "S256"); err == nil {
		t.Fatal("Authorize with unregistered redirect_uri: expected error, got nil")
	}
}

func TestAuthorizeRejectsDisallowedScope(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.Authorize(ctx, "internal-service", testRedirectURI, "write:resource", "xyz", "user-alice", testChallenge, "S256"); err == nil {
		t.Fatal("Authorize with disallowed scope: expected error, got nil")
	}
}

func TestAuthorizeRejectsMissingCodeChallenge(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", "", "S256"); err == nil {
		t.Fatal("Authorize with empty code_challenge: expected error, got nil")
	}
}

func TestAuthorizeRejectsPlainMethod(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "plain"); err == nil {
		t.Fatal("Authorize with code_challenge_method=plain: expected error, got nil")
	}
}

func TestAuthorizeRequiresConsentForFlaggedClient(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if !result.ConsentRequired {
		t.Fatal("ConsentRequired = false, want true for a RequireConsent client")
	}

	if result.ConsentID == "" {
		t.Fatal("ConsentID is empty")
	}

	if result.RedirectURL != "" {
		t.Errorf("RedirectURL = %q, want empty when consent is pending", result.RedirectURL)
	}
}

func TestDecideApprove(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	redirectURL, err := s.Decide(ctx, result.ConsentID, true, "user-alice")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	code := extractCode(t, redirectURL)

	if _, _, err := s.Exchange(ctx, "partner-app", "partner-secret", code, testPartnerRedirectURI, testVerifier); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
}

func TestDecideDeny(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	redirectURL, err := s.Decide(ctx, result.ConsentID, false, "user-alice")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	u, err := url.Parse(redirectURL)
	if err != nil {
		t.Fatalf("parse redirect URL: %v", err)
	}

	if got := u.Query().Get("error"); got != "access_denied" {
		t.Errorf("error = %q, want access_denied", got)
	}

	if u.Query().Get("code") != "" {
		t.Error("denied decision must not carry a code")
	}
}

func TestDecideRejectsWrongUser(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if _, err := s.Decide(ctx, result.ConsentID, true, "user-bob"); err == nil {
		t.Fatal("Decide by a different user: expected error, got nil")
	}
}

func TestDecideRejectsReusedConsent(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	if _, err := s.Decide(ctx, result.ConsentID, true, "user-alice"); err != nil {
		t.Fatalf("first Decide: %v", err)
	}

	if _, err := s.Decide(ctx, result.ConsentID, true, "user-alice"); err == nil {
		t.Fatal("second Decide with reused consentID: expected error, got nil")
	}
}

func TestDecideApprovePreservesCodeChallenge(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "partner-app", testPartnerRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	redirectURL, err := s.Decide(ctx, result.ConsentID, true, "user-alice")
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	code := extractCode(t, redirectURL)

	// the code_challenge from the original Authorize call must have
	// survived the consent hop — a wrong verifier must still be rejected.
	if _, _, err := s.Exchange(ctx, "partner-app", "partner-secret", code, testPartnerRedirectURI, "wrong-verifier"); err == nil {
		t.Fatal("Exchange with wrong verifier after consent: expected error, got nil")
	}
}

func TestClientCredentials(t *testing.T) {
	s, authSvc := testService(t)
	ctx := context.Background()

	access, err := s.ClientCredentials(ctx, "internal-service", "dev-secret", "read:resource")
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}

	claims, err := authSvc.ValidateAccessToken(ctx, access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.Subject != "internal-service" || claims.ClientID != "internal-service" {
		t.Errorf("Subject/ClientID = %q/%q, want internal-service/internal-service", claims.Subject, claims.ClientID)
	}

	if got := claims.Permissions(); len(got) != 1 || got[0] != "read:resource" {
		t.Errorf("Permissions() = %v, want [read:resource]", got)
	}
}

func TestClientCredentialsRejectsBadSecret(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.ClientCredentials(ctx, "internal-service", "wrong-secret", "read:resource"); err == nil {
		t.Fatal("ClientCredentials with bad secret: expected error, got nil")
	}
}

func TestClientCredentialsRejectsUnallowedScope(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	if _, err := s.ClientCredentials(ctx, "internal-service", "dev-secret", "admin:everything"); err == nil {
		t.Fatal("ClientCredentials with unallowed scope: expected error, got nil")
	}
}

func TestRefreshToken(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	_, refresh, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, testVerifier)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	access2, refresh2, err := s.RefreshToken(ctx, "internal-service", "dev-secret", refresh)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}

	if access2 == "" || refresh2 == "" {
		t.Fatal("RefreshToken returned empty tokens")
	}

	// old refresh token must now be rejected (single use / rotation)
	if _, _, err := s.RefreshToken(ctx, "internal-service", "dev-secret", refresh); err == nil {
		t.Fatal("RefreshToken with already-used refresh token: expected error, got nil")
	}
}

func TestRefreshTokenRejectsBadSecret(t *testing.T) {
	s, _ := testService(t)
	ctx := context.Background()

	result, err := s.Authorize(ctx, "internal-service", testRedirectURI, "read:resource", "xyz", "user-alice", testChallenge, "S256")
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	code := extractCode(t, result.RedirectURL)

	_, refresh, err := s.Exchange(ctx, "internal-service", "dev-secret", code, testRedirectURI, testVerifier)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}

	if _, _, err := s.RefreshToken(ctx, "internal-service", "wrong-secret", refresh); err == nil {
		t.Fatal("RefreshToken with bad secret: expected error, got nil")
	}
}
