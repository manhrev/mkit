package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoggerFlushes(t *testing.T) {
	var flushErr error
	h := Logger(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flushErr = http.NewResponseController(w).Flush()
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if flushErr != nil || !rec.Flushed {
		t.Fatalf("Flush through Logger: err=%v flushed=%v", flushErr, rec.Flushed)
	}
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://ok.example"})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	// refused origin: no allow header, but still Vary
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Vary") != "Origin" || rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("refused origin headers = %v", rec.Header())
	}

	// preflight from an allowed origin
	req = httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://ok.example")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent ||
		rec.Header().Get("Access-Control-Allow-Origin") != "https://ok.example" ||
		rec.Header().Get("Access-Control-Max-Age") != "600" {
		t.Fatalf("preflight: code=%d headers=%v", rec.Code, rec.Header())
	}
}
