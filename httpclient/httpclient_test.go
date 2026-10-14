package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type pong struct {
	Msg string `json:"msg"`
}

func TestSend(t *testing.T) {
	otel.SetTracerProvider(sdktrace.NewTracerProvider())
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var gotTraceparent, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		switch r.URL.Path {
		case "/ok":
			w.Write([]byte(`{"msg":"hi"}`))
		case "/empty":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("bad key"))
		}
	}))
	defer srv.Close()

	ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
	defer span.End()
	c := NewClient(nil)

	res, err := c.Req().SetBody(map[string]string{"a": "b"}).Post[pong](ctx, srv.URL+"/ok")
	if err != nil || res.StatusCode != 200 || res.Body.Msg != "hi" {
		t.Fatalf("2xx decode: res=%+v err=%v", res, err)
	}
	if gotBody != `{"a":"b"}` {
		t.Fatalf("request body = %q", gotBody)
	}
	if gotTraceparent == "" {
		t.Fatal("traceparent not propagated")
	}

	empty, err := c.Req().Get[pong](ctx, srv.URL+"/empty")
	if err != nil || empty.StatusCode != 204 || empty.Body != (pong{}) {
		t.Fatalf("empty 2xx: res=%+v err=%v", empty, err)
	}

	bad, err := c.Req().Get[pong](ctx, srv.URL+"/bad")
	if err != nil || bad.StatusCode != 400 || bad.Body != (pong{}) || string(bad.Raw) != "bad key" {
		t.Fatalf("non-2xx: res=%+v err=%v", bad, err)
	}
}

// A retried POST must resend its body; exhausting retries on 503 returns
// the last response, not an error (or a nil response).
func TestRetryPost(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if b, _ := io.ReadAll(r.Body); string(b) != `"x"` {
			t.Errorf("attempt %d body = %q", calls, b)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	res, err := NewClient(nil).Req().SetBody("x").SetRetry(2).Post[struct{}](context.Background(), srv.URL)
	if err != nil || res.StatusCode != 503 || calls != 2 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, calls)
	}
}
