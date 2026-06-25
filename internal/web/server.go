package web

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"strconv"

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
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic("web: assets sub-FS: " + err.Error())
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(sub)))
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /study/{id}", s.handleStudyEntry)
	mux.HandleFunc("POST /study/{id}/prepare", s.handlePrepare)
	mux.HandleFunc("GET /study/{id}/{idx}", s.handleSection)
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

func (s *Server) handleStudyEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	prepared, err := s.store.IsPrepared(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !prepared {
		s.render(w, "prepare.html", map[string]any{"Title": src.Title, "Source": src})
		return
	}
	statuses, err := s.store.GetSectionStatuses(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	idx := study.CurrentSectionIdx(study.OrderedStatuses(src, statuses))
	http.Redirect(w, r, "/study/"+id+"/"+itoa(idx), http.StatusSeeOther)
}

func (s *Server) handlePrepare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := study.PrepareSource(ctx, s.inv, s.store, src, s.newID, nil); err != nil {
		http.Error(w, "prepare failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/study/"+id+"/0", http.StatusSeeOther)
}

func itoa(i int) string { return strconv.Itoa(i) }

type navItem struct {
	Idx      int
	Title    string
	Mark     string
	Current  bool
	Unlocked bool
}

func mark(status string) string {
	switch status {
	case model.StatusPassed:
		return "[x]"
	case model.StatusSkipped:
		return "[-]"
	default:
		return "[ ]"
	}
}

func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if idx < 0 || idx >= len(src.Sections) {
		http.NotFound(w, r)
		return
	}
	statuses, err := s.store.GetSectionStatuses(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ordered := study.OrderedStatuses(src, statuses)
	unlocked := study.ComputeUnlocked(ordered)
	if !unlocked[idx] {
		s.render(w, "locked.html", map[string]any{"Title": "Locked", "SourceID": id, "PrevIdx": idx - 1})
		return
	}
	sec := src.Sections[idx]
	stud, err := s.store.GetSectionStudy(ctx, sec.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	body, err := renderMarkdown(sec.Markdown)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var nav []navItem
	done := 0
	for i, seci := range src.Sections {
		nav = append(nav, navItem{Idx: i, Title: seci.Title, Mark: mark(ordered[i]), Current: i == idx, Unlocked: unlocked[i]})
		if ordered[i] == model.StatusPassed || ordered[i] == model.StatusSkipped {
			done++
		}
	}
	hasNext := idx+1 < len(src.Sections)
	data := map[string]any{
		"Title": src.Title, "Source": src, "Section": sec, "Body": body,
		"Concepts": stud.KeyConcepts, "Questions": stud.Questions, "Nav": nav,
		"Done": done, "Total": len(src.Sections),
		"HasNext": hasNext, "NextIdx": idx + 1,
		"NextUnlocked": ordered[idx] == model.StatusPassed || ordered[idx] == model.StatusSkipped,
	}
	s.render(w, "section.html", data)
}
