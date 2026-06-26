package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devandbenz/tanaka/internal/build"
	"github.com/devandbenz/tanaka/internal/model"
)

func postTest(t *testing.T, srv *Server) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/build/src1/go/test", nil))
	return rec
}

func TestRunTestsPassAdvances(t *testing.T) {
	srv, st := testServer(t)
	srv.runner = &build.FakeRunner{Result: build.Result{Passed: true, Output: "ok"}}
	startBuild(t, srv)
	rec := postTest(t, srv)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Passed   bool `json:"passed"`
		Complete bool `json:"complete"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Passed {
		t.Fatalf("resp = %+v, want passed", resp)
	}
	// Step 0 now passed, step 1 unlocked.
	b, _ := st.GetBuild(context.Background(), "src1", "go")
	if b.Steps[0].Status != model.StatusPassed || b.Steps[1].Status != model.StatusUnlocked {
		t.Fatalf("steps after pass: %+v", b.Steps)
	}
}

func TestRunTestsFailDoesNotAdvance(t *testing.T) {
	srv, st := testServer(t)
	srv.runner = &build.FakeRunner{Result: build.Result{Passed: false, Output: "assertion failed"}}
	startBuild(t, srv)
	rec := postTest(t, srv)
	var resp struct {
		Passed bool   `json:"passed"`
		Output string `json:"output"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Passed || resp.Output != "assertion failed" {
		t.Fatalf("resp = %+v, want failed with output", resp)
	}
	b, _ := st.GetBuild(context.Background(), "src1", "go")
	if b.Steps[0].Status != model.StatusUnlocked {
		t.Fatalf("step 0 should stay unlocked on failure: %+v", b.Steps)
	}
}

func TestRunTestsRunError(t *testing.T) {
	srv, _ := testServer(t)
	srv.runner = &build.FakeRunner{Result: build.Result{RunError: true, Output: "cargo: not found"}}
	startBuild(t, srv)
	rec := postTest(t, srv)
	var resp struct {
		RunError bool `json:"runError"`
		Passed   bool `json:"passed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.RunError || resp.Passed {
		t.Fatalf("resp = %+v, want runError", resp)
	}
}
