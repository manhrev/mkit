package httpclient

import "time"

// Config holds NewClient's defaults, applied to every Request built via
// Client.Req unless overridden with SetTimeout/SetRetry.
type Config struct {
	DefaultTimeout    time.Duration
	DefaultRetryCount int
}
