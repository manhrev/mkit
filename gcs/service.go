// Package gcs wraps a Google Cloud Storage client with the app's default
// bucket, so callers can pass just an object name instead of a
// bucket+object pair on every call. Mirrors package minio.
package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/manhrev/mkit/config"
)

// ErrObjectNotFound is returned in place of storage.ErrObjectNotExist so
// callers check one error whichever object store they use.
var ErrObjectNotFound = errors.New("gcs: object not found")

// ErrObjectExists is returned by CopyObject with ifNotExists when the
// destination is already there.
var ErrObjectExists = errors.New("gcs: object already exists")

type Service struct {
	client        *storage.Client
	logger        *slog.Logger
	defaultBucket string
	signerEmail   string
}

// UploadInfo describes a stored object.
type UploadInfo struct {
	Bucket      string
	Name        string
	Size        int64
	ContentType string
}

// New creates the client. Unlike minio.New it doesn't check the bucket:
// that needs storage.buckets.get, which an object-only service account
// lacks, and would add a network round trip to every cold start.
func New(ctx context.Context, appCfg *config.App, logger *slog.Logger) (*Service, error) {
	cfg := appCfg.GCS
	if cfg == nil {
		return nil, fmt.Errorf("gcs: config is nil")
	}

	var opts []option.ClientOption
	if cfg.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.CredentialsFile))
	}
	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("gcs: cannot init client: %w", err)
	}

	logger.Info("gcs: client ready", "bucket", cfg.BucketName)
	return &Service{client: client, logger: logger, defaultBucket: cfg.BucketName, signerEmail: cfg.SignerEmail}, nil
}

// Close releases the client's connections.
func (s *Service) Close() error {
	return s.client.Close()
}

// PutObjectWithBucket uploads data as objectName in bucketName. An empty
// contentType stores application/octet-stream; an empty cacheControl
// leaves GCS's default.
func (s *Service) PutObjectWithBucket(
	ctx context.Context, bucketName, objectName string, data io.Reader, contentType, cacheControl string,
) (UploadInfo, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w := s.client.Bucket(bucketName).Object(objectName).NewWriter(ctx)
	w.ContentType = contentType
	w.CacheControl = cacheControl
	size, err := io.Copy(w, data)
	if err != nil {
		_ = w.Close()
		return UploadInfo{}, fmt.Errorf("gcs: write object %s: %w", objectName, err)
	}
	// Close commits the upload; its error is the one that matters.
	if err := w.Close(); err != nil {
		return UploadInfo{}, fmt.Errorf("gcs: put object %s: %w", objectName, err)
	}

	return UploadInfo{Bucket: bucketName, Name: objectName, Size: size, ContentType: contentType}, nil
}

// PutObject uploads data as objectName in the default bucket.
func (s *Service) PutObject(
	ctx context.Context, objectName string, data io.Reader, contentType, cacheControl string,
) (UploadInfo, error) {
	return s.PutObjectWithBucket(ctx, s.defaultBucket, objectName, data, contentType, cacheControl)
}

// CheckIfObjectExistsWithBucket reports whether objectName exists in bucketName.
func (s *Service) CheckIfObjectExistsWithBucket(ctx context.Context, bucketName, objectName string) (bool, error) {
	_, err := s.client.Bucket(bucketName).Object(objectName).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("gcs: stat object %s: %w", objectName, err)
	}

	return true, nil
}

// CheckIfObjectExists reports whether objectName exists in the default bucket.
func (s *Service) CheckIfObjectExists(ctx context.Context, objectName string) (bool, error) {
	return s.CheckIfObjectExistsWithBucket(ctx, s.defaultBucket, objectName)
}

// ListObjectNamesWithBucket lists object keys under basePath in bucketName
// (non-recursive: "sub/" prefixes are returned as names, like minio).
func (s *Service) ListObjectNamesWithBucket(ctx context.Context, bucketName, basePath string) ([]string, error) {
	if basePath != "" && !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	it := s.client.Bucket(bucketName).Objects(ctx, &storage.Query{Prefix: basePath, Delimiter: "/"})
	var files []string
	for {
		obj, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return files, nil
		}
		if err != nil {
			s.logger.Error("gcs: list objects", "bucket", bucketName, "prefix", basePath, "error", err)
			return nil, err
		}
		if obj.Prefix != "" {
			files = append(files, obj.Prefix)
		} else {
			files = append(files, obj.Name)
		}
	}
}

// ListObjectNames lists object keys under basePath in the default bucket (non-recursive).
func (s *Service) ListObjectNames(ctx context.Context, basePath string) ([]string, error) {
	return s.ListObjectNamesWithBucket(ctx, s.defaultBucket, basePath)
}

// GetObjectWithBucket opens objectName in bucketName for reading; the
// caller closes the reader. Unlike minio, a missing object fails here with
// ErrObjectNotFound, not on the first Read.
func (s *Service) GetObjectWithBucket(ctx context.Context, bucketName, objectName string) (*storage.Reader, error) {
	r, err := s.client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("gcs: get object %s: %w", objectName, err)
	}

	return r, nil
}

// GetObject opens objectName in the default bucket. See GetObjectWithBucket.
func (s *Service) GetObject(ctx context.Context, objectName string) (*storage.Reader, error) {
	return s.GetObjectWithBucket(ctx, s.defaultBucket, objectName)
}

// SignedPutURLWithBucket is a V4 signed URL that lets anyone PUT objectName
// into bucketName until expires passes. The client must send Content-Type:
// contentType (when set) and every header in headers ("Name:value", e.g.
// "x-goog-content-length-range:0,1048576") exactly, or GCS rejects the
// upload with SignatureDoesNotMatch.
func (s *Service) SignedPutURLWithBucket(bucketName, objectName, contentType string, expires time.Duration, headers []string) (string, error) {
	u, err := s.client.Bucket(bucketName).SignedURL(objectName, &storage.SignedURLOptions{
		GoogleAccessID: s.signerEmail,
		Scheme:         storage.SigningSchemeV4,
		Method:         http.MethodPut,
		Expires:        time.Now().Add(expires),
		ContentType:    contentType,
		Headers:        headers,
	})
	if err != nil {
		return "", fmt.Errorf("gcs: sign put %s: %w", objectName, err)
	}
	return u, nil
}

// SignedPutURL signs a PUT for objectName in the default bucket. See SignedPutURLWithBucket.
func (s *Service) SignedPutURL(objectName, contentType string, expires time.Duration, headers []string) (string, error) {
	return s.SignedPutURLWithBucket(s.defaultBucket, objectName, contentType, expires, headers)
}

// StatObjectWithBucket returns objectName's attributes, or ErrObjectNotFound.
func (s *Service) StatObjectWithBucket(ctx context.Context, bucketName, objectName string) (*storage.ObjectAttrs, error) {
	attrs, err := s.client.Bucket(bucketName).Object(objectName).Attrs(ctx)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("gcs: stat object %s: %w", objectName, err)
	}
	return attrs, nil
}

// StatObject returns objectName's attributes in the default bucket.
func (s *Service) StatObject(ctx context.Context, objectName string) (*storage.ObjectAttrs, error) {
	return s.StatObjectWithBucket(ctx, s.defaultBucket, objectName)
}

// GetObjectRangeWithBucket opens length bytes of objectName from offset
// (length -1 = to the end); the caller closes the reader. A plain reader,
// so callers can fake it; StatObject has the attributes.
func (s *Service) GetObjectRangeWithBucket(ctx context.Context, bucketName, objectName string, offset, length int64) (io.ReadCloser, error) {
	r, err := s.client.Bucket(bucketName).Object(objectName).NewRangeReader(ctx, offset, length)
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil, ErrObjectNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("gcs: get object %s: %w", objectName, err)
	}
	return r, nil
}

// GetObjectRange opens part of objectName in the default bucket. See GetObjectRangeWithBucket.
func (s *Service) GetObjectRange(ctx context.Context, objectName string, offset, length int64) (io.ReadCloser, error) {
	return s.GetObjectRangeWithBucket(ctx, s.defaultBucket, objectName, offset, length)
}

// CopyObjectWithBucket copies src to dst inside bucketName, setting dst's
// content type and cache control (empty = keep src's). With ifNotExists the
// copy fails with ErrObjectExists when dst is already there, so it needs
// only object create permission, never overwrite.
func (s *Service) CopyObjectWithBucket(
	ctx context.Context, bucketName, src, dst, contentType, cacheControl string, ifNotExists bool,
) (UploadInfo, error) {
	bucket := s.client.Bucket(bucketName)
	dstObj := bucket.Object(dst)
	if ifNotExists {
		dstObj = dstObj.If(storage.Conditions{DoesNotExist: true})
	}
	c := dstObj.CopierFrom(bucket.Object(src))
	c.ContentType = contentType
	c.CacheControl = cacheControl
	attrs, err := c.Run(ctx)
	if e, ok := errors.AsType[*googleapi.Error](err); ok && e.Code == http.StatusPreconditionFailed {
		return UploadInfo{}, ErrObjectExists
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return UploadInfo{}, ErrObjectNotFound
	}
	if err != nil {
		return UploadInfo{}, fmt.Errorf("gcs: copy %s to %s: %w", src, dst, err)
	}
	return UploadInfo{Bucket: bucketName, Name: attrs.Name, Size: attrs.Size, ContentType: attrs.ContentType}, nil
}

// CopyObject copies src to dst in the default bucket. See CopyObjectWithBucket.
func (s *Service) CopyObject(ctx context.Context, src, dst, contentType, cacheControl string, ifNotExists bool) (UploadInfo, error) {
	return s.CopyObjectWithBucket(ctx, s.defaultBucket, src, dst, contentType, cacheControl, ifNotExists)
}

// PublicURLWithBucket is objectName's storage.googleapis.com URL. It only
// works when the bucket grants public read.
func PublicURLWithBucket(bucketName, objectName string) string {
	parts := strings.Split(objectName, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return "https://storage.googleapis.com/" + bucketName + "/" + strings.Join(parts, "/")
}

// PublicURL is objectName's public URL in the default bucket.
func (s *Service) PublicURL(objectName string) string {
	return PublicURLWithBucket(s.defaultBucket, objectName)
}
