package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func prep(t *testing.T, srv *Server) {
	t.Helper()
	srv.inv = studyFake()
	addSource(t, srv.store, "src1", 2)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare failed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSectionPageRendersReadingAndQuiz(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/0", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "quiz-form") || !strings.Contains(body, "why") {
		t.Fatalf("section page missing quiz: %q", body)
	}
}

func TestLockedSectionShowsNotice(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	// Section 1 is locked (section 0 only unlocked after prepare).
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Finish the previous section first") {
		t.Fatalf("expected locked notice, got %q", rec.Body.String())
	}
}

func TestSectionOutOfRangeIs404(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/9", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	_ = context.Background
	_ = model.StatusLocked
}
