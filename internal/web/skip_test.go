package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/devandbenz/tanaka/internal/model"
)

func TestSkipMarksSkippedAndUnlocksNext(t *testing.T) {
	srv, st := testServer(t)
	prep(t, srv) // src1 with 2 sections, section 0 unlocked
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("POST", "/study/src1/0/skip", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/study/src1/1" {
		t.Fatalf("redirect = %q, want /study/src1/1", loc)
	}
	statuses, _ := st.GetSectionStatuses(context.Background(), "src1")
	if statuses["src1-s0"] != model.StatusSkipped {
		t.Fatalf("s0 = %q, want skipped", statuses["src1-s0"])
	}
	if statuses["src1-s1"] != model.StatusUnlocked {
		t.Fatalf("s1 = %q, want unlocked", statuses["src1-s1"])
	}
}
