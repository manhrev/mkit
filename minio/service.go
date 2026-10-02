// Package minio wraps a minio-go client with the app's default bucket, so
// callers can pass just an object name instead of a bucket+object pair on
// every call.
package minio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/manhrev/mkit/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrObjectNotFound is returned in place of minio's raw "NoSuchKey" error so
// callers can check for it with errors.Is instead of string-matching.
var ErrObjectNotFound = errors.New("minio: object not found")

type Service struct {
	client        *minio.Client
	logger        *slog.Logger
	defaultBucket string
}

// New connects to Minio and fails fast if the default bucket isn't
// reachable.
func New(ctx context.Context, appCfg *config.App, logger *slog.Logger) (*Service, error) {
	cfg := appCfg.Minio
	if cfg == nil {
		return nil, fmt.Errorf("minio: config is nil")
	}

	client, err := minio.New(
		fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		&minio.Options{
			Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
			Secure: cfg.SSLEnabled,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("minio: cannot init client: %w", err)
	}

	exists, err := client.BucketExists(ctx, cfg.BucketName)
	if err != nil {
		return nil, fmt.Errorf("minio: bucket check failed: %w", err)
	}
	if !exists {
		logger.Info("minio: default bucket does not exist", "bucket", cfg.BucketName)
	} else {
		logger.Info("minio: connected", "bucket", cfg.BucketName)
	}

	return &Service{
		client:        client,
		logger:        logger,
		defaultBucket: cfg.BucketName,
	}, nil
}

// PutObjectWithBucket uploads data as objectName in bucketName.
func (s *Service) PutObjectWithBucket(
	ctx context.Context, bucketName, objectName string, data io.Reader, size int64, contentType string,
) (minio.UploadInfo, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return s.client.PutObject(ctx, bucketName, objectName, data, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
}

// PutObject uploads data as objectName in the default bucket.
func (s *Service) PutObject(
	ctx context.Context, objectName string, data io.Reader, size int64, contentType string,
) (minio.UploadInfo, error) {
	return s.PutObjectWithBucket(ctx, s.defaultBucket, objectName, data, size, contentType)
}

// CheckIfObjectExistsWithBucket reports whether objectName exists in bucketName.
func (s *Service) CheckIfObjectExistsWithBucket(
	ctx context.Context, bucketName, objectName string,
) (bool, error) {
	_, err := s.client.StatObject(ctx, bucketName, objectName, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("minio: stat object %s: %w", objectName, err)
	}

	return true, nil
}

// CheckIfObjectExists reports whether objectName exists in the default bucket.
func (s *Service) CheckIfObjectExists(ctx context.Context, objectName string) (bool, error) {
	return s.CheckIfObjectExistsWithBucket(ctx, s.defaultBucket, objectName)
}

// ListObjectNamesWithBucket lists object keys under basePath in bucketName (non-recursive).
func (s *Service) ListObjectNamesWithBucket(ctx context.Context, bucketName, basePath string) ([]string, error) {
	if basePath != "" && !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	objectCh := s.client.ListObjects(ctx, bucketName, minio.ListObjectsOptions{
		Prefix:    basePath,
		Recursive: false,
	})

	var files []string
	for obj := range objectCh {
		if obj.Err != nil {
			s.logger.Error("minio: list objects", "bucket", bucketName, "prefix", basePath, "error", obj.Err)
			return nil, obj.Err
		}
		files = append(files, obj.Key)
	}

	return files, nil
}

// ListObjectNames lists object keys under basePath in the default bucket (non-recursive).
func (s *Service) ListObjectNames(ctx context.Context, basePath string) ([]string, error) {
	return s.ListObjectNamesWithBucket(ctx, s.defaultBucket, basePath)
}

// GetObjectWithBucket returns a lazy reader for objectName in bucketName. Per
// minio-go's own design, a missing key does not error here — it surfaces on
// the first Read/Stat of the returned object, at which point callers should
// check errors.Is(err, ErrObjectNotFound).
func (s *Service) GetObjectWithBucket(ctx context.Context, bucketName, objectName string) (*minio.Object, error) {
	obj, err := s.client.GetObject(ctx, bucketName, objectName, minio.GetObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrObjectNotFound
		}
		return nil, fmt.Errorf("minio: get object %s: %w", objectName, err)
	}

	return obj, nil
}

// GetObject returns a lazy reader for objectName in the default bucket. See
// GetObjectWithBucket for how missing-key errors surface.
func (s *Service) GetObject(ctx context.Context, objectName string) (*minio.Object, error) {
	return s.GetObjectWithBucket(ctx, s.defaultBucket, objectName)
}
