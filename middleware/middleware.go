// Package middleware holds net/http middleware shared across the app's
// HTTP entrypoint (request metadata, CORS, logging).
package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/manhrev/mkit/dto/response"
	applog "github.com/manhrev/mkit/log"
)

// Metadata stamps a response.Meta into the request context: a fresh
// RequestID, the active span's TraceID/SpanID (zero if tracing is
// disabled or no span is active), the app version, and the request's
// start time. It does not itself write anything into a response body —
// callers read it back via response.MetaFromContext.
func Metadata(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sc := trace.SpanContextFromContext(r.Context())

			meta := response.Meta{
				RequestID: uuid.NewString(),
				TraceID:   sc.TraceID().String(),
				SpanID:    sc.SpanID().String(),
				Version:   version,
				RequestAt: time.Now(),
			}

			ctx := response.WithMeta(r.Context(), meta)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Logger logs one line per request (method, path, status, duration,
// request ID if Metadata ran first), at error level for 5xx responses. Put it after Metadata in the chain so RequestID is available.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			r = r.WithContext(applog.WithLogger(r.Context(), logger))

			next.ServeHTTP(sw, r)

			meta, _ := response.MetaFromContext(r.Context())
			duration := time.Since(start)
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			}

			// Message carries the summary (what log viewers show per row);
			// attrs stay for filtering.
			logger.Log(r.Context(), level,
				fmt.Sprintf("%s %s %d %s", r.Method, r.URL.Path, sw.status, duration.Round(time.Microsecond)),
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"durationMs", duration.Milliseconds(),
				"requestId", meta.RequestID,
			)
		})
	}
}

// Recoverer catches a panic anywhere downstream, logs it (Error level,
// with stack trace) and writes a 500 in the same envelope shape as
// response.NewError, instead of the connection just dying. Put it
// innermost (wrapping the router) so Logger still sees the resulting
// status/duration.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}

			ctx := r.Context()
			applog.FromContext(ctx).ErrorContext(ctx, "panic recovered",
				"panic", rec, "stack", string(debug.Stack()))

			meta, _ := response.MetaFromContext(ctx)
			meta.ResponseAt = time.Now()
			body, err := json.Marshal(&response.ErrorOutput{
				Meta: meta,
				ErrorModel: &huma.ErrorModel{
					Title:  http.StatusText(http.StatusInternalServerError),
					Status: http.StatusInternalServerError,
					Detail: "Internal server error.",
				},
			})
			if err != nil {
				// Marshaling our own fixed-shape struct should never fail;
				// fall back to a plain-text 500 if it somehow does.
				http.Error(w, "Internal server error.", http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(body)
		}()

		next.ServeHTTP(w, r)
	})
}

// statusWriter captures the status code written so Logger can log it —
// http.ResponseWriter doesn't expose it after the fact.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// CORS allows the given origins (["*"] allows any origin) and answers
// preflight OPTIONS requests directly.
//
// ponytail: hand-rolled, no credentialed/wildcard-subdomain support — swap
// for github.com/rs/cors if that's needed.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	allowAny := len(allowedOrigins) == 1 && allowedOrigins[0] == "*"

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAny || slices.ContainsFunc(allowedOrigins, func(o string) bool { return strings.EqualFold(o, origin) })) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
