// Package httpclient is a fluent outbound HTTP client built on
// github.com/avast/retry-go/v4: Client.Req() starts a builder, chained
// SetX calls configure it, and Get/Post execute it.
package httpclient

import (
	"net/http"
	"time"
)

const (
	defaultTimeout    = 30 * time.Second
	defaultRetryCount = 3
)

type Client struct {
	client *http.Client
	config *Config
}

// NewClient builds a Client. A nil cfg, or a zero-value DefaultTimeout /
// DefaultRetryCount within it, falls back to package defaults — in
// particular DefaultRetryCount must not be left at its zero value, since
// retry-go treats retry.Attempts(0) as "retry forever", not "no retries".
func NewClient(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.DefaultTimeout == 0 {
		cfg.DefaultTimeout = defaultTimeout
	}
	if cfg.DefaultRetryCount == 0 {
		cfg.DefaultRetryCount = defaultRetryCount
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// customize transport here (connection pool size, TLS, ...) if needed;
	// starting from DefaultTransport keeps its proxy/dial/idle-conn defaults.

	return &Client{
		client: &http.Client{
			Transport: transport,
		},
		config: cfg,
	}
}

// Req starts a new request builder seeded with this client's defaults.
func (c *Client) Req() *Request {
	return &Request{
		headers:      make(map[string][]string),
		queries:      make(map[string]string),
		client:       c,
		timeout:      c.config.DefaultTimeout,
		retryCount:   c.config.DefaultRetryCount,
		requestBody:  nil,
		responseBody: nil,
	}
}

// GetHTTPClient returns the underlying *http.Client.
func (c *Client) GetHTTPClient() *http.Client {
	return c.client
}
