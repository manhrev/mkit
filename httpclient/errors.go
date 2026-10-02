package httpclient

import "fmt"

var (
	ErrRequestTimeout           = fmt.Errorf("request timeout")
	ErrCannotDecodeResponseBody = fmt.Errorf("cannot decode response body")
	errStatusCodeShouldRetry    = fmt.Errorf("status code should retry")
)
