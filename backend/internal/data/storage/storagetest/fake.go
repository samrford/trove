// Package storagetest provides an in-memory storage.FileStore for tests, so
// handler/data tests can run without a real S3/MinIO.
package storagetest

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"trove/backend/internal/data/storage"
)

type object struct {
	data        []byte
	contentType string
	modTime     time.Time
}

// Fake is an in-memory storage.FileStore. Safe for concurrent use.
type Fake struct {
	mu      sync.Mutex
	objects map[string]object

	// UploadErr, when non-nil, is returned by UploadStream instead of storing
	// — lets tests exercise the upload-failure / size-cap (413) paths.
	UploadErr error
}

// Compile-time check that Fake satisfies the interface handlers depend on.
var _ storage.FileStore = (*Fake)(nil)

func NewFake() *Fake {
	return &Fake{objects: make(map[string]object)}
}

// Put seeds an object directly. Test helper — not part of FileStore.
func (f *Fake) Put(key string, data []byte, contentType string, modTime time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = object{data: append([]byte(nil), data...), contentType: contentType, modTime: modTime}
}

func (f *Fake) UploadStream(_ context.Context, key string, body io.Reader, contentType string) (int64, error) {
	if f.UploadErr != nil {
		return 0, f.UploadErr
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	f.mu.Lock()
	f.objects[key] = object{data: b, contentType: contentType, modTime: time.Now()}
	f.mu.Unlock()
	return int64(len(b)), nil
}

func (f *Fake) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
	return nil
}

// SignedURL returns a deterministic fake URL containing the key (tests assert
// the key is present in the returned URL).
func (f *Fake) SignedURL(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.fake/" + key, nil
}

func (f *Fake) ListWithPrefix(_ context.Context, prefix string) ([]storage.ObjectInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []storage.ObjectInfo{}
	for k, o := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, storage.ObjectInfo{Key: k, LastModified: o.modTime})
		}
	}
	return out, nil
}
