// Package objectstorage is the one bucket interface every service uses.
// Implementations (subpackages): s3store (any S3 API, e.g.
// Cloudflare R2), gcsstore (Google Cloud Storage) and localbucket (dev only, on disk). Keys are object names
// inside the bucket, e.g. "uploads/avatars/<user>/<stem>_<id>.webp".
package objectstorage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	// ErrObjectNotFound: the key has no object.
	ErrObjectNotFound = errors.New("object not found")
	// ErrObjectExists: CopyObject's destination already has an object.
	ErrObjectExists = errors.New("object already exists")
)

// ObjectStorage is a public bucket of immutable objects.
type ObjectStorage interface {
	// PutObject stores data at key with the given Content-Type and Cache-Control.
	PutObject(ctx context.Context, key string, data io.Reader, contentType, cacheControl string) (ObjectInfo, error)
	// PublicURL is where anyone can GET key.
	PublicURL(key string) string
	// SignedPutURL returns a URL the client PUTs one file to until expires,
	// and the headers it must send with it. The file must be contentType and
	// size bytes (size <= maxBytes); how strictly each is enforced depends on
	// the backend (GCS, localbucket: at most maxBytes; S3: exactly size). Callers still
	// check the stored object.
	SignedPutURL(ctx context.Context, key, contentType string, size, maxBytes int64, expires time.Duration) (string, map[string]string, error)
	// StatObject returns key's metadata, ErrObjectNotFound when missing.
	StatObject(ctx context.Context, key string) (ObjectInfo, error)
	// GetObjectRange reads length bytes of key from offset, ErrObjectNotFound when missing.
	GetObjectRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error)
	// CopyObject copies src to dst with new Content-Type and Cache-Control.
	// It never overwrites: ErrObjectExists when dst exists, ErrObjectNotFound
	// when src is missing.
	CopyObject(ctx context.Context, src, dst, contentType, cacheControl string) (ObjectInfo, error)
}

// ObjectInfo is one stored object's metadata.
type ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}
