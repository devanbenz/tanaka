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

func seedMCQ(t *testing.T, st store.Store) {
	t.Helper()
	ctx := context.Background()
	if err := st.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindMCQ,
			Prompt: "pick", Options: []string{"x", "y"}, CorrectIndex: 1, Explanation: "y!"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestObsidianSyncOnSkip(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st)
	req := httptest.NewRequest("POST", "/study/src1/0/skip", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	srv.obsWG.Wait()
	if _, err := os.Stat(filepath.Join(vault, "my-paper", "My Paper.md")); err != nil {
		t.Fatalf("hub note not synced: %v", err)
	}
}

func TestObsidianSyncOnSectionPass(t *testing.T) {
	vault := t.TempDir()
	srv, st := testServerWithObsidian(t, vault)
	seedMCQ(t, st)
	req := httptest.NewRequest("POST", "/grade", strings.NewReader(`{"questionId":"q0","choice":1}`))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	srv.obsWG.Wait()
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
	seedMCQ(t, st)
	req := httptest.NewRequest("POST", "/study/src1/0/skip", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d", rec.Code)
	}
	srv.obsWG.Wait() // returns immediately; no goroutine was spawned
}
