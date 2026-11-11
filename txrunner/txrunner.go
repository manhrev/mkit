// Package txrunner runs multi-repo operations in a single Postgres
// transaction. A repo doesn't care whether its executor is the pool or a
// tx — both bobpgx.Pool and bobpgx.Tx satisfy bob.Executor — so Run just
// hands each repo's WithExecutor(exec) the tx instead of the pool for the
// duration of fn.
package txrunner

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stephenafamo/bob"
	bobpgx "github.com/stephenafamo/bob/drivers/pgx"
)

type Runner struct {
	pool bobpgx.Pool
}

func New(pool *pgxpool.Pool) *Runner {
	return &Runner{pool: bobpgx.NewPool(pool)}
}

// Option changes how one Run call behaves.
type Option func(*options)

type options struct {
	retries int
	tx      pgx.TxOptions
}

// WithRetry makes Run start over, up to retries more times, while fn or
// Commit fails with a serialization failure (SQLSTATE 40001), which
// CockroachDB returns under contention, as does Postgres at repeatable
// read or serializable. fn may then run more than once, so it must have no
// side effects outside the transaction and must not rely on variables it
// set in a previous attempt: the rollback undoes DB writes, not Go state.
// Reset any captured variable at the top of fn.
func WithRetry(retries int) Option {
	return func(o *options) { o.retries = retries }
}

// WithIsolation sets the transaction's isolation level; without it the
// database's default applies (read committed on Postgres, serializable on
// CockroachDB). Above read committed, pair it with WithRetry.
func WithIsolation(level pgx.TxIsoLevel) Option {
	return func(o *options) { o.tx.IsoLevel = level }
}

// WithReadOnly makes the transaction read only: a write inside fn fails.
func WithReadOnly() Option {
	return func(o *options) { o.tx.AccessMode = pgx.ReadOnly }
}

func newOptions(opts []Option) options {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// Run begins a transaction and calls fn with a bob.Executor bound to it.
// Commits if fn returns nil; rolls back (and re-panics) otherwise. Without
// opts fn runs once, in a read-write transaction at the default isolation.
func (r *Runner) Run(ctx context.Context, fn func(ctx context.Context, exec bob.Executor) error, opts ...Option) error {
	o := newOptions(opts)

	return retry(ctx, o.retries, func() error { return r.runOnce(ctx, o.tx, fn) })
}

// retry calls run, then up to retries more times while it fails with a
// serialization failure and ctx is alive.
func retry(ctx context.Context, retries int, run func() error) error {
	err := run()
	for range retries {
		if !isSerializationFailure(err) || ctx.Err() != nil {
			break
		}
		err = run()
	}

	return err
}

func (r *Runner) runOnce(ctx context.Context, txOpts pgx.TxOptions, fn func(ctx context.Context, exec bob.Executor) error) (err error) {
	tx, err := r.pool.BeginTx(ctx, txOpts)
	if err != nil {
		return fmt.Errorf("txrunner: begin: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return
		}
		err = tx.Commit(ctx)
	}()

	return fn(ctx, tx)
}

func isSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
