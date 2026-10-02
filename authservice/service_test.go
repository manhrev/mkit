package authservice

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/manhrev/mkit/jwtmanager"
)

// --- fakes ---

type fakeVerifier struct {
	users map[string]string // username -> password
}

func (f *fakeVerifier) Verify(_ context.Context, username, password string) (string, error) {
	want, ok := f.users[username]
	if !ok || want != password {
		return "", errors.New("bad credentials")
	}

	return "user-" + username, nil
}

type fakeUserLookup struct {
	roles map[string][]string // userID -> roles
}

func (f *fakeUserLookup) RolesByUserID(_ context.Context, userID string) ([]string, error) {
	return f.roles[userID], nil
}

type fakeStore struct {
	mu      sync.Mutex
	records map[string]string // jti -> userID
}

func newFakeStore() *fakeStore { return &fakeStore{records: map[string]string{}} }

func (f *fakeStore) Save(_ context.Context, jti, userID string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records[jti] = userID

	return nil
}

func (f *fakeStore) Get(_ context.Context, jti string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	userID, ok := f.records[jti]
	if !ok {
		return "", errors.New("not found")
	}

	return userID, nil
}

func (f *fakeStore) Delete(_ context.Context, jti string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.records, jti)

	return nil
}

type fakeBlocklist struct {
	mu      sync.Mutex
	blocked map[string]bool
}

func newFakeBlocklist() *fakeBlocklist { return &fakeBlocklist{blocked: map[string]bool{}} }

func (f *fakeBlocklist) Block(_ context.Context, jti string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocked[jti] = true

	return nil
}

func (f *fakeBlocklist) IsBlocked(_ context.Context, jti string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.blocked[jti], nil
}

// --- setup ---

func testAuthService(t *testing.T) *Service {
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
		Issuer:               "authservice-test",
	})
	if err != nil {
		t.Fatalf("jwtmanager.New: %v", err)
	}

	verifier := &fakeVerifier{users: map[string]string{"alice": "hunter2"}}
	users := &fakeUserLookup{roles: map[string][]string{"user-alice": {"admin"}}}

	return New(jwtSvc, verifier, users, newFakeStore(), newFakeBlocklist())
}

// --- tests ---

func TestLoginSuccess(t *testing.T) {
	s := testAuthService(t)

	access, refresh, err := s.Login(context.Background(), "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	claims, err := s.ValidateAccessToken(context.Background(), access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.Subject != "user-alice" {
		t.Errorf("Subject = %q, want user-alice", claims.Subject)
	}

	if len(claims.Roles) != 1 || claims.Roles[0] != "admin" {
		t.Errorf("Roles = %v, want [admin]", claims.Roles)
	}

	if refresh == "" {
		t.Error("refresh token empty")
	}
}

func TestLoginBadPassword(t *testing.T) {
	s := testAuthService(t)

	if _, _, err := s.Login(context.Background(), "alice", "wrong"); err == nil {
		t.Fatal("Login with bad password: expected error, got nil")
	}
}

func TestRefreshRotates(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh1, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	access2, refresh2, err := s.Refresh(ctx, refresh1)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if _, err := s.ValidateAccessToken(ctx, access2); err != nil {
		t.Fatalf("ValidateAccessToken(access2): %v", err)
	}

	if refresh2 == refresh1 {
		t.Error("refresh2 == refresh1, expected rotation to a new token")
	}

	// old refresh token must now be rejected (single use / rotation)
	if _, _, err := s.Refresh(ctx, refresh1); err == nil {
		t.Fatal("Refresh with already-used refresh token: expected error, got nil")
	}

	// new refresh token must still work
	if _, _, err := s.Refresh(ctx, refresh2); err != nil {
		t.Fatalf("Refresh with rotated token: %v", err)
	}
}

func TestRefreshRejectsAccessToken(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	access, _, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, _, err := s.Refresh(ctx, access); err == nil {
		t.Fatal("Refresh with access token: expected error, got nil")
	}
}

func TestRefreshUnknownJTI(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// simulate revoke/logout: drop the record before it's used
	svc := s
	claims, err := svc.jwt.Verify(refresh, jwtmanager.TokenTypeRefresh)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if err := svc.store.Delete(ctx, claims.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, _, err := s.Refresh(ctx, refresh); err == nil {
		t.Fatal("Refresh with revoked refresh token: expected error, got nil")
	}
}

func TestRevokeRefreshToken(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if err := s.RevokeRefreshToken(ctx, refresh); err != nil {
		t.Fatalf("RevokeRefreshToken: %v", err)
	}

	if _, _, err := s.Refresh(ctx, refresh); err == nil {
		t.Fatal("Refresh with revoked refresh token: expected error, got nil")
	}
}

func TestIssueAccessForClient(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	access, err := s.IssueAccessForClient(ctx, "partner-app", "read:resource")
	if err != nil {
		t.Fatalf("IssueAccessForClient: %v", err)
	}

	claims, err := s.ValidateAccessToken(ctx, access)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.Subject != "partner-app" {
		t.Errorf("Subject = %q, want partner-app", claims.Subject)
	}

	if claims.ClientID != "partner-app" {
		t.Errorf("ClientID = %q, want partner-app", claims.ClientID)
	}

	if got := claims.Permissions(); len(got) != 1 || got[0] != "read:resource" {
		t.Errorf("Permissions() = %v, want [read:resource]", got)
	}
}

func TestIssueForClientThenRefreshRejected(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh, err := s.IssueForClient(ctx, "user-alice", "internal-service", "read:resource")
	if err != nil {
		t.Fatalf("IssueForClient: %v", err)
	}

	// a delegated refresh token must not be redeemable at /auth/refresh —
	// that path always refetches full user roles (RFC 6749 §6 violation
	// otherwise).
	if _, _, err := s.Refresh(ctx, refresh); err == nil {
		t.Fatal("Refresh with delegated refresh token: expected error, got nil")
	}
}

func TestRefreshForClient(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh1, err := s.IssueForClient(ctx, "user-alice", "internal-service", "read:resource")
	if err != nil {
		t.Fatalf("IssueForClient: %v", err)
	}

	access2, refresh2, err := s.RefreshForClient(ctx, "internal-service", refresh1)
	if err != nil {
		t.Fatalf("RefreshForClient: %v", err)
	}

	claims, err := s.ValidateAccessToken(ctx, access2)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}

	if claims.Subject != "user-alice" {
		t.Errorf("Subject = %q, want user-alice", claims.Subject)
	}

	if claims.ClientID != "internal-service" {
		t.Errorf("ClientID = %q, want internal-service", claims.ClientID)
	}

	if got := claims.Permissions(); len(got) != 1 || got[0] != "read:resource" {
		t.Errorf("Permissions() = %v, want [read:resource] — must carry forward unchanged, not refetch roles", got)
	}

	if len(claims.Roles) != 0 {
		t.Errorf("Roles = %v, want empty — RefreshForClient must never grant the user's full roles", claims.Roles)
	}

	// old refresh token must now be rejected (single use / rotation)
	if _, _, err := s.RefreshForClient(ctx, "internal-service", refresh1); err == nil {
		t.Fatal("RefreshForClient with already-used refresh token: expected error, got nil")
	}

	// rotated refresh token must still work
	if _, _, err := s.RefreshForClient(ctx, "internal-service", refresh2); err != nil {
		t.Fatalf("RefreshForClient with rotated token: %v", err)
	}
}

func TestRefreshForClientRejectsWrongClient(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh, err := s.IssueForClient(ctx, "user-alice", "internal-service", "read:resource")
	if err != nil {
		t.Fatalf("IssueForClient: %v", err)
	}

	if _, _, err := s.RefreshForClient(ctx, "partner-app", refresh); err == nil {
		t.Fatal("RefreshForClient with mismatched clientID: expected error, got nil")
	}
}

func TestRefreshForClientRejectsPlainLoginToken(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	_, refresh, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, _, err := s.RefreshForClient(ctx, "internal-service", refresh); err == nil {
		t.Fatal("RefreshForClient with plain login refresh token: expected error, got nil")
	}
}

func TestRevokeAccessToken(t *testing.T) {
	s := testAuthService(t)
	ctx := context.Background()

	access, _, err := s.Login(ctx, "alice", "hunter2")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if _, err := s.ValidateAccessToken(ctx, access); err != nil {
		t.Fatalf("ValidateAccessToken before revoke: %v", err)
	}

	if err := s.RevokeAccessToken(ctx, access); err != nil {
		t.Fatalf("RevokeAccessToken: %v", err)
	}

	if _, err := s.ValidateAccessToken(ctx, access); err == nil {
		t.Fatal("ValidateAccessToken after revoke: expected error, got nil")
	}
}
