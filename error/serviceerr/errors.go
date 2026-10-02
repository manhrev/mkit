package serviceerr

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Base error
var (
	ErrInvalidArgument    = fmt.Errorf("invalid argument")
	ErrNotFound           = fmt.Errorf("not found")
	ErrInternal           = fmt.Errorf("internal server error")
	ErrPermissionDenied   = fmt.Errorf("permission denied")
	ErrUnauthenticated    = fmt.Errorf("unauthenticated")
	ErrConflict           = fmt.Errorf("conflict")
	ErrUnavailable        = fmt.Errorf("unavailable")         // downstream/dependency down; safe to retry
	ErrDeadlineExceeded   = fmt.Errorf("deadline exceeded")   // request/context timeout
	ErrResourceExhausted  = fmt.Errorf("resource exhausted")  // rate limit / quota
	ErrFailedPrecondition = fmt.Errorf("failed precondition") // state not ready for this op
)

// Error is a custom error type, basically Base error but with custom info.
// Internal-only: never JSON-marshaled directly, always converted to a
// transport-specific response later (huma error model, gRPC status
// details, ...) — hence every field is private, read only via the
// accessors below or the Set*/Add* builders.
//
// Transport codes (HTTP status, gRPC code) are derived from which base
// error it wraps, so one Error works for both REST (via GetStatus, huma's
// StatusError interface) and gRPC (via GRPCStatus, the interface
// google.golang.org/grpc/status.FromError checks for). Use
// SetHTTPStatus/SetGRPCCode to override the derived code for the rare case
// a specific instance needs a nonstandard one.
type Error struct {
	message    string
	details    []Detail
	err        error
	httpStatus *int
	grpcCode   *codes.Code
}

// NewError wraps wrappedErr as an *Error. Panics if wrappedErr is nil —
// there's no meaningful status/message to derive from no error at all;
// use one of the New* helpers below instead of calling this directly.
func NewError(wrappedErr error) *Error {
	if wrappedErr == nil {
		panic("serviceerr: NewError called with nil error")
	}
	return &Error{
		err: wrappedErr,
	}
}

func (e *Error) Error() string {
	return e.err.Error()
}

func (e *Error) Unwrap() error {
	return e.err
}

// Message returns the human-facing message set via SetMessage.
func (e *Error) Message() string {
	return e.message
}

// Details returns the machine + human readable detail items set via
// AddDetail/SetDetails.
func (e *Error) Details() []Detail {
	return e.details
}

func (e *Error) SetMessage(message string) *Error {
	e.message = message

	return e
}

// SetHTTPStatus overrides the HTTP status GetStatus would otherwise derive
// from the wrapped base error.
func (e *Error) SetHTTPStatus(status int) *Error {
	e.httpStatus = &status

	return e
}

// SetGRPCCode overrides the gRPC code GRPCStatus would otherwise derive
// from the wrapped base error.
func (e *Error) SetGRPCCode(code codes.Code) *Error {
	e.grpcCode = &code

	return e
}

// GetStatus returns the HTTP status code for this error. Implements huma's
// StatusError interface, so returning *Error from a huma operation handler
// renders with the right status automatically.
func (e *Error) GetStatus() int {
	if e.httpStatus != nil {
		return *e.httpStatus
	}

	switch {
	case errors.Is(e.err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(e.err, ErrInvalidArgument):
		return http.StatusBadRequest
	case errors.Is(e.err, ErrConflict):
		return http.StatusConflict
	case errors.Is(e.err, ErrPermissionDenied):
		return http.StatusForbidden
	case errors.Is(e.err, ErrUnauthenticated):
		return http.StatusUnauthorized
	case errors.Is(e.err, ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(e.err, ErrDeadlineExceeded):
		return http.StatusGatewayTimeout
	case errors.Is(e.err, ErrResourceExhausted):
		return http.StatusTooManyRequests
	case errors.Is(e.err, ErrFailedPrecondition):
		return http.StatusPreconditionFailed
	case errors.Is(e.err, ErrInternal):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// GRPCStatus returns the gRPC status for this error. Implements the
// interface google.golang.org/grpc/status.FromError checks for, so
// returning *Error from a gRPC handler renders with the right code
// automatically.
func (e *Error) GRPCStatus() *status.Status {
	if e.grpcCode != nil {
		return status.New(*e.grpcCode, e.message)
	}

	switch {
	case errors.Is(e.err, ErrNotFound):
		return status.New(codes.NotFound, e.message)
	case errors.Is(e.err, ErrInvalidArgument):
		return status.New(codes.InvalidArgument, e.message)
	case errors.Is(e.err, ErrConflict):
		return status.New(codes.AlreadyExists, e.message)
	case errors.Is(e.err, ErrPermissionDenied):
		return status.New(codes.PermissionDenied, e.message)
	case errors.Is(e.err, ErrUnauthenticated):
		return status.New(codes.Unauthenticated, e.message)
	case errors.Is(e.err, ErrUnavailable):
		return status.New(codes.Unavailable, e.message)
	case errors.Is(e.err, ErrDeadlineExceeded):
		return status.New(codes.DeadlineExceeded, e.message)
	case errors.Is(e.err, ErrResourceExhausted):
		return status.New(codes.ResourceExhausted, e.message)
	case errors.Is(e.err, ErrFailedPrecondition):
		return status.New(codes.FailedPrecondition, e.message)
	case errors.Is(e.err, ErrInternal):
		return status.New(codes.Internal, e.message)
	default:
		return status.New(codes.Internal, e.message)
	}
}

func NewInternal(err error) *Error {
	jErr := errors.Join(err, ErrInternal)

	return NewError(jErr).
		SetMessage("Internal server error.")
}

func NewNotFound(err error) *Error {
	jErr := errors.Join(err, ErrNotFound)

	return NewError(jErr).
		SetMessage("Not found.")
}

func NewInvalidArgument(err error) *Error {
	jErr := errors.Join(err, ErrInvalidArgument)

	return NewError(jErr).
		SetMessage("Invalid argument.")
}

func NewUnauthenticated(err error) *Error {
	jErr := errors.Join(err, ErrUnauthenticated)

	return NewError(jErr).
		SetMessage("Unauthenticated.")
}

func NewPermissionDenied(err error) *Error {
	jErr := errors.Join(err, ErrPermissionDenied)

	return NewError(jErr).
		SetMessage("Permission denied.")
}

func NewConflict(err error) *Error {
	jErr := errors.Join(err, ErrConflict)

	return NewError(jErr).
		SetMessage("Conflict.")
}

func NewUnavailable(err error) *Error {
	jErr := errors.Join(err, ErrUnavailable)

	return NewError(jErr).
		SetMessage("Service unavailable, please retry.")
}

func NewDeadlineExceeded(err error) *Error {
	jErr := errors.Join(err, ErrDeadlineExceeded)

	return NewError(jErr).
		SetMessage("Deadline exceeded.")
}

func NewResourceExhausted(err error) *Error {
	jErr := errors.Join(err, ErrResourceExhausted)

	return NewError(jErr).
		SetMessage("Resource exhausted.")
}

func NewFailedPrecondition(err error) *Error {
	jErr := errors.Join(err, ErrFailedPrecondition)

	return NewError(jErr).
		SetMessage("Failed precondition.")
}
