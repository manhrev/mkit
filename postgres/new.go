// Package postgres connects to Postgres via pgxpool. *pgxpool.Pool is the
// only connection type exposed — Migrate needs a database/sql handle
// internally to satisfy golang-migrate's driver API, but that handle is
// opened from the pool, scoped to the function, and never returned.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/manhrev/mkit/config"

	"github.com/exaring/otelpgx"
	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// DefaultMigrationsPath is used by New when cfg.IsMigrateSchema is set.
const DefaultMigrationsPath = "migrations"

// New connects to Postgres via pgxpool and, if cfg.IsMigrateSchema is set,
// runs schema migrations before returning.
func New(ctx context.Context, appCfg *config.App, logger *slog.Logger) (*pgxpool.Pool, error) {
	cfg := appCfg.Postgres
	if cfg == nil || cfg.ConnectionParams == nil {
		return nil, fmt.Errorf("postgres: config is nil")
	}
	cp := cfg.ConnectionParams

	// Built via net/url (not raw Sprintf interpolation) so a password
	// containing special characters (space, @, :, ...) doesn't corrupt the
	// connection string.
	dsn := (&url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cp.User, cp.Password),
		Host:     fmt.Sprintf("%s:%d", cp.Host, cp.Port),
		Path:     "/" + cp.DBName,
		RawQuery: url.Values{"sslmode": {cp.SSLMode}}.Encode(),
	}).String()

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: unable to parse pool config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.MaxOpenConn)
	poolConfig.MinConns = int32(cfg.MaxIdleConn) // pgxpool has no direct MaxIdleConn analog; MinConns is the closest fit

	// Safe unconditionally: when tracing is disabled, no TracerProvider is
	// ever registered, so otelpgx falls back to the global no-op provider
	// (negligible overhead), matching the otelhttp wiring in serve.go.
	poolConfig.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithDisableAcquireTracer())

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("postgres: failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: failed to connect: %w", err)
	}

	if cfg.IsMigrateSchema {
		if err := Migrate(pool, DefaultMigrationsPath, logger); err != nil {
			pool.Close()
			return nil, fmt.Errorf("postgres: error migrating database: %w", err)
		}
	}

	return pool, nil
}

// Migrate runs schema migrations found under migrationsPath against pool.
// It opens a database/sql handle from the pool (golang-migrate's driver API
// requires one), scoped to this call only.
func Migrate(pool *pgxpool.Pool, migrationsPath string, logger *slog.Logger) error {
	logger.Info("postgres: running migrations", "path", migrationsPath)

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	driver, err := migratepgx.WithInstance(db, &migratepgx.Config{})
	if err != nil {
		return fmt.Errorf("postgres: init migrate driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "pgx", driver)
	if err != nil {
		return fmt.Errorf("postgres: init migrate instance: %w", err)
	}

	if err := m.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("postgres: no pending migrations")
		} else {
			return fmt.Errorf("postgres: run migrations: %w", err)
		}
	} else {
		version, _, verErr := m.Version()
		if verErr != nil {
			logger.Info("postgres: migrations applied")
		} else {
			logger.Info("postgres: migrations applied", "version", version)
		}
	}

	srcErr, dbErr := m.Close()
	if srcErr != nil {
		return fmt.Errorf("postgres: close migration source: %w", srcErr)
	}
	if dbErr != nil {
		return fmt.Errorf("postgres: close migration db: %w", dbErr)
	}

	return nil
}
