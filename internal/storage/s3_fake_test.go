package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
)

// fakeS3 is a tiny in-memory, path-style S3-compatible server: enough of
// PutObject / GetObject / DeleteObject / ListObjectsV2 to drive S3Storage
// against a simulated backend without any network or real bucket.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Path style: /{bucket}/{key...}
	trimmed := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.SplitN(trimmed, "/", 2)
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch {
	case r.Method == http.MethodGet && r.URL.Query().Has("list-type"):
		prefix := r.URL.Query().Get("prefix")
		var keys []string
		for k := range f.objects {
			if strings.HasPrefix(k, prefix) {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
		b.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
		b.WriteString(fmt.Sprintf("<Name>%s</Name><Prefix>%s</Prefix><KeyCount>%d</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>", parts[0], prefix, len(keys)))
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-01-01T00:00:00.000Z</LastModified><ETag>\"x\"</ETag><StorageClass>STANDARD</StorageClass></Contents>", k, len(f.objects[k])))
		}
		b.WriteString(`</ListBucketResult>`)
		w.Header().Set("Content-Type", "application/xml")
		io.WriteString(w, b.String())

	case r.Method == http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		f.objects[key] = body // body may be aws-chunked framed; presence is what we assert
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet:
		data, ok := f.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `<?xml version="1.0"?><Error><Code>NoSuchKey</Code></Error>`)
			return
		}
		w.Write(data)

	case r.Method == http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusBadRequest)
	}
}

func newFakeS3Storage(t *testing.T) (*S3Storage, *fakeS3) {
	t.Helper()
	fake := &fakeS3{objects: map[string][]byte{}}
	ts := httptest.NewServer(fake)
	t.Cleanup(ts.Close)

	s, err := New("s3", map[string]string{
		"bucket":            "backups",
		"endpoint":          ts.URL, // httptest serves http
		"insecure_endpoint": "true",
		"region":            "us-east-1",
		"access_key":        "test",
		"secret_key":        "test",
	})
	if err != nil {
		t.Fatalf("New s3: %v", err)
	}
	return s.(*S3Storage), fake
}

func TestS3UploadReachesBackend(t *testing.T) {
	s, fake := newFakeS3Storage(t)
	if err := s.Upload(context.Background(), "site/a.zip", bytes.NewReader([]byte("payload"))); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if _, ok := fake.objects["site/a.zip"]; !ok {
		t.Error("uploaded object did not reach the backend under the expected key")
	}
}

func TestS3DownloadReturnsObject(t *testing.T) {
	s, fake := newFakeS3Storage(t)
	fake.objects["dl/clean.txt"] = []byte("hello s3")

	var buf bytes.Buffer
	if err := s.Download(context.Background(), "dl/clean.txt", &buf); err != nil {
		t.Fatalf("Download: %v", err)
	}
	if buf.String() != "hello s3" {
		t.Errorf("downloaded %q, want %q", buf.String(), "hello s3")
	}
}

func TestS3DownloadMissingErrors(t *testing.T) {
	s, _ := newFakeS3Storage(t)
	var buf bytes.Buffer
	if err := s.Download(context.Background(), "nope.zip", &buf); err == nil {
		t.Error("expected error downloading a missing object")
	}
}

func TestS3List(t *testing.T) {
	s, fake := newFakeS3Storage(t)
	fake.objects["site/a.zip"] = []byte("a")
	fake.objects["site/b.zip"] = []byte("b")
	fake.objects["other/c.zip"] = []byte("c")

	keys, err := s.List(context.Background(), "site/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2: %v", len(keys), keys)
	}
	for _, k := range keys {
		if !strings.HasPrefix(k, "site/") {
			t.Errorf("unexpected key %q outside the prefix", k)
		}
	}
}

func TestS3Delete(t *testing.T) {
	s, fake := newFakeS3Storage(t)
	fake.objects["del/x.zip"] = []byte("x")

	if err := s.Delete(context.Background(), "del/x.zip"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := fake.objects["del/x.zip"]; ok {
		t.Error("object still present after Delete")
	}
}
