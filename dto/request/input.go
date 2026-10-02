// Package request holds request-side types shared across every
// API/transport's operations — the common query-param shapes (pagination),
// not any one domain's inputs (those live in api/dto).
package request

// Pagination is the common page/limit query params for list operations.
// Embed it in an operation's Input struct to get "page"/"limit" query
// params for free.
type Pagination struct {
	Page  int `query:"page" default:"1" minimum:"1" doc:"Page number (1-indexed)"`
	Limit int `query:"limit" default:"20" minimum:"1" maximum:"100" doc:"Page size"`
}

// SortOrder is the direction for a "sortOrder" query param. Sortable field
// names are resource-specific, so each operation's Input declares its own
// SortBy string with its own enum tag rather than embedding a shared type.
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)
