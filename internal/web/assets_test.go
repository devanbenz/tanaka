package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKaomojiAndJobsAssetsServed(t *testing.T) {
	srv, _ := testServer(t)
	for _, path := range []string{"/static/kaomoji.js", "/static/jobs.js"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || rec.Body.Len() == 0 {
			t.Fatalf("%s not served: status=%d len=%d", path, rec.Code, rec.Body.Len())
		}
	}
}

func TestBaseLoadsKaomojiAndJobs(t *testing.T) {
	srv, st := testServer(t)
	addSource(t, st, "src1", 1) // a page that renders base via a sub-page
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/study/src1", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "/static/kaomoji.js") || !strings.Contains(body, "/static/jobs.js") {
		t.Fatalf("base.html does not load kaomoji.js + jobs.js: %q", body)
	}
}
