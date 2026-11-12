package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Reader reads environment variables into typed values and collects every
// error, so one startup reports all bad keys. The zero value is ready to use.
type Reader struct{ errs []error }

// Var is one key being read: chain Required/RequiredIf, end with a getter.
type Var struct {
	r        *Reader
	key, val string
}

// Key starts reading key. Empty counts as unset: deploy tools pass FOO= for unset vars.
func (r *Reader) Key(key string) Var { return Var{r, key, os.Getenv(key)} }

// Err returns every recorded error joined, or nil.
func (r *Reader) Err() error { return errors.Join(r.errs...) }

// Required records an error when the key is unset or empty.
func (v Var) Required() Var { return v.RequiredIf(true) }

// RequiredIf is Required only when cond holds, e.g. only on Cloud Run.
func (v Var) RequiredIf(cond bool) Var {
	if cond && v.val == "" {
		v.r.errs = append(v.r.errs, fmt.Errorf("%s: required", v.key))
	}
	return v
}

func (v Var) String(def string) string {
	if v.val == "" {
		return def
	}
	return v.val
}

func (v Var) Int(def int) int                          { return v.As(def, strconv.Atoi) }
func (v Var) Bool(def bool) bool                       { return v.As(def, strconv.ParseBool) }
func (v Var) Duration(def time.Duration) time.Duration { return v.As(def, time.ParseDuration) }

// List splits a comma-separated value, trimming and dropping blanks.
func (v Var) List(def ...string) []string {
	var out []string
	for _, p := range strings.Split(v.val, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if out == nil {
		return def
	}
	return out
}

// As parses the value for types without a getter.
// A value that does not parse records an error and returns def.
func (v Var) As[T any](def T, parse func(string) (T, error)) T {
	if v.val == "" {
		return def
	}
	x, err := parse(v.val)
	if err != nil {
		// key only: parse errors quote the value, which may be a secret
		v.r.errs = append(v.r.errs, fmt.Errorf("%s: invalid value", v.key))
		return def
	}
	return x
}
