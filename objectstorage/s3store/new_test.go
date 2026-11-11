package s3store

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/manhrev/mkit/objectstorage"
)

// fakeS3 answers HEAD with 404 for missing.jpg and new.jpg (until copied)
// and 200 for everything else; it records the copy request's headers.
func fakeS3(t *testing.T) (*Store, *http.Header) {
	var mu sync.Mutex
	copied := http.Header{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPut && r.Header.Get("x-amz-copy-source") != "" {
			copied = r.Header.Clone()
			_, _ = w.Write([]byte(`<CopyObjectResult></CopyObjectResult>`))
			return
		}
		if r.Method == http.MethodHead && (strings.HasSuffix(r.URL.Path, "/missing.jpg") || strings.HasSuffix(r.URL.Path, "/new.jpg") && len(copied) == 0) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", "3")
		w.Header().Set("Content-Type", "image/jpeg")
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "auto", "b", "key", "secret", "https://img.test"), &copied
}

func TestStore(t *testing.T) {
	ctx := context.Background()
	s, copied := fakeS3(t)

	if got := s.PublicURL("uploads/a b/ảnh.jpg"); got != "https://img.test/uploads/a%20b/%E1%BA%A3nh.jpg" {
		t.Errorf("public url: %s", got)
	}

	u, headers, err := s.SignedPutURL(ctx, "tmp/x.jpg", "image/jpeg", 1234, 5000, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.Parse(u)
	// Anything else signed (e.g. an SDK checksum) is a header the browser won't send: R2 then refuses the PUT.
	if sh := q.Query().Get("X-Amz-SignedHeaders"); sh != "content-length;content-type;host" {
		t.Errorf("signed headers %q", sh)
	}
	for k := range q.Query() {
		if k := strings.ToLower(k); strings.HasPrefix(k, "x-amz-checksum") || strings.HasPrefix(k, "x-amz-sdk-checksum") {
			t.Errorf("checksum param %s in presigned URL", k)
		}
	}
	if len(headers) != 1 || headers["Content-Type"] != "image/jpeg" {
		t.Errorf("client headers: %v", headers)
	}

	if _, err := s.StatObject(ctx, "missing.jpg"); !errors.Is(err, objectstorage.ErrObjectNotFound) {
		t.Errorf("missing: %v", err)
	}
	if info, err := s.StatObject(ctx, "a.jpg"); err != nil || info.Size != 3 || info.ContentType != "image/jpeg" {
		t.Errorf("stat: %+v %v", info, err)
	}
	if _, err := s.CopyObject(ctx, "tmp/a.jpg", "a.jpg", "image/jpeg", ""); !errors.Is(err, objectstorage.ErrObjectExists) {
		t.Errorf("copy onto existing: %v", err)
	}
	if info, err := s.CopyObject(ctx, "tmp/new.jpg", "new.jpg", "image/webp", "public, max-age=1"); err != nil || info.Size != 3 {
		t.Fatalf("copy: %+v %v", info, err)
	}
	h := *copied
	if h.Get("x-amz-copy-source") != "b/tmp/new.jpg" || h.Get("x-amz-metadata-directive") != "REPLACE" ||
		h.Get("Content-Type") != "image/webp" || h.Get("Cache-Control") != "public, max-age=1" {
		t.Errorf("copy request headers: %v", h)
	}
}
