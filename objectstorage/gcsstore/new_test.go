package gcsstore

import (
	"errors"
	"net/http"
	"testing"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"

	"github.com/manhrev/mkit/objectstorage"
)

func TestPublicURL(t *testing.T) {
	got := (&Store{bucket: "b"}).PublicURL("uploads/a b/ảnh#1.webp")
	want := "https://storage.googleapis.com/b/uploads/a%20b/%E1%BA%A3nh%231.webp"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestMapErr(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		err  error
		want error
	}{
		{storage.ErrObjectNotExist, objectstorage.ErrObjectNotFound},
		{&googleapi.Error{Code: http.StatusPreconditionFailed}, objectstorage.ErrObjectExists},
		{boom, boom},
	}
	for _, c := range cases {
		if got := mapErr("x", c.err); !errors.Is(got, c.want) {
			t.Errorf("mapErr(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}
