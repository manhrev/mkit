// Package secret loads application secrets from a secret manager into the
// process environment, so config code keeps reading plain env vars.
package secret

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Provider fetches one bundle of secrets as env-style key/value pairs.
type Provider interface {
	Fetch(ctx context.Context) (map[string]string, error)
	// FetchAndLoadToEnv fetches the bundle and sets every pair in the
	// process env, overriding variables already set (the secret is the
	// source of truth). Implementations call fetchAndLoadToEnv with themselves.
	FetchAndLoadToEnv(ctx context.Context) error
}

func fetchAndLoadToEnv(ctx context.Context, p Provider) error {
	m, err := p.Fetch(ctx)
	if err != nil {
		return err
	}
	for k, v := range m {
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("secret: set %s: %w", k, err)
		}
	}
	return nil
}

const (
	// EnvEnabled turns secret loading on (true/1); off when unset.
	EnvEnabled = "SECRET_ENABLED"
	// EnvGCPName names the env var holding the GCP secret version to load.
	EnvGCPName = "SECRET_GCP_NAME"
)

// FromEnv returns the provider the environment asks for, or nil when secret
// loading is off or no provider is configured. Its settings are plain env
// vars: they say where the secrets are, so they can't live inside them.
func FromEnv() Provider {
	if on, _ := strconv.ParseBool(os.Getenv(EnvEnabled)); !on {
		return nil
	}
	if n := os.Getenv(EnvGCPName); n != "" {
		return GCP{Name: n}
	}
	return nil
}

// ParseDotenv parses KEY=value text (comments, quotes and multi-line quoted
// values as godotenv accepts them), the format providers store bundles in.
func ParseDotenv(text string) (map[string]string, error) {
	m, err := godotenv.Unmarshal(text)
	if err != nil {
		return nil, fmt.Errorf("secret: parse dotenv: %w", err)
	}
	return m, nil
}
