package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

// startBuild drives the start endpoint and waits until the build job completes.
func startBuild(t *testing.T, srv *Server) {
	t.Helper()
	srv.inv = buildFakeAgent()
	addBuildSource(t, srv.store, "src1")
	form := url.Values{"language": {"go"}, "difficulty": {"spec+tests"}}
	req := httptest.NewRequest("POST", "/build/src1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start failed: %d %s", rec.Code, rec.Body.String())
	}
	waitJob(t, srv.jobs, "build:src1:go")
}

func TestBuildViewRendersCurrentStep(t *testing.T) {
	srv, _ := testServer(t)
	startBuild(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1/go", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "parse input") { // first step goal
		t.Fatalf("view missing current step goal: %q", body)
	}
	if !strings.Contains(body, "src1-go") { // workspace path
		t.Fatalf("view missing workspace path: %q", body)
	}
	if !strings.Contains(body, "Run tests") {
		t.Fatalf("view missing run-tests control: %q", body)
	}
}

func TestBuildViewUnknown404(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1/go", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestCurrentBuildStep(t *testing.T) {
	b := &model.Build{Steps: []model.BuildStep{
		{Status: model.StatusPassed}, {Status: model.StatusUnlocked}, {Status: model.StatusLocked},
	}}
	if got := currentBuildStep(b); got != 1 {
		t.Fatalf("currentBuildStep = %d, want 1", got)
	}
	done := &model.Build{Steps: []model.BuildStep{{Status: model.StatusPassed}, {Status: model.StatusSkipped}}}
	if got := currentBuildStep(done); got != -1 {
		t.Fatalf("currentBuildStep(all done) = %d, want -1", got)
	}
}
