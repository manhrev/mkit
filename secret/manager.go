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
// loading is off (EnvEnabled unset or empty). An EnvEnabled that is not a
// bool, or loading turned on with no provider configured, is an error: either
// would silently skip the secrets. Its settings are plain env vars: they say
// where the secrets are, so they can't live inside them.
func FromEnv() (Provider, error) {
	v := os.Getenv(EnvEnabled)
	if v == "" {
		return nil, nil
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return nil, fmt.Errorf("secret: %s: invalid value", EnvEnabled)
	}
	if !on {
		return nil, nil
	}
	if n := os.Getenv(EnvGCPName); n != "" {
		return GCP{Name: n}, nil
	}
	return nil, fmt.Errorf("secret: %s is on but no provider is configured (set %s)", EnvEnabled, EnvGCPName)
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
