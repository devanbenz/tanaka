package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareStartsJobAndShowsPreparing(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = studyFake() // from prepare_test.go; answers "study package"
	addSource(t, st, "src1", 1) // from prepare_test.go helper

	// POST prepare returns fast (303) and registers a job.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303; %s", rec.Code, rec.Body.String())
	}
	// The job exists and finishes.
	waitJob(t, srv.jobs, "prepare:src1")
}

func TestStudyEntryShowsErrorWhenPrepareFailed(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 1)
	srv.jobs.Start("prepare:src1", "prepare", "src1", "", func(progress func(string)) error {
		return errors.New("kaboom")
	})
	waitJob(t, srv.jobs, "prepare:src1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "kaboom") || !strings.Contains(body, "Retry") {
		t.Fatalf("expected error + retry, got %q", body)
	}
}

func TestStudyEntryShowsPreparingWhileRunning(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 1)
	// Register a still-running prepare job by hand (no real work).
	release := make(chan struct{})
	srv.jobs.Start("prepare:src1", "prepare", "src1", "", func(progress func(string)) error {
		progress("section 1/1")
		<-release
		return nil
	})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "window.__pollJob") {
		t.Fatalf("expected preparing page (window.__pollJob), got %q", rec.Body.String())
	}
	close(release)
	waitJob(t, srv.jobs, "prepare:src1")
}
