// Package txrunner runs multi-repo operations in a single Postgres
// transaction. A repo doesn't care whether its executor is the pool or a
// tx — both bobpgx.Pool and bobpgx.Tx satisfy bob.Executor — so Run just
// hands each repo's WithExecutor(exec) the tx instead of the pool for the
// duration of fn.
package txrunner

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// Run begins a transaction and calls fn with a bob.Executor bound to it.
// Commits if fn returns nil; rolls back (and re-panics) otherwise.
func (r *Runner) Run(ctx context.Context, fn func(ctx context.Context, exec bob.Executor) error) (err error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
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
