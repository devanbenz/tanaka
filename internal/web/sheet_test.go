package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

func seedForExport(t *testing.T, srv *Server) {
	t.Helper()
	ctx := context.Background()
	if err := srv.store.SaveSource(ctx, &model.Source{
		ID: "src1", Title: "My Paper", Origin: "http://x", CreatedAt: time.Unix(1, 0),
		Sections: []model.Section{{ID: "sec0", SourceID: "src1", Idx: 0, Title: "Intro", Markdown: "# hi"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.store.SaveSectionStudy(ctx, &model.SectionStudy{
		SectionID: "sec0", Summary: "sum", KeyConcepts: []string{"a"},
		Questions: []model.Question{{ID: "q0", SectionID: "sec0", Idx: 0, Kind: model.KindFree, Prompt: "why", Rubric: "r"}},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExportEndpoint(t *testing.T) {
	srv, _ := testServer(t)
	seedForExport(t, srv)
	req := httptest.NewRequest("GET", "/export/src1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd == "" {
		t.Fatal("missing Content-Disposition")
	}
	// Body must be valid gzip (magic bytes 0x1f 0x8b).
	b := rec.Body.Bytes()
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		t.Fatalf("body is not gzip: %v", b[:min(2, len(b))])
	}
}

func TestExportEndpointNotFound(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest("GET", "/export/missing", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestImportEndpointRoundTrip(t *testing.T) {
	// Export from one server, import into another.
	srvA, _ := testServer(t)
	seedForExport(t, srvA)
	exportRec := httptest.NewRecorder()
	srvA.Handler().ServeHTTP(exportRec, httptest.NewRequest("GET", "/export/src1", nil))
	sheetBytes := exportRec.Body.Bytes()

	srvB, stB := testServer(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, err := mw.CreateFormFile("sheet", "x.tanaka")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(sheetBytes)
	mw.Close()

	req := httptest.NewRequest("POST", "/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srvB.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body=%s", rec.Code, rec.Body.String())
	}
	srcs, err := stB.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(srcs) != 1 || srcs[0].Title != "My Paper" {
		t.Fatalf("imported source wrong: %+v", srcs)
	}
}

func TestImportEndpointBadFile(t *testing.T) {
	srv, _ := testServer(t)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("sheet", "x.tanaka")
	fw.Write([]byte("not gzip"))
	mw.Close()
	req := httptest.NewRequest("POST", "/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
