package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := Read(context.Background(), path, nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "hello" || raw.Origin != path {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadStdin(t *testing.T) {
	raw, err := Read(context.Background(), "-", strings.NewReader("piped"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "piped" || raw.Origin != "stdin" {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("from web"))
	}))
	defer srv.Close()
	raw, err := Read(context.Background(), srv.URL, nil)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(raw.Bytes) != "from web" || raw.Origin != srv.URL {
		t.Fatalf("got %q / %q", raw.Bytes, raw.Origin)
	}
}

func TestReadMissingFile(t *testing.T) {
	_, err := Read(context.Background(), filepath.Join(t.TempDir(), "nope.md"), nil)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
