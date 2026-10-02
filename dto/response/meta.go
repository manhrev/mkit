package response

import "time"

// Meta accompanies every response body's data. Fields are populated by
// request-scoped middleware (request ID, timing) and the active trace
// span (trace/span ID) — not part of this type's own responsibility.
type Meta struct {
	RequestID  string    `json:"requestId"`
	TraceID    string    `json:"traceId"`
	SpanID     string    `json:"spanId"`
	Version    string    `json:"version"`
	RequestAt  time.Time `json:"requestAt"`
	ResponseAt time.Time `json:"responseAt"`
}
