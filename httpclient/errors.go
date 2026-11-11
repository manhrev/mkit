package httpclient

import "fmt"

var (
	ErrRequestTimeout           = fmt.Errorf("request timeout")
	ErrCannotDecodeResponseBody = fmt.Errorf("cannot decode response body")
	ErrResponseTooLarge         = fmt.Errorf("response body exceeds max size")
	errStatusCodeShouldRetry    = fmt.Errorf("status code should retry")
)
