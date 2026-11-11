package txrunner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsSerializationFailure(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("boom"), false},
		{&pgconn.PgError{Code: "23505"}, false},
		{&pgconn.PgError{Code: "40001"}, true},
		{fmt.Errorf("repo: %w", &pgconn.PgError{Code: "40001"}), true},
	}
	for _, c := range cases {
		if got := isSerializationFailure(c.err); got != c.want {
			t.Errorf("isSerializationFailure(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestRetry(t *testing.T) {
	conflict := fmt.Errorf("repo: %w", &pgconn.PgError{Code: "40001"})
	boom := errors.New("boom")
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name    string
		ctx     context.Context
		retries int
		errs    []error // result of each run, in order
		calls   int
		want    error
	}{
		{"no option runs once", context.Background(), 0, []error{conflict}, 1, conflict},
		{"conflict then ok", context.Background(), 2, []error{conflict, nil}, 2, nil},
		{"gives up after retries", context.Background(), 2, []error{conflict, conflict, conflict}, 3, conflict},
		{"other error not retried", context.Background(), 2, []error{boom}, 1, boom},
		{"canceled ctx not retried", canceled, 2, []error{conflict}, 1, conflict},
	}
	for _, c := range cases {
		calls := 0
		err := retry(c.ctx, c.retries, func() error {
			calls++
			return c.errs[calls-1]
		})
		if calls != c.calls || !errors.Is(err, c.want) || (c.want == nil && err != nil) {
			t.Errorf("%s: calls = %d, err = %v; want %d, %v", c.name, calls, err, c.calls, c.want)
		}
	}
}

func TestOptions(t *testing.T) {
	if o := newOptions(nil); o.retries != 0 || o.tx != (pgx.TxOptions{}) {
		t.Errorf("no options = %+v, want the zero value", o)
	}
	o := newOptions([]Option{WithRetry(3), WithIsolation(pgx.Serializable), WithReadOnly()})
	if o.retries != 3 || o.tx.IsoLevel != pgx.Serializable || o.tx.AccessMode != pgx.ReadOnly {
		t.Errorf("options = %+v", o)
	}
}
