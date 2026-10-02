// Package response holds response types shared across every API/transport
// (currently HTTP via huma; not tied to it) — the generic response
// envelope, not any one domain's DTOs (those live in api/dto).
package response

// EmptyData is the Output/Response data payload for operations with
// nothing to return (e.g. delete).
type EmptyData struct{}

// PaginatedData is the common data payload for list operations.
type PaginatedData[T any] struct {
	Items      []T `json:"items"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	TotalPages int `json:"totalPages"`
}
