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
	waitJob(t, srv.jobs, "prepare:src1")
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

func TestSectionMCQRadiosShareOneName(t *testing.T) {
	srv, st := testServer(t)
	ctx := context.Background()
	addSource(t, st, "src1", 1)
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "src1-s0", Summary: "s", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "qm", SectionID: "src1-s0", Idx: 0, Kind: model.KindMCQ, Prompt: "pick", Options: []string{"Paris", "Rome", "Berlin"}, CorrectIndex: 0}},
	}); err != nil {
		t.Fatal(err)
	}
	st.SetSectionStatus(ctx, "src1-s0", model.StatusUnlocked)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/0", nil))
	body := rec.Body.String()
	if c := strings.Count(body, `name="q-qm"`); c != 3 {
		t.Fatalf("expected 3 radios sharing name q-qm, got %d; body=%s", c, body)
	}
	if strings.Contains(body, `name="opt-`) {
		t.Fatalf("old per-option name still present")
	}
	// correct answer must not leak into the page
	if strings.Contains(body, "CorrectIndex") {
		t.Fatalf("correct answer leaked")
	}
}

func TestNextNotNavigableUntilSectionDone(t *testing.T) {
	srv, _ := testServer(t)
	prep(t, srv) // src1, 2 sections, section 0 unlocked, not yet passed
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1/0", nil))
	body := rec.Body.String()
	// Section 0 is not passed/skipped, so there must be no navigable link to section 1.
	if strings.Contains(body, `href="/study/src1/1"`) {
		t.Fatalf("Next should not be a followable link before the section is done; body=%s", body)
	}
}
