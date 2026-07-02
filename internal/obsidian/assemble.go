package obsidian

import (
	"context"
	"errors"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// Assemble gathers exactly what belongs in the learner's Obsidian vault:
// only questions with a pass/partial verdict, only sections holding at
// least one such question, and progress only for included questions. The
// vault "builds itself" as the learner completes questions. Propagates
// store.ErrNotFound for an unknown source.
func Assemble(ctx context.Context, st store.Store, sourceID string) (*Export, error) {
	src, err := st.GetSource(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	exp := &Export{
		Source:     src,
		Studies:    map[string]*model.SectionStudy{},
		Progress:   map[string]map[string]model.QuestionProgress{},
		ExportedAt: time.Now(),
	}
	var included []model.Section
	for _, sec := range src.Sections {
		study, err := st.GetSectionStudy(ctx, sec.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		prog, err := st.GetSectionProgress(ctx, sec.ID)
		if err != nil {
			return nil, err
		}
		var questions []model.Question
		completed := map[string]model.QuestionProgress{}
		for _, q := range study.Questions {
			p, ok := prog[q.ID]
			if !ok || p.Verdict == "fail" {
				continue
			}
			questions = append(questions, q)
			completed[q.ID] = p
		}
		if len(questions) == 0 {
			continue
		}
		study.Questions = questions
		included = append(included, sec)
		exp.Studies[sec.ID] = study
		exp.Progress[sec.ID] = completed
	}
	src.Sections = included
	return exp, nil
}
