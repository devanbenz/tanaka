package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
)

func gradeReq(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/grade", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

// Build a prepared source with one free question we control directly.
func preppedWithFreeQ(t *testing.T, srv *Server) string {
	t.Helper()
	ctx := context.Background()
	addSource(t, srv.store, "src1", 2)
	err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "src1-s0", Summary: "s", KeyConcepts: []string{"k"},
		Questions: []model.Question{{ID: "q1", SectionID: "src1-s0", Idx: 0, Kind: model.KindFree, Prompt: "why", Rubric: "r"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Second section too, so "unlock next" is observable.
	if err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{SectionID: "src1-s1", Summary: "s2", KeyConcepts: []string{"k"}}); err != nil {
		t.Fatal(err)
	}
	srv.store.SetSectionStatus(ctx, "src1-s0", model.StatusUnlocked)
	return "q1"
}

// fakeGrader implements agent.Invoker, always returning a fixed verdict JSON.
type fakeGrader struct {
	verdict, feedback string
}

func (f *fakeGrader) Check(ctx context.Context) error { return nil }
func (f *fakeGrader) Invoke(ctx context.Context, job agent.Job) (json.RawMessage, error) {
	return json.RawMessage(`{"verdict":"` + f.verdict + `","feedback":"` + f.feedback + `"}`), nil
}

func TestGradeFreePassUnlocksNext(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = &fakeGrader{verdict: "pass", feedback: "good"}
	preppedWithFreeQ(t, srv)
	rec := gradeReq(t, srv, `{"questionId":"q1","answer":"because"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Verdict       string `json:"verdict"`
		SectionPassed bool   `json:"sectionPassed"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Verdict != "pass" || !resp.SectionPassed {
		t.Fatalf("resp = %+v, want pass + sectionPassed", resp)
	}
	// Section 0 should now be passed and section 1 unlocked.
	statuses, _ := st.GetSectionStatuses(context.Background(), "src1")
	if statuses["src1-s0"] != model.StatusPassed {
		t.Fatalf("s0 = %q, want passed", statuses["src1-s0"])
	}
	if statuses["src1-s1"] != model.StatusUnlocked {
		t.Fatalf("s1 = %q, want unlocked", statuses["src1-s1"])
	}
}

func TestGradeUnknownQuestion404(t *testing.T) {
	srv, _ := testServer(t)
	rec := gradeReq(t, srv, `{"questionId":"nope","answer":"x"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
