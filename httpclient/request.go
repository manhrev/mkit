package httpclient

import (
	"net/http"
	"time"
)

// Response is the result of an executed Request. Body holds whatever value
// was passed to SetResult, decoded in place; it's nil if SetResult was never
// called.
type Response struct {
	Body        any
	StatusCode  int
	RawResponse *http.Response
}

// Request is a fluent builder for a single outbound HTTP call. Build one via
// Client.Req, chain SetX calls, then execute with Get or Post.
type Request struct {
	client       *Client
	url          string
	headers      http.Header
	queries      map[string]string
	requestBody  any
	responseBody any
	timeout      time.Duration
	retryCount   int
}

// SetBody marshals body as the JSON request body and sets Content-Type.
func (r *Request) SetBody(body any) *Request {
	r.requestBody = body
	r.headers["Content-Type"] = []string{"application/json"}

	return r
}

// SetHeader adds a single header value, keeping any already set under key.
func (r *Request) SetHeader(key, value string) *Request {
	r.headers[key] = append(r.headers[key], value)

	return r
}

// SetQuery sets a single query parameter.
func (r *Request) SetQuery(key, value string) *Request {
	r.queries[key] = value

	return r
}

// SetHeaders replaces all headers set so far.
func (r *Request) SetHeaders(header http.Header) *Request {
	r.headers = header

	return r
}

// SetQueries replaces all query parameters set so far.
func (r *Request) SetQueries(queries map[string]string) *Request {
	r.queries = queries

	return r
}

// SetTimeout overrides the client's default per-request timeout.
func (r *Request) SetTimeout(timeout time.Duration) *Request {
	r.timeout = timeout

	return r
}

// SetRetry overrides the client's default retry count. <= 0 means no
// retries — it does not mean "unlimited" (unlike retry-go's own default).
func (r *Request) SetRetry(retryCount int) *Request {
	r.retryCount = retryCount

	return r
}

// SetResult sets the destination that a successful 2xx JSON response body
// is decoded into.
func (r *Request) SetResult(body any) *Request {
	r.responseBody = body

	return r
}
