package serviceerr

// Detail is one machine + human readable item returned by Error.Details(),
// e.g. one invalid-field entry out of a failed validation. Field/Code are
// stable and meant to be programmatically matched on by clients; Message is
// free-text for display, not for switching on.
type Detail struct {
	Field   string `json:"field,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// AddDetail appends one detail item (e.g. one invalid-field entry from
// validation). Code is optional — leave "" if there's no stable code for
// this particular detail yet.
func (e *Error) AddDetail(field, code, message string) *Error {
	e.details = append(e.details, Detail{Field: field, Code: code, Message: message})

	return e
}

// SetDetails replaces the detail list wholesale.
func (e *Error) SetDetails(details ...Detail) *Error {
	e.details = details

	return e
}
