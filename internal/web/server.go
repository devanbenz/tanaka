package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/devandbenz/tanaka/internal/agent"
	"github.com/devandbenz/tanaka/internal/build"
	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/sheet"
	"github.com/devandbenz/tanaka/internal/store"
	"github.com/devandbenz/tanaka/internal/study"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed assets/*
var assetsFS embed.FS

// Server holds dependencies for the study UI.
type Server struct {
	store     store.Store
	inv       agent.Invoker
	newID     func() string
	runner    build.Runner
	buildsDir string
	log       *slog.Logger
	jobs      *JobManager
	tmpl      *template.Template
}

// NewServer parses the embedded templates and returns a Server.
func NewServer(st store.Store, inv agent.Invoker, newID func() string, runner build.Runner, buildsDir string, log *slog.Logger) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{store: st, inv: inv, newID: newID, runner: runner, buildsDir: buildsDir,
		log: log, jobs: NewJobManager(log), tmpl: tmpl}, nil
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
	mux.HandleFunc("GET /export/{id}", s.handleExport)
	mux.HandleFunc("POST /import", s.handleImport)
	mux.HandleFunc("GET /study/{id}", s.handleStudyEntry)
	mux.HandleFunc("POST /study/{id}/prepare", s.handlePrepare)
	mux.HandleFunc("GET /study/{id}/{idx}", s.handleSection)
	mux.HandleFunc("POST /grade", s.handleGrade)
	mux.HandleFunc("POST /study/{id}/{idx}/skip", s.handleSkip)
	mux.HandleFunc("GET /build/{id}", s.handleBuildEntry)
	mux.HandleFunc("POST /build/{id}", s.handleBuildStart)
	mux.HandleFunc("GET /build/{id}/{lang}", s.handleBuildView)
	mux.HandleFunc("POST /build/{id}/{lang}/test", s.handleBuildTest)
	mux.HandleFunc("POST /build/{id}/{lang}/skip", s.handleBuildSkip)
	mux.HandleFunc("POST /build/{id}/{lang}/hint", s.handleBuildHint)
	mux.HandleFunc("GET /jobs", s.handleJobs)
	return logRequests(s.log, mux)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.jobs.Snapshot())
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
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	prepared, err := s.store.IsPrepared(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !prepared {
		if j, ok := s.jobs.Get("prepare:" + id); ok {
			if j.Status == "running" {
				s.render(w, "preparing.html", map[string]any{"Title": src.Title, "SourceID": id, "Progress": j.Progress})
				return
			}
			if j.Status == "error" {
				s.render(w, "preparing.html", map[string]any{"Title": src.Title, "SourceID": id, "Error": j.Err})
				return
			}
		}
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
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jobs.Start("prepare:"+id, "prepare", id, "", func(progress func(string)) error {
		return study.PrepareSource(context.Background(), s.inv, s.store, src, s.newID,
			func(i, total int, title string) { progress(fmt.Sprintf("section %d/%d", i+1, total)) })
	})
	http.Redirect(w, r, "/study/"+id, http.StatusSeeOther)
}

func itoa(i int) string { return strconv.Itoa(i) }

type gradeRequest struct {
	QuestionID string `json:"questionId"`
	Choice     int    `json:"choice"`
	Answer     string `json:"answer"`
}

type gradeResponse struct {
	Verdict       string `json:"verdict"`
	Feedback      string `json:"feedback"`
	SectionPassed bool   `json:"sectionPassed"`
}

func (s *Server) handleGrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req gradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	q, err := s.store.GetQuestion(ctx, req.QuestionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var v study.Verdict
	if q.Kind == model.KindMCQ {
		v = study.GradeChoice(q, req.Choice)
	} else {
		sec, err := s.store.GetSection(ctx, q.SectionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		v, err = study.GradeFree(ctx, s.inv, sec.Markdown, q, req.Answer)
		if err != nil {
			http.Error(w, "grading unavailable", http.StatusBadGateway)
			return
		}
	}
	if err := s.store.SaveQuestionProgress(ctx, q.ID, v.Verdict, req.Answer, req.Choice, v.Feedback); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := gradeResponse{Verdict: v.Verdict, Feedback: v.Feedback}
	if v.Verdict != "fail" {
		satisfied, err := s.store.SectionSatisfied(ctx, q.SectionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if satisfied {
			if err := s.passAndUnlockNext(ctx, q.SectionID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp.SectionPassed = true
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// passAndUnlockNext marks the section passed and unlocks the following section
// (by source order) if it is currently locked.
func (s *Server) passAndUnlockNext(ctx context.Context, sectionID string) error {
	sec, err := s.store.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if err := s.store.SetSectionStatus(ctx, sectionID, model.StatusPassed); err != nil {
		return err
	}
	src, err := s.store.GetSource(ctx, sec.SourceID)
	if err != nil {
		return err
	}
	next := sec.Idx + 1
	if next < len(src.Sections) {
		statuses, err := s.store.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			return err
		}
		nextID := src.Sections[next].ID
		if statuses[nextID] == model.StatusLocked {
			return s.store.SetSectionStatus(ctx, nextID, model.StatusUnlocked)
		}
	}
	return nil
}

func (s *Server) handleSkip(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if idx < 0 || idx >= len(src.Sections) {
		http.NotFound(w, r)
		return
	}
	if err := s.store.SetSectionStatus(ctx, src.Sections[idx].ID, model.StatusSkipped); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	next := idx + 1
	if next < len(src.Sections) {
		statuses, err := s.store.GetSectionStatuses(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if statuses[src.Sections[next].ID] == model.StatusLocked {
			if err := s.store.SetSectionStatus(ctx, src.Sections[next].ID, model.StatusUnlocked); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		http.Redirect(w, r, "/study/"+id+"/"+itoa(next), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/study/"+id+"/"+itoa(idx), http.StatusSeeOther)
}

func (s *Server) handleBuildEntry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "build_picker.html", map[string]any{
		"Title": src.Title, "Source": src,
		"Languages":    []string{"rust", "go", "cpp", "c", "python"},
		"Difficulties": []string{"guided", "spec+tests", "blank-page"},
	})
}

func (s *Server) handleBuildStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	src, err := s.store.GetSource(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	lang := r.FormValue("language")
	diff := r.FormValue("difficulty")
	if !model.ValidLanguage(lang) || !model.ValidDifficulty(diff) {
		http.Error(w, "invalid language or difficulty", http.StatusBadRequest)
		return
	}
	// Resume if a build already exists for this language.
	if _, err := s.store.GetBuild(ctx, id, lang); err == nil {
		http.Redirect(w, r, "/build/"+id+"/"+lang, http.StatusSeeOther)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.jobs.Start("build:"+id+":"+lang, "build", id, lang, func(progress func(string)) error {
		progress("generating build plan")
		_, err := build.StartBuild(context.Background(), s.inv, s.store, src, lang, diff, s.newID, s.buildsDir)
		return err
	})
	http.Redirect(w, r, "/build/"+id+"/"+lang, http.StatusSeeOther)
}

// currentBuildStep returns the index of the first unlocked step, or -1 if the
// build has no active step (all passed/skipped = complete).
func currentBuildStep(b *model.Build) int {
	for i, st := range b.Steps {
		if st.Status == model.StatusUnlocked {
			return i
		}
	}
	return -1
}

type buildNavItem struct {
	Goal    string
	Mark    string
	Current bool
}

func (s *Server) handleBuildView(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	lang := r.PathValue("lang")
	b, err := s.store.GetBuild(ctx, id, lang)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if j, ok := s.jobs.Get("build:" + id + ":" + lang); ok {
				if j.Status == "running" {
					s.render(w, "building.html", map[string]any{"Title": id, "SourceID": id, "Lang": lang, "Progress": j.Progress})
					return
				}
				if j.Status == "error" {
					s.render(w, "building.html", map[string]any{"Title": id, "SourceID": id, "Lang": lang, "Error": j.Err})
					return
				}
			}
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := id
	if src, serr := s.store.GetSource(ctx, id); serr == nil {
		title = src.Title
	}
	cur := currentBuildStep(b)
	var nav []buildNavItem
	for i, st := range b.Steps {
		nav = append(nav, buildNavItem{Goal: st.Goal, Mark: mark(st.Status), Current: i == cur})
	}
	data := map[string]any{
		"Title": title, "SourceID": id, "Lang": lang, "Workspace": b.Workspace,
		"Nav": nav, "Complete": cur == -1,
	}
	if cur >= 0 {
		data["Step"] = b.Steps[cur]
		data["StepNum"] = cur + 1
		data["Total"] = len(b.Steps)
	}
	s.render(w, "build_view.html", data)
}

type navItem struct {
	Idx      int
	Title    string
	Mark     string
	Current  bool
	Unlocked bool
}

// sectionQuestion pairs a quiz question with the learner's latest saved attempt
// so the section template can restore the answer, selected choice, and verdict.
type sectionQuestion struct {
	model.Question
	Answered    bool
	Verdict     string
	VerdictText string // "verdict - feedback" (matches app.js), feedback optional
	Answer      string
	Choice      int
}

// buildSectionQuestions attaches saved progress to each question. Unanswered
// questions carry Answered=false, Choice=-1, and no verdict.
func buildSectionQuestions(questions []model.Question, progress map[string]model.QuestionProgress) []sectionQuestion {
	out := make([]sectionQuestion, 0, len(questions))
	for _, q := range questions {
		sq := sectionQuestion{Question: q, Choice: -1}
		if p, ok := progress[q.ID]; ok {
			sq.Answered = true
			sq.Verdict = p.Verdict
			sq.Answer = p.Answer
			sq.Choice = p.Choice
			sq.VerdictText = p.Verdict
			if p.Feedback != "" {
				sq.VerdictText += " - " + p.Feedback
			}
		}
		out = append(out, sq)
	}
	return out
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

type buildTestResponse struct {
	Passed   bool   `json:"passed"`
	Output   string `json:"output"`
	RunError bool   `json:"runError"`
	Complete bool   `json:"complete"`
}

func (s *Server) handleBuildTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, lang := r.PathValue("id"), r.PathValue("lang")
	b, err := s.store.GetBuild(ctx, id, lang)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cur := currentBuildStep(b)
	if cur < 0 {
		writeJSON(w, buildTestResponse{Complete: true})
		return
	}
	res, err := s.runner.Run(ctx, b.Workspace, lang)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	resp := buildTestResponse{Passed: res.Passed, Output: res.Output, RunError: res.RunError}
	if res.Passed {
		if err := build.PassStep(ctx, s.store, b, cur); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Complete = currentBuildStep(b) < 0
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleBuildSkip(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, lang := r.PathValue("id"), r.PathValue("lang")
	b, err := s.store.GetBuild(ctx, id, lang)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cur := currentBuildStep(b)
	if cur >= 0 {
		if err := build.SkipStep(ctx, s.store, b, cur); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, "/build/"+id+"/"+lang, http.StatusSeeOther)
}

func (s *Server) handleBuildHint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, lang := r.PathValue("id"), r.PathValue("lang")
	b, err := s.store.GetBuild(ctx, id, lang)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cur := currentBuildStep(b)
	if cur < 0 {
		http.Error(w, "build complete", http.StatusBadRequest)
		return
	}
	var req struct {
		Output string `json:"output"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	code := readWorkspaceText(b.Workspace)
	hint, err := build.Hint(ctx, s.inv, b.Steps[cur].Goal, code, req.Output)
	if err != nil {
		http.Error(w, "hint unavailable", http.StatusBadGateway)
		return
	}
	writeJSON(w, map[string]string{"hint": hint})
}

// readWorkspaceText concatenates UTF-8 text files under ws (path-labelled),
// capped to keep the agent prompt bounded; binary files are skipped.
func readWorkspaceText(ws string) string {
	const capBytes = 100_000
	var sb strings.Builder
	total := 0
	filepath.WalkDir(ws, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if de.IsDir() {
			switch filepath.Base(p) {
			case "target", "build", "node_modules", ".git":
				return filepath.SkipDir
			}
			return nil
		}
		b, e := os.ReadFile(p)
		if e != nil || !utf8.Valid(b) {
			return nil
		}
		rel, _ := filepath.Rel(ws, p)
		chunk := "=== " + rel + " ===\n" + string(b) + "\n"
		if total+len(chunk) > capBytes {
			return nil
		}
		sb.WriteString(chunk)
		total += len(chunk)
		return nil
	})
	return sb.String()
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	sh, err := s.store.ExportSource(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+sheet.Filename(sh.Source.Title)+"\"")
	if err := sheet.Encode(w, sh); err != nil {
		// Headers already sent; log and stop.
		s.log.Error("export encode failed", "id", id, "err", err)
	}
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Cap the upload at 32 MiB.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "bad upload", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("sheet")
	if err != nil {
		http.Error(w, "no sheet file", http.StatusBadRequest)
		return
	}
	defer file.Close()
	sh, err := sheet.Decode(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := s.store.ImportSheet(ctx, sh, s.newID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	progress, err := s.store.GetSectionProgress(ctx, sec.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	questions := buildSectionQuestions(stud.Questions, progress)
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
		"Concepts": stud.KeyConcepts, "Questions": questions, "Nav": nav,
		"Done": done, "Total": len(src.Sections),
		"HasNext": hasNext, "NextIdx": idx + 1,
		"NextUnlocked": ordered[idx] == model.StatusPassed || ordered[idx] == model.StatusSkipped,
	}
	s.render(w, "section.html", data)
}
