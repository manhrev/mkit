// Package repoerr holds the generic sentinel errors a repository/store
// implementation returns to its service layer. These should only ever be
// returned from the repository layer — never constructed or checked
// against outside it.
//
// These are the generic, storage-agnostic cases only. A repo needing a
// more specific error (e.g. "username already taken", detected via a DB
// unique-constraint violation) should define that error itself, in its own
// package, rather than adding a one-off case here.
package repoerr

import "fmt"

var (
	ErrNotFound = fmt.Errorf("not found")
	ErrExisted  = fmt.Errorf("existed")
)
