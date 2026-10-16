package secret

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"google.golang.org/api/option"
)

type fake map[string]string

func (f fake) Fetch(context.Context) (map[string]string, error) { return f, nil }
func (f fake) FetchAndLoadToEnv(ctx context.Context) error      { return fetchAndLoadToEnv(ctx, f) }

func TestFetchAndLoadToEnvOverridesEnv(t *testing.T) {
	t.Setenv("SECRET_TEST_KEEP", "real")
	os.Unsetenv("SECRET_TEST_NEW")
	t.Cleanup(func() { os.Unsetenv("SECRET_TEST_NEW") })

	p := fake{"SECRET_TEST_KEEP": "secret", "SECRET_TEST_NEW": "v"}
	if err := p.FetchAndLoadToEnv(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SECRET_TEST_KEEP"); got != "secret" {
		t.Fatalf("existing env not overridden: %q", got)
	}
	if got := os.Getenv("SECRET_TEST_NEW"); got != "v" {
		t.Fatalf("new env not set: %q", got)
	}
}

func TestGCPFetch(t *testing.T) {
	const name = "projects/p/secrets/s/versions/latest"
	body := "A=1\nPASS='p@ss$word'\nKEY=\"l1\nl2\"\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/"+name+":access" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"name":%q,"payload":{"data":%q}}`, name, base64.StdEncoding.EncodeToString([]byte(body)))
	}))
	defer srv.Close()

	m, err := GCP{Name: name, Options: []option.ClientOption{
		option.WithEndpoint(srv.URL), option.WithoutAuthentication(),
	}}.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "1", "PASS": "p@ss$word", "KEY": "l1\nl2"}
	for k, v := range want {
		if m[k] != v {
			t.Fatalf("%s = %q, want %q", k, m[k], v)
		}
	}
}

func TestParseDotenvRejectsJSON(t *testing.T) {
	if _, err := ParseDotenv(`{"A":"1"}`); err == nil {
		t.Fatal("expected error for JSON input")
	}
}

func TestFromEnv(t *testing.T) {
	const name = "projects/p/secrets/s/versions/latest"
	t.Setenv(EnvGCPName, name)

	t.Setenv(EnvEnabled, "")
	if p := FromEnv(); p != nil {
		t.Fatalf("want nil when disabled, got %v", p)
	}
	t.Setenv(EnvEnabled, "true")
	if p, ok := FromEnv().(GCP); !ok || p.Name != name {
		t.Fatalf("want GCP provider, got %v", p)
	}
	t.Setenv(EnvGCPName, "")
	if p := FromEnv(); p != nil {
		t.Fatalf("want nil without provider config, got %v", p)
	}
}
