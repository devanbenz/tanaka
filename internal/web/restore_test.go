package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func getSection(t *testing.T, srv *Server, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d; body=%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestSectionRestoresFreeAnswer(t *testing.T) {
	srv, _ := testServer(t)
	srv.inv = &fakeGrader{verdict: "pass", feedback: "good reasoning"}
	preppedWithFreeQ(t, srv)

	// Answer the free-response question, then revisit the section.
	if rec := gradeReq(t, srv, `{"questionId":"q1","answer":"because gravity"}`); rec.Code != http.StatusOK {
		t.Fatalf("grade status = %d; body=%s", rec.Code, rec.Body.String())
	}
	body := getSection(t, srv, "/study/src1/0")

	if !strings.Contains(body, "because gravity") {
		t.Fatalf("saved answer not restored in textarea: %q", body)
	}
	if !strings.Contains(body, "verdict-pass") || !strings.Contains(body, "pass - good reasoning") {
		t.Fatalf("saved verdict/feedback not restored: %q", body)
	}
}

func TestSectionRestoresMCQChoice(t *testing.T) {
	srv, _ := testServer(t)
	ctx := context.Background()
	addSource(t, srv.store, "src1", 2)
	if err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "src1-s0", Summary: "s", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "qm", SectionID: "src1-s0", Idx: 0, Kind: model.KindMCQ,
			Prompt: "pick", Options: []string{"Paris", "Rome", "Berlin"}, CorrectIndex: 0, Explanation: "Paris is right"}},
	}); err != nil {
		t.Fatal(err)
	}
	srv.store.SetSectionStatus(ctx, "src1-s0", model.StatusUnlocked)

	// Choose index 2 (wrong -> fail), then revisit.
	if rec := gradeReq(t, srv, `{"questionId":"qm","choice":2}`); rec.Code != http.StatusOK {
		t.Fatalf("grade status = %d; body=%s", rec.Code, rec.Body.String())
	}
	body := getSection(t, srv, "/study/src1/0")

	if !strings.Contains(body, `value="2" checked`) {
		t.Fatalf("chosen MCQ option not restored as checked: %q", body)
	}
	if !strings.Contains(body, "verdict-fail") {
		t.Fatalf("MCQ verdict not restored: %q", body)
	}
}

func TestSectionUnansweredHasNoVerdictClass(t *testing.T) {
	srv, _ := testServer(t)
	preppedWithFreeQ(t, srv)
	body := getSection(t, srv, "/study/src1/0")
	// The verdict div renders empty (no verdict-pass/partial/fail) until answered.
	if strings.Contains(body, "verdict-pass") || strings.Contains(body, "verdict-fail") || strings.Contains(body, "verdict-partial") {
		t.Fatalf("unanswered question should have no verdict class: %q", body)
	}
}
