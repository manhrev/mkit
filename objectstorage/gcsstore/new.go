// Package gcsstore is objectstorage.ObjectStorage on a Google Cloud Storage
// bucket that grants public read.
package gcsstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/manhrev/mkit/objectstorage"
)

// lengthRangeHeader makes GCS refuse a signed PUT larger than its max.
const lengthRangeHeader = "x-goog-content-length-range"

var _ objectstorage.ObjectStorage = (*Store)(nil)

type Store struct {
	client      *storage.Client
	bucket      string
	signerEmail string
}

// New creates the client for bucket; Close it on shutdown. Credentials come
// from credentialsFile (a service account JSON) when set, else Application
// Default Credentials. signerEmail is the service account signed URLs are
// signed as, through the IAM Credentials signBlob API; empty = detect it
// from the credentials (a key file, or Cloud Run/GCE; not user ADC).
//
// It doesn't check the bucket: that needs storage.buckets.get, which an
// object-only service account lacks, and would add a network round trip to
// every cold start.
func New(ctx context.Context, bucket, credentialsFile, signerEmail string) (*Store, error) {
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcsstore: cannot init client: %w", err)
	}

	return &Store{client: client, bucket: bucket, signerEmail: signerEmail}, nil
}

// Close releases the client's connections.
func (s *Store) Close() error { return s.client.Close() }

func (s *Store) object(key string) *storage.ObjectHandle {
	return s.client.Bucket(s.bucket).Object(key)
}

// PutObject stores application/octet-stream for an empty contentType; an
// empty cacheControl leaves GCS's default.
func (s *Store) PutObject(ctx context.Context, key string, data io.Reader, contentType, cacheControl string) (objectstorage.ObjectInfo, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w := s.object(key).NewWriter(ctx)
	w.ContentType = contentType
	w.CacheControl = cacheControl
	size, err := io.Copy(w, data)
	if err != nil {
		_ = w.Close()
		return objectstorage.ObjectInfo{}, fmt.Errorf("gcsstore: write object %s: %w", key, err)
	}
	// Close commits the upload; its error is the one that matters.
	if err := w.Close(); err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("gcsstore: put object %s: %w", key, err)
	}

	return objectstorage.ObjectInfo{Key: key, Size: size, ContentType: contentType}, nil
}

// PublicURL is key's storage.googleapis.com URL, each path segment escaped.
func (s *Store) PublicURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}

	return "https://storage.googleapis.com/" + s.bucket + "/" + strings.Join(parts, "/")
}

// SignedPutURL is a V4 signed URL. It signs the max size
// (x-goog-content-length-range), not the exact one; the client must send
// the returned headers exactly, or GCS answers SignatureDoesNotMatch.
func (s *Store) SignedPutURL(_ context.Context, key, contentType string, _, maxBytes int64, expires time.Duration) (string, map[string]string, error) {
	lengthRange := "0," + strconv.FormatInt(maxBytes, 10)
	u, err := s.client.Bucket(s.bucket).SignedURL(key, &storage.SignedURLOptions{
		GoogleAccessID: s.signerEmail,
		Scheme:         storage.SigningSchemeV4,
		Method:         http.MethodPut,
		Expires:        time.Now().Add(expires),
		ContentType:    contentType,
		Headers:        []string{lengthRangeHeader + ":" + lengthRange},
	})
	if err != nil {
		return "", nil, fmt.Errorf("gcsstore: sign put %s: %w", key, err)
	}

	return u, map[string]string{"Content-Type": contentType, lengthRangeHeader: lengthRange}, nil
}

func (s *Store) StatObject(ctx context.Context, key string) (objectstorage.ObjectInfo, error) {
	attrs, err := s.object(key).Attrs(ctx)
	if err != nil {
		return objectstorage.ObjectInfo{}, mapErr("stat object "+key, err)
	}

	return objectstorage.ObjectInfo{Key: attrs.Name, Size: attrs.Size, ContentType: attrs.ContentType}, nil
}

func (s *Store) GetObjectRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	r, err := s.object(key).NewRangeReader(ctx, offset, length)
	if err != nil {
		return nil, mapErr("get object "+key, err)
	}

	return r, nil
}

// CopyObject copies only if dst does not exist (DoesNotExist precondition),
// so it needs object create permission, never overwrite.
func (s *Store) CopyObject(ctx context.Context, src, dst, contentType, cacheControl string) (objectstorage.ObjectInfo, error) {
	c := s.object(dst).If(storage.Conditions{DoesNotExist: true}).CopierFrom(s.object(src))
	c.ContentType = contentType
	c.CacheControl = cacheControl
	attrs, err := c.Run(ctx)
	if err != nil {
		return objectstorage.ObjectInfo{}, mapErr("copy "+src+" to "+dst, err)
	}

	return objectstorage.ObjectInfo{Key: attrs.Name, Size: attrs.Size, ContentType: attrs.ContentType}, nil
}

// mapErr turns GCS's not-found and failed-precondition errors into
// objectstorage's sentinels, and wraps the rest with what was being done.
func mapErr(doing string, err error) error {
	if errors.Is(err, storage.ErrObjectNotExist) {
		return objectstorage.ErrObjectNotFound
	}
	if e, ok := errors.AsType[*googleapi.Error](err); ok && e.Code == http.StatusPreconditionFailed {
		return objectstorage.ErrObjectExists
	}

	return fmt.Errorf("gcsstore: %s: %w", doing, err)
}
