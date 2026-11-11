// Package httpclient is a fluent outbound HTTP client built on
// github.com/avast/retry-go/v4: Client.Req() starts a builder, chained
// SetX calls configure it, and Get[T]/Post[T] execute it and decode the
// JSON response into T. Requests are traced via otelhttp.
package httpclient

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	defaultTimeout     = 30 * time.Second
	defaultRetryCount  = 3
	defaultMaxBodySize = 10 << 20 // 10 MiB
)

type Client struct {
	client      *http.Client
	timeout     time.Duration
	retryCount  int
	maxBodySize int64
}

// Option sets one of a Client's defaults, which every Request built via
// Client.Req starts from.
type Option func(*Client)

// WithTimeout sets the default per-request timeout (default 30s); see
// Request.SetTimeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) { c.timeout = timeout }
}

// WithRetry sets the default retry count (default 3). <= 0 means no
// retries; see Request.SetRetry.
func WithRetry(retryCount int) Option {
	return func(c *Client) { c.retryCount = retryCount }
}

// WithMaxBodySize caps how many response body bytes are read (default
// 10 MiB); a larger body fails with ErrResponseTooLarge. See
// Request.SetMaxBodySize.
func WithMaxBodySize(maxBodySize int64) Option {
	return func(c *Client) { c.maxBodySize = maxBodySize }
}

// NewClient builds a Client with the package defaults, changed by opts.
func NewClient(opts ...Option) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// customize transport here (connection pool size, TLS, ...) if needed;
	// starting from DefaultTransport keeps its proxy/dial/idle-conn defaults.

	c := &Client{
		client: &http.Client{
			// one client span per attempt, with traceparent injected via the
			// global propagator (no-op until tracing.NewService sets it)
			Transport: otelhttp.NewTransport(transport),
		},
		timeout:     defaultTimeout,
		retryCount:  defaultRetryCount,
		maxBodySize: defaultMaxBodySize,
	}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Req starts a new request builder seeded with this client's defaults.
func (c *Client) Req() *Request {
	return &Request{
		headers:     make(map[string][]string),
		queries:     make(map[string]string),
		client:      c,
		timeout:     c.timeout,
		retryCount:  c.retryCount,
		maxBodySize: c.maxBodySize,
		requestBody: nil,
	}
}

// GetHTTPClient returns the underlying *http.Client.
func (c *Client) GetHTTPClient() *http.Client {
	return c.client
}
