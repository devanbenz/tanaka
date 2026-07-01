package obsidian

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

// Export is everything needed to write one source's Obsidian folder.
// Studies and Progress are keyed by section ID; Progress's inner map by
// question ID. A section absent from Studies has no generated study package.
type Export struct {
	Source     *model.Source
	Studies    map[string]*model.SectionStudy
	Progress   map[string]map[string]model.QuestionProgress
	ExportedAt time.Time
}

// concept is a deduplicated key concept and the sections that list it.
type concept struct {
	Name     string   // first-seen sanitized casing; also the filename
	Sections []string // section note names, in source order
}

// Write renders exp into dir, creating it if needed and overwriting
// previously generated files. Idempotent for a fixed Export.
func Write(dir string, exp *Export) error {
	for _, sub := range []string{"", "sections", "questions", "concepts"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Join(dir, sub), err)
		}
	}
	hub := SourceName(exp.Source.Title)
	concepts := collectConcepts(exp)
	// Build canonical-casing lookup: lower(name) -> canonical name.
	canon := make(map[string]string, len(concepts))
	for _, c := range concepts {
		canon[strings.ToLower(c.Name)] = c.Name
	}
	if err := writeNote(dir, hub, renderHub(exp)); err != nil {
		return err
	}
	for _, sec := range exp.Source.Sections {
		study := exp.Studies[sec.ID]
		name := SectionName(sec.Idx, sec.Title)
		if err := writeNote(filepath.Join(dir, "sections"), name, renderSection(hub, sec, study, canon)); err != nil {
			return err
		}
		if study == nil {
			continue
		}
		prog := exp.Progress[sec.ID]
		for _, q := range study.Questions {
			var qp *model.QuestionProgress
			if p, ok := prog[q.ID]; ok {
				qp = &p
			}
			qname := QuestionName(sec.Idx, sec.Title, q.Idx)
			if err := writeNote(filepath.Join(dir, "questions"), qname, renderQuestion(sec, q, qp, concepts)); err != nil {
				return err
			}
		}
	}
	for _, c := range concepts {
		if err := writeNote(filepath.Join(dir, "concepts"), c.Name, renderConcept(c)); err != nil {
			return err
		}
	}
	return nil
}

func writeNote(dir, name, body string) error {
	path := filepath.Join(dir, name+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// collectConcepts deduplicates key concepts case-insensitively across
// sections (first-seen casing wins) and records which sections list each.
func collectConcepts(exp *Export) []concept {
	var out []concept
	index := map[string]int{} // lower-cased name -> position in out
	// seenSection guards against duplicate (concept, section) pairs that arise
	// when a section lists the same concept twice or in two casings.
	seenSection := map[string]bool{} // "conceptIdx:secName"
	for _, sec := range exp.Source.Sections {
		study := exp.Studies[sec.ID]
		if study == nil {
			continue
		}
		secName := SectionName(sec.Idx, sec.Title)
		for _, raw := range study.KeyConcepts {
			name := Sanitize(raw)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			i, ok := index[key]
			if !ok {
				i = len(out)
				index[key] = i
				out = append(out, concept{Name: name})
			}
			guard := fmt.Sprintf("%d:%s", i, secName)
			if !seenSection[guard] {
				seenSection[guard] = true
				out[i].Sections = append(out[i].Sections, secName)
			}
		}
	}
	return out
}

func renderHub(exp *Export) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\norigin: %q\nexported: %s\ntool: tanaka\n---\n\n",
		exp.Source.Origin, exp.ExportedAt.Format("2006-01-02"))
	fmt.Fprintf(&b, "# %s\n\n", SourceName(exp.Source.Title))
	for _, sec := range exp.Source.Sections {
		fmt.Fprintf(&b, "- [[%s]]\n", SectionName(sec.Idx, sec.Title))
	}
	return b.String()
}

func renderSection(hub string, sec model.Section, study *model.SectionStudy, canon map[string]string) string {
	var b strings.Builder
	t := Sanitize(sec.Title)
	if t == "" {
		t = "Untitled"
	}
	fmt.Fprintf(&b, "---\nsource: %q\norder: %d\n---\n\n# %s\n\n",
		"[["+hub+"]]", sec.Idx+1, t)
	if study != nil {
		fmt.Fprintf(&b, "## Summary\n\n%s\n\n", study.Summary)
		var links []string
		for _, c := range study.KeyConcepts {
			s := Sanitize(c)
			if s == "" {
				continue
			}
			if canonical, ok := canon[strings.ToLower(s)]; ok {
				s = canonical
			}
			links = append(links, "- [["+s+"]]")
		}
		if len(links) > 0 {
			fmt.Fprintf(&b, "## Key concepts\n\n%s\n\n", strings.Join(links, "\n"))
		}
	}
	fmt.Fprintf(&b, "## Content\n\n%s\n", sec.Markdown)
	if study != nil && len(study.Questions) > 0 {
		b.WriteString("\n## Questions\n\n")
		for _, q := range study.Questions {
			fmt.Fprintf(&b, "- [[%s]]\n", QuestionName(sec.Idx, sec.Title, q.Idx))
		}
	}
	return b.String()
}

// calloutLines prefixes every line of s with "> " so multi-line text stays
// inside the callout block.
func calloutLines(b *strings.Builder, s string) {
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		fmt.Fprintf(b, "> %s\n", line)
	}
}

func renderQuestion(sec model.Section, q model.Question, prog *model.QuestionProgress, concepts []concept) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\nsection: %q\nkind: %s\n", "[["+SectionName(sec.Idx, sec.Title)+"]]", q.Kind)
	if prog != nil {
		fmt.Fprintf(&b, "verdict: %s\n", prog.Verdict)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# Q%d\n\n%s\n\n", q.Idx+1, q.Prompt)

	if q.Kind == model.KindMCQ && len(q.Options) > 0 {
		b.WriteString("**Options:**\n\n")
		for i, opt := range q.Options {
			fmt.Fprintf(&b, "%d. %s\n", i+1, opt)
		}
		b.WriteString("\n")
	}

	var answer strings.Builder
	if q.Kind == model.KindMCQ && q.CorrectIndex >= 0 && q.CorrectIndex < len(q.Options) {
		calloutLines(&answer, "**Correct:** "+q.Options[q.CorrectIndex])
	}
	if q.Explanation != "" {
		calloutLines(&answer, q.Explanation)
	}
	if q.Kind == model.KindFree && q.Rubric != "" {
		calloutLines(&answer, "**Rubric:** "+q.Rubric)
	}
	if answer.Len() > 0 {
		b.WriteString("> [!success]- Answer\n")
		b.WriteString(answer.String())
		b.WriteString("\n")
	}

	if prog != nil {
		b.WriteString("## My attempt\n\n")
		switch {
		case q.Kind == model.KindMCQ && prog.Choice >= 0 && prog.Choice < len(q.Options):
			fmt.Fprintf(&b, "- **Answer:** %s\n", q.Options[prog.Choice])
		case q.Kind == model.KindFree && prog.Answer != "":
			fmt.Fprintf(&b, "- **Answer:** %s\n", prog.Answer)
		}
		fmt.Fprintf(&b, "- **Verdict:** %s\n", prog.Verdict)
		if prog.Feedback != "" {
			fmt.Fprintf(&b, "- **Feedback:** %s\n", prog.Feedback)
		}
		b.WriteString("\n")
	}

	hay := strings.ToLower(q.Prompt + " " + q.Explanation + " " + q.Rubric)
	var rel []string
	for _, c := range concepts {
		if strings.Contains(hay, strings.ToLower(c.Name)) {
			rel = append(rel, "- [["+c.Name+"]]")
		}
	}
	if len(rel) > 0 {
		fmt.Fprintf(&b, "## Related concepts\n\n%s\n", strings.Join(rel, "\n"))
	}
	return b.String()
}

func renderConcept(c concept) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntool: tanaka\n---\n\n# %s\n\nAppears in:\n\n", c.Name)
	for _, s := range c.Sections {
		fmt.Fprintf(&b, "- [[%s]]\n", s)
	}
	return b.String()
}
