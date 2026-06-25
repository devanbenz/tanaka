package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
	"github.com/devandbenz/tanaka/internal/study"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets/*
var assetsFS embed.FS

// Server holds dependencies for the study UI.
type Server struct {
	store store.Store
	inv   agent.Invoker
	newID func() string
	tmpl  *template.Template
}

// NewServer parses the embedded templates and returns a Server.
func NewServer(st store.Store, inv agent.Invoker, newID func() string) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: st, inv: inv, newID: newID, tmpl: tmpl}, nil
}

// Handler returns the HTTP router.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(assetsFS, "assets")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))
	mux.HandleFunc("GET /{$}", s.handleHome)
	return mux
}

// render executes the base template with the named content block and data.
func (s *Server) render(w http.ResponseWriter, page string, data map[string]any) {
	t, err := s.tmpl.Clone()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := t.ParseFS(templatesFS, "templates/"+page); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

type sourceRow struct {
	ID, Title   string
	Done, Total int
}

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sources, err := s.store.ListSources(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rows []sourceRow
	for _, src := range sources {
		full, err := s.store.GetSource(ctx, src.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		statuses, err := s.store.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		done := 0
		for _, st := range study.OrderedStatuses(full, statuses) {
			if st == model.StatusPassed || st == model.StatusSkipped {
				done++
			}
		}
		rows = append(rows, sourceRow{ID: src.ID, Title: src.Title, Done: done, Total: len(full.Sections)})
	}
	s.render(w, "home.html", map[string]any{"Title": "", "Sources": rows})
}
