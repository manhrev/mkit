package secret

import (
	"context"
	"encoding/base64"
	"fmt"

	"google.golang.org/api/option"
	"google.golang.org/api/secretmanager/v1"
)

// GCP reads a dotenv-formatted Google Secret Manager secret version.
// Credentials come from Application Default Credentials (Cloud Run's runtime
// service account), which needs roles/secretmanager.secretAccessor on it.
type GCP struct {
	// Name is the full version resource name, e.g.
	// projects/my-project/secrets/my-app-prod/versions/latest.
	// shortcut: global secrets only (regional ones need a regional endpoint), add one if needed.
	Name string
	// Options are extra client options (tests point it at a fake endpoint).
	Options []option.ClientOption
}

func (g GCP) FetchAndLoadToEnv(ctx context.Context) error { return fetchAndLoadToEnv(ctx, g) }

func (g GCP) Fetch(ctx context.Context) (map[string]string, error) {
	svc, err := secretmanager.NewService(ctx, g.Options...)
	if err != nil {
		return nil, fmt.Errorf("secret: gcp client: %w", err)
	}
	res, err := svc.Projects.Secrets.Versions.Access(g.Name).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("secret: gcp access %s: %w", g.Name, err)
	}
	if res.Payload == nil {
		return nil, fmt.Errorf("secret: gcp access %s: empty payload", g.Name)
	}
	data, err := base64.StdEncoding.DecodeString(res.Payload.Data)
	if err != nil {
		return nil, fmt.Errorf("secret: gcp decode %s: %w", g.Name, err)
	}
	return ParseDotenv(string(data))
}
