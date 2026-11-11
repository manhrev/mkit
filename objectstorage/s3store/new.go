// Package s3store is objectstorage.ObjectStorage on any S3 API bucket, made
// for Cloudflare R2 (aws-sdk-go-v2).
package s3store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/manhrev/mkit/objectstorage"
)

var _ objectstorage.ObjectStorage = (*Store)(nil)

type Store struct {
	client    *s3.Client
	presign   *s3.PresignClient
	bucket    string
	publicURL string // ends with "/"
}

// New talks to bucket at endpoint (R2: https://<account_id>.r2.cloudflarestorage.com,
// region "auto"). publicURL is where objects are served to everyone (R2: the
// bucket's custom domain or r2.dev URL), not the API endpoint.
func New(endpoint, region, bucket, accessKeyID, secretAccessKey, publicURL string) *Store {
	client := s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		UsePathStyle: true,
		// The SDK's default CRC checksums aren't all supported by R2; send them only when an API requires one.
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Store{client: client, presign: s3.NewPresignClient(client), bucket: bucket, publicURL: strings.TrimRight(publicURL, "/") + "/"}
}

func (s *Store) PutObject(ctx context.Context, key string, data io.Reader, contentType, cacheControl string) (objectstorage.ObjectInfo, error) {
	// The SDK needs a seekable body to sign; uploads are small (<= 10 MB).
	b, err := io.ReadAll(data)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:       &s.bucket,
		Key:          &key,
		Body:         bytes.NewReader(b),
		ContentType:  &contentType,
		CacheControl: &cacheControl,
	})
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("s3: put %s: %w", key, mapErr(err))
	}
	return objectstorage.ObjectInfo{Key: key, Size: int64(len(b)), ContentType: contentType}, nil
}

// PublicURL escapes each path segment.
func (s *Store) PublicURL(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return s.publicURL + strings.Join(parts, "/")
}

// SignedPutURL signs Content-Type and the exact Content-Length (size): S3
// has no max-size header for a presigned PUT. Browsers set Content-Length
// themselves, so only Content-Type goes back to the client.
func (s *Store) SignedPutURL(ctx context.Context, key, contentType string, size, _ int64, expires time.Duration) (string, map[string]string, error) {
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        &s.bucket,
		Key:           &key,
		ContentType:   &contentType,
		ContentLength: &size,
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return "", nil, fmt.Errorf("s3: sign put %s: %w", key, err)
	}
	return req.URL, map[string]string{"Content-Type": contentType}, nil
}

func (s *Store) StatObject(ctx context.Context, key string) (objectstorage.ObjectInfo, error) {
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: &key})
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("s3: stat %s: %w", key, mapErr(err))
	}
	return objectstorage.ObjectInfo{Key: key, Size: aws.ToInt64(out.ContentLength), ContentType: aws.ToString(out.ContentType)}, nil
}

func (s *Store) GetObjectRange(ctx context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)),
	})
	if err != nil {
		return nil, fmt.Errorf("s3: get %s: %w", key, mapErr(err))
	}
	return out.Body, nil
}

// CopyObject replaces the metadata, so the tmp object's client-sent
// Content-Type doesn't carry over.
func (s *Store) CopyObject(ctx context.Context, src, dst, contentType, cacheControl string) (objectstorage.ObjectInfo, error) {
	// shortcut: R2 has no If-None-Match on CopyObject, so a stat before the copy;
	// two concurrent copies to dst can both run (same validated source bytes, harmless here).
	// Upgrade to IfNoneMatch "*" if R2 adds it.
	if _, err := s.StatObject(ctx, dst); err == nil {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectExists
	} else if !errors.Is(err, objectstorage.ErrObjectNotFound) {
		return objectstorage.ObjectInfo{}, err
	}
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:            &s.bucket,
		Key:               &dst,
		CopySource:        aws.String(url.PathEscape(s.bucket) + "/" + escapeKey(src)),
		ContentType:       &contentType,
		CacheControl:      &cacheControl,
		MetadataDirective: types.MetadataDirectiveReplace,
	})
	if err != nil {
		return objectstorage.ObjectInfo{}, fmt.Errorf("s3: copy %s to %s: %w", src, dst, mapErr(err))
	}
	// CopyObject's result carries no size.
	return s.StatObject(ctx, dst)
}

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// mapErr turns S3's not-found and precondition errors into objectstorage's sentinels.
func mapErr(err error) error {
	if apiErr, ok := errors.AsType[smithy.APIError](err); ok {
		switch apiErr.ErrorCode() {
		case "NotFound", "NoSuchKey":
			return objectstorage.ErrObjectNotFound
		case "PreconditionFailed":
			return objectstorage.ErrObjectExists
		}
	}
	// HeadObject has no body, so some services only give the status.
	if respErr, ok := errors.AsType[*smithyhttp.ResponseError](err); ok {
		switch respErr.HTTPStatusCode() {
		case http.StatusNotFound:
			return objectstorage.ErrObjectNotFound
		case http.StatusPreconditionFailed:
			return objectstorage.ErrObjectExists
		}
	}
	return err
}
