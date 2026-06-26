package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func addSource(t *testing.T, st store.Store, id string, nSections int) {
	t.Helper()
	src := &model.Source{ID: id, Title: "T", Origin: "o", CreatedAt: time.Unix(1, 0)}
	for i := 0; i < nSections; i++ {
		src.Sections = append(src.Sections, model.Section{
			ID: id + "-s" + string(rune('0'+i)), SourceID: id, Idx: i, Title: "S", Markdown: "body",
		})
	}
	if err := st.SaveSource(context.Background(), src); err != nil {
		t.Fatal(err)
	}
}

func studyFake() *agent.Fake {
	return &agent.Fake{Responses: map[string]json.RawMessage{
		"study package": json.RawMessage(`{"summary":"s","key_concepts":["k"],"questions":[{"kind":"free","prompt":"why","rubric":"r"}]}`),
	}}
}

func TestStudyEntryUnpreparedShowsPreparePage(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 2)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "Prepare this source") {
		t.Fatalf("expected prepare page, got %q", rec.Body.String())
	}
}

func TestStudyEntryUnknownIs404(t *testing.T) {
	srv, _ := testServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPrepareThenEntryRedirects(t *testing.T) {
	srv, st := testServer(t)
	srv.inv = studyFake() // server uses the fake invoker for PrepareSource
	addSource(t, st, "src1", 2)
	// Prepare: now async — returns 303 to /study/src1 immediately.
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/prepare", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("prepare status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/study/src1" {
		t.Fatalf("prepare redirect = %q, want /study/src1", loc)
	}
	// Wait for the background job to complete.
	waitJob(t, srv.jobs, "prepare:src1")
	// Now entry redirects to current section instead of the prepare page.
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, httptest.NewRequest("GET", "/study/src1", nil))
	if rec2.Code != http.StatusSeeOther {
		t.Fatalf("entry status = %d, want 303", rec2.Code)
	}
	if loc := rec2.Header().Get("Location"); !strings.HasPrefix(loc, "/study/src1/") {
		t.Fatalf("entry redirect = %q", loc)
	}
}
