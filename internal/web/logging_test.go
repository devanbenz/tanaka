package web

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogRequestsEmitsLine(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	h := logRequests(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/x", nil))
	out := buf.String()
	if !strings.Contains(out, "request") || !strings.Contains(out, "method=GET") ||
		!strings.Contains(out, "path=/x") || !strings.Contains(out, "status=418") {
		t.Fatalf("log line missing fields: %q", out)
	}
}

func TestStatusRecorderDefaults200(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: 200}
	sr.Write([]byte("hi")) // no WriteHeader -> stays 200
	if sr.status != 200 {
		t.Fatalf("status = %d, want 200", sr.status)
	}
}
