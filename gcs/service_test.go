package gcs

import "testing"

func TestPublicURLWithBucket(t *testing.T) {
	got := PublicURLWithBucket("b", "uploads/a b/ảnh#1.webp")
	want := "https://storage.googleapis.com/b/uploads/a%20b/%E1%BA%A3nh%231.webp"
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
