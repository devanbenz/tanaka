package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

func seedHomeSource(t *testing.T, st store.Store) {
	t.Helper()
	if err := st.SaveSource(context.Background(), &model.Source{
		ID: "src1", Title: "My Paper", Origin: "o", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "s0", SourceID: "src1", Idx: 0, Title: "A", Markdown: "a"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteSourceRemovesAndRedirects(t *testing.T) {
	srv, st := testServer(t)
	seedHomeSource(t, st)
	req := httptest.NewRequest("POST", "/sources/src1/delete", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/" {
		t.Fatalf("redirect = %q, want /", loc)
	}
	if _, err := st.GetSource(context.Background(), "src1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("source still present after delete: err = %v", err)
	}
}

func TestDeleteUnknownSource404s(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest("POST", "/sources/nope/delete", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Each source row offers a guarded delete form and styles build/export as
// visible actions.
func TestHomeRendersDeleteAndActionLinks(t *testing.T) {
	srv, st := testServer(t)
	seedHomeSource(t, st)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`action="/sources/src1/delete"`,
		`class="link-action delete"`,
		"return confirm(",
		`<a class="action" href="/build/src1">build</a>`,
		`<a class="action" href="/export/src1">export</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("home body missing %q:\n%s", want, body)
		}
	}
}
