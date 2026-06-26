package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func buildFakeAgent() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"build plan": json.RawMessage(`{"skeleton_files":[{"path":"go.mod","content":"module x"}],"steps":[{"goal":"parse input","files":[{"path":"parse_test.go","content":"package x"}]},{"goal":"compute","files":[{"path":"compute_test.go","content":"package x"}]}]}`),
		"hint":       json.RawMessage(`{"hint":"try the base case"}`),
	}}
}

func addBuildSource(t *testing.T, st store.Store, id string) {
	t.Helper()
	if err := st.SaveSource(context.Background(), &model.Source{
		ID: id, Title: "Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: id + "-s0", SourceID: id, Idx: 0, Title: "A", Markdown: "alpha"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildPickerShown(t *testing.T) {
	srv, st := testServer(t)
	addBuildSource(t, st, "src1")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/src1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"rust", "guided", "blank-page", "Start"} {
		if !strings.Contains(body, want) {
			t.Fatalf("picker missing %q: %q", want, body)
		}
	}
}

func TestBuildPickerUnknownSource404(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/build/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestBuildStartRedirectsToView(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = buildFakeAgent()
	addBuildSource(t, st, "src1")
	form := url.Values{"language": {"go"}, "difficulty": {"spec+tests"}}
	req := httptest.NewRequest("POST", "/build/src1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/build/src1/go" {
		t.Fatalf("redirect = %q, want /build/src1/go", loc)
	}
	// Build is created by the background job; wait for it to finish.
	waitJob(t, srv.jobs, "build:src1:go")
	if _, err := st.GetBuild(context.Background(), "src1", "go"); err != nil {
		t.Fatalf("build not persisted: %v", err)
	}
}

func TestBuildStartResumesExisting(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = buildFakeAgent()
	addBuildSource(t, st, "src1")
	post := func() *httptest.ResponseRecorder {
		form := url.Values{"language": {"go"}, "difficulty": {"spec+tests"}}
		req := httptest.NewRequest("POST", "/build/src1", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		return rec
	}
	if post().Code != http.StatusSeeOther {
		t.Fatal("first start should redirect")
	}
	// Second start with same language must not error (resume), still 303.
	if rec := post(); rec.Code != http.StatusSeeOther {
		t.Fatalf("resume start status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
}
