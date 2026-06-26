package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/build"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func testServer(t *testing.T) (*Server, store.Store) {
	t.Helper()
	st, err := store.NewSQLite(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	n := 0
	srv, err := NewServer(st, &agent.Fake{}, func() string { n++; return "id" + string(rune('0'+n)) },
		&build.FakeRunner{Result: build.Result{Passed: true}}, t.TempDir())
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv, st
}

func TestHomeListsSources(t *testing.T) {
	srv, st := testServer(t)
	st.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	})
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "My Paper") || !strings.Contains(body, "0/1") {
		t.Fatalf("home body missing source/progress: %q", body)
	}
}

func TestStaticAssetsServed(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest("GET", "/static/98.css", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
		t.Fatalf("98.css not served: status=%d len=%d", rec.Code, rec.Body.Len())
	}
}
