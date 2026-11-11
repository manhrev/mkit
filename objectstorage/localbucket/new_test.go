package localbucket

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/manhrev/mkit/objectstorage"
)

func TestPresignedPutThenCommit(t *testing.T) {
	ctx := context.Background()
	s, err := New(t.TempDir(), "http://api.test/local-bucket")
	if err != nil {
		t.Fatal(err)
	}
	h := s.Mount(http.NotFoundHandler())
	put := func(u, ct, body string) int {
		req := httptest.NewRequest(http.MethodPut, u, strings.NewReader(body))
		req.Header.Set("Content-Type", ct)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	u, _, err := s.SignedPutURL(ctx, "tmp/uploads/a/b_1.jpg", "image/jpeg", 3, 5, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if code := put(u, "image/png", "abc"); code != http.StatusForbidden {
		t.Fatalf("other content type: %d", code)
	}
	if code := put(strings.Replace(u, "b_1", "b_2", 1), "image/jpeg", "abc"); code != http.StatusForbidden {
		t.Fatalf("other name: %d", code)
	}
	if code := put(u, "image/jpeg", "too long"); code != http.StatusBadRequest {
		t.Fatalf("over max: %d", code)
	}
	if code := put(u, "image/jpeg", "abc"); code != http.StatusOK {
		t.Fatalf("put: %d", code)
	}

	attrs, err := s.StatObject(ctx, "tmp/uploads/a/b_1.jpg")
	if err != nil || attrs.Size != 3 || attrs.ContentType != "image/jpeg" {
		t.Fatalf("stat: %+v %v", attrs, err)
	}
	if _, err := s.CopyObject(ctx, "tmp/uploads/a/b_1.jpg", "uploads/a/b_1.jpg", "image/jpeg", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CopyObject(ctx, "tmp/uploads/a/b_1.jpg", "uploads/a/b_1.jpg", "image/jpeg", ""); !errors.Is(err, objectstorage.ErrObjectExists) {
		t.Fatalf("second copy: %v", err)
	}
	rc, err := s.GetObjectRange(ctx, "uploads/a/b_1.jpg", 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "bc" {
		t.Fatalf("range: %q", b)
	}
	if _, err := s.StatObject(ctx, "missing.jpg"); !errors.Is(err, objectstorage.ErrObjectNotFound) {
		t.Fatalf("missing: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/local-bucket/uploads/a/b_1.jpg", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "abc" {
		t.Fatalf("get: %d %q", rec.Code, rec.Body.String())
	}
}

func TestRefusesNamesOutsideDir(t *testing.T) {
	s, err := New(t.TempDir(), "http://api.test/local-bucket/")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SignedPutURL(context.Background(), "../escape.jpg", "image/jpeg", 1, 1, time.Minute); err == nil {
		t.Fatal("signed a name outside the dir")
	}
	if _, err := s.PutObject(context.Background(), "/abs.jpg", strings.NewReader("x"), "image/jpeg", ""); err == nil {
		t.Fatal("wrote an absolute name")
	}
}
