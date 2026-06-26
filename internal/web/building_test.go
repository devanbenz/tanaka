package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBuildStartIsAsyncAndShowsBuilding(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = buildFakeAgent() // from build_test.go
	addBuildSource(t, st, "src1")

	form := url.Values{"language": {"go"}, "difficulty": {"spec+tests"}}
	req := httptest.NewRequest("POST", "/build/src1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("start status = %d, want 303", rec.Code)
	}
	waitJob(t, srv.jobs, "build:src1:go")
}

func TestBuildViewShowsErrorWhenBuildFailed(t *testing.T) {
	srv, st := testServer(t)
	addBuildSource(t, st, "src1")
	srv.jobs.Start("build:src1:go", "build", "src1", "go", func(progress func(string)) error {
		return errors.New("kaboom")
	})
	waitJob(t, srv.jobs, "build:src1:go")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1/go", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "kaboom") || !strings.Contains(body, "Retry") {
		t.Fatalf("expected error + retry, got %q", body)
	}
}

func TestBuildViewShowsBuildingWhileRunning(t *testing.T) {
	srv, st := testServer(t)
	addBuildSource(t, st, "src1")
	release := make(chan struct{})
	srv.jobs.Start("build:src1:go", "build", "src1", "go", func(progress func(string)) error {
		<-release
		return nil
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1/go", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Building") {
		t.Fatalf("expected building page, got %q", rec.Body.String())
	}
	close(release)
	waitJob(t, srv.jobs, "build:src1:go")
}
