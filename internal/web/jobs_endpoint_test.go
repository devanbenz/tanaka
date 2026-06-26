package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJobsEndpointEmpty(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jobs", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var jobs []Job
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("body not JSON array: %v (%s)", err, rec.Body.String())
	}
	if len(jobs) != 0 {
		t.Fatalf("want empty, got %+v", jobs)
	}
}

func TestJobsEndpointReportsRegistered(t *testing.T) {
	srv, _ := testServer(t)
	srv.jobs.Start("prepare:x", "prepare", "x", "", func(progress func(string)) error { return nil })
	// Let it finish.
	waitJob(t, srv.jobs, "prepare:x")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/jobs", nil))
	var jobs []Job
	json.Unmarshal(rec.Body.Bytes(), &jobs)
	if len(jobs) != 1 || jobs[0].Key != "prepare:x" || jobs[0].Status != "done" {
		t.Fatalf("jobs = %+v", jobs)
	}
}
