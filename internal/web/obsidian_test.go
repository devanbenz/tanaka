package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/build"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func testServerWithObsidian(t *testing.T, obsidianDir string) (*Server, store.Store) {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := 0
	srv, err := NewServer(st, &agent.Fake{}, func() string { n++; return "id" + string(rune('0'+n)) },
		&build.FakeRunner{Result: build.Result{Passed: true}},
		t.TempDir(), obsidianDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, st
}

// seedMCQ seeds one section with nq multiple-choice questions (q0, q1, ...);
// the correct choice is always 1.
func seedMCQ(t *testing.T, st store.Store, nq int) {
	t.Helper()
	ctx := context.Background()
	if err := st.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	study := &model.SectionStudy{SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"}}
	for i := range nq {
		study.Questions = append(study.Questions, model.Question{
			ID: "q" + itoa(i), SectionID: "sec0", Idx: i, Kind: model.KindMCQ,
			Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "y!"})
	}
	if err := st.SaveSectionStudy(ctx, study); err != nil {
		t.Fatal(err)
	}
}

func grade(t *testing.T, srv *Server, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", "/grade", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	return rec
}

// A passing answer syncs immediately, before the section is complete, and
// only the answered question appears in the vault.
func TestObsidianSyncOnEachAnswer(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st, 2)
	grade(t, srv, `{"questionId":"q0","choice":1}`)
	srv.DrainObsidian()
	b, err := os.ReadFile(filepath.Join(vault, "my-paper", "questions", "01 Intro Q1.md"))
	if err != nil {
		t.Fatalf("answered question not synced before section pass: %v", err)
	}
	if !strings.Contains(string(b), "verdict: pass") {
		t.Fatalf("synced note missing verdict:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(vault, "my-paper", "questions", "01 Intro Q2.md")); err == nil {
		t.Fatal("unanswered question was synced")
	}
	sec := filepath.Join(vault, "my-paper", "sections", "01 Intro.md")
	if s, err := os.ReadFile(sec); err != nil {
		t.Fatalf("section note not synced: %v", err)
	} else if strings.Contains(string(s), "[[01 Intro Q2]]") {
		t.Fatalf("section note links unanswered question:\n%s", s)
	}
}

// A failing answer leaves no trace in the vault.
func TestObsidianNoSyncOnFail(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st, 1)
	grade(t, srv, `{"questionId":"q0","choice":0}`)
	srv.DrainObsidian()
	if _, err := os.Stat(filepath.Join(vault, "my-paper")); !os.IsNotExist(err) {
		t.Fatalf("failed answer should not create vault files, stat err = %v", err)
	}
}

// Skipping a section with no completed questions writes nothing.
func TestObsidianNoSyncOnSkipWithoutProgress(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st, 1)
	req := httptest.NewRequest("POST", "/study/src1/0/skip", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	srv.DrainObsidian()
	if _, err := os.Stat(filepath.Join(vault, "my-paper")); !os.IsNotExist(err) {
		t.Fatalf("skip without progress should not create vault files, stat err = %v", err)
	}
}

func TestObsidianSyncOnSectionPass(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st, 1)
	grade(t, srv, `{"questionId":"q0","choice":1}`)
	srv.DrainObsidian()
	b, err := os.ReadFile(filepath.Join(vault, "my-paper", "questions", "01 Intro Q1.md"))
	if err != nil {
		t.Fatalf("question note not synced: %v", err)
	}
	if !strings.Contains(string(b), "verdict: pass") {
		t.Fatalf("synced note missing verdict:\n%s", b)
	}
}

func TestObsidianSyncDisabled(t *testing.T) {
	srv, st := testServerWithObsidian(t, "")
	seedMCQ(t, st, 1)
	req := httptest.NewRequest("POST", "/study/src1/0/skip", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	srv.DrainObsidian() // returns immediately; no goroutine was spawned
}
