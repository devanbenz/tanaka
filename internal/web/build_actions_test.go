package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestBuildSkipAdvances(t *testing.T) {
	srv, st := testServer(t)
	startBuild(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/build/src1/go/skip", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/build/src1/go" {
		t.Fatalf("redirect = %q", loc)
	}
	b, _ := st.GetBuild(context.Background(), "src1", "go")
	if b.Steps[0].Status != model.StatusSkipped || b.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("steps after skip: %+v", b.Steps)
	}
}

func TestBuildHint(t *testing.T) {
	srv, _ := testServer(t)
	srv.inv = buildFakeAgent() // responds to "hint"
	startBuild(t, srv)
	body := strings.NewReader(`{"output":"FAIL: boom"}`)
	req := httptest.NewRequest("POST", "/build/src1/go/hint", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Hint string `json:"hint"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.Contains(resp.Hint, "base case") {
		t.Fatalf("hint = %q", resp.Hint)
	}
}

func TestReadWorkspaceText(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "a.go"), []byte("package main"), 0o644)
	os.MkdirAll(filepath.Join(ws, "sub"), 0o755)
	os.WriteFile(filepath.Join(ws, "sub", "b.txt"), []byte("hello"), 0o644)
	got := readWorkspaceText(ws)
	if !strings.Contains(got, "package main") || !strings.Contains(got, "hello") {
		t.Fatalf("readWorkspaceText missing content: %q", got)
	}
	if !strings.Contains(got, "a.go") {
		t.Fatalf("readWorkspaceText should label paths: %q", got)
	}
}
