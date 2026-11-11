// Package localbucket is a dev-only stand-in for the bucket: objects live
// in a folder on disk and the API itself serves them under Prefix, including
// the presigned PUT, so uploads work locally without a real bucket.
package localbucket

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/manhrev/mkit/objectstorage"
)

// Prefix is the URL path the objects are served under.
const Prefix = "/local-bucket/"

var _ objectstorage.ObjectStorage = (*Store)(nil)

type Store struct {
	dir       string
	publicURL string // ends with "/"
	key       []byte // signs upload URLs; new per process, so old URLs die on restart
}

// New stores objects under dir (created if missing). publicURL is where the
// API serves Prefix, e.g. http://localhost:8080/local-bucket/.
func New(dir, publicURL string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("local bucket dir: %w", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &Store{dir: dir, publicURL: strings.TrimRight(publicURL, "/") + "/", key: key}, nil
}

// file maps an object name to its path, refusing names that leave dir.
func (s *Store) file(name string) (string, error) {
	if name == "" || !filepath.IsLocal(filepath.FromSlash(name)) {
		return "", fmt.Errorf("bad object name %q", name)
	}
	return filepath.Join(s.dir, filepath.FromSlash(name)), nil
}

func (s *Store) PutObject(_ context.Context, objectName string, data io.Reader, contentType, _ string) (objectstorage.ObjectInfo, error) {
	p, err := s.file(objectName)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	n, err := writeFile(p, data)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	return objectstorage.ObjectInfo{Key: objectName, Size: n, ContentType: contentType}, nil
}

func (s *Store) PublicURL(objectName string) string {
	return s.publicURL + objectName
}

// SignedPutURL returns a URL to this API's PUT handler, valid for expires,
// for contentType and at most maxBytes.
func (s *Store) SignedPutURL(_ context.Context, objectName, contentType string, _, maxBytes int64, expires time.Duration) (string, map[string]string, error) {
	if _, err := s.file(objectName); err != nil {
		return "", nil, err
	}
	limit := strconv.FormatInt(maxBytes, 10)
	exp := strconv.FormatInt(time.Now().Add(expires).Unix(), 10)
	q := url.Values{"ct": {contentType}, "max": {limit}, "exp": {exp}, "sig": {s.sign(objectName, contentType, limit, exp)}}
	return s.publicURL + objectName + "?" + q.Encode(), map[string]string{"Content-Type": contentType}, nil
}

func (s *Store) sign(parts ...string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Store) StatObject(_ context.Context, objectName string) (objectstorage.ObjectInfo, error) {
	p, err := s.file(objectName)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	fi, err := os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectNotFound
	}
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	return objectstorage.ObjectInfo{Key: objectName, Size: fi.Size(), ContentType: mime.TypeByExtension(path.Ext(objectName))}, nil
}

func (s *Store) GetObjectRange(_ context.Context, objectName string, offset, length int64) (io.ReadCloser, error) {
	p, err := s.file(objectName)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, objectstorage.ErrObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{io.NewSectionReader(f, offset, length), f}, nil
}

func (s *Store) CopyObject(ctx context.Context, src, dst, contentType, _ string) (objectstorage.ObjectInfo, error) {
	from, err := s.file(src)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	to, err := s.file(dst)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	if _, err := os.Stat(to); err == nil {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectExists
	}
	f, err := os.Open(from)
	if errors.Is(err, os.ErrNotExist) {
		return objectstorage.ObjectInfo{}, objectstorage.ErrObjectNotFound
	}
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	defer f.Close()
	n, err := writeFile(to, f)
	if err != nil {
		return objectstorage.ObjectInfo{}, err
	}
	return objectstorage.ObjectInfo{Key: dst, Size: n, ContentType: contentType}, nil
}

// writeFile writes through a temp file so a reader never sees half an object.
func writeFile(p string, data io.Reader) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	n, err := io.Copy(tmp, data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return 0, err
	}
	return n, os.Rename(tmp.Name(), p)
}

// Mount serves Prefix itself and passes every other request to next. It sits
// outside the API's CORS middleware, which doesn't allow the upload header.
func (s *Store) Mount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, ok := strings.CutPrefix(r.URL.Path, Prefix)
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		p, err := s.file(name)
		switch {
		case r.Method == http.MethodOptions:
			w.WriteHeader(http.StatusNoContent)
		case err != nil:
			http.NotFound(w, r)
		case r.Method == http.MethodGet || r.Method == http.MethodHead:
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, p)
		case r.Method == http.MethodPut:
			s.put(w, r, name, p)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

// put checks the signed URL like a bucket checks its signature: the name, type,
// size limit and expiry it was signed for.
func (s *Store) put(w http.ResponseWriter, r *http.Request, name, p string) {
	q := r.URL.Query()
	ct, maxBytes, exp := q.Get("ct"), q.Get("max"), q.Get("exp")
	expAt, err := strconv.ParseInt(exp, 10, 64)
	switch {
	case err != nil || !hmac.Equal([]byte(q.Get("sig")), []byte(s.sign(name, ct, maxBytes, exp))):
		http.Error(w, "bad signature", http.StatusForbidden)
		return
	case time.Now().Unix() > expAt:
		http.Error(w, "expired", http.StatusForbidden)
		return
	case r.Header.Get("Content-Type") != ct:
		http.Error(w, "content type differs from the signed one", http.StatusForbidden)
		return
	}
	body := io.Reader(r.Body)
	limit, _ := strconv.ParseInt(maxBytes, 10, 64)
	if limit > 0 {
		body = io.LimitReader(r.Body, limit+1)
	}
	n, err := writeFile(p, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if limit > 0 && n > limit {
		_ = os.Remove(p)
		http.Error(w, "file too large", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}
