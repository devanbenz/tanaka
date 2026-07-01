package obsidian

import (
	"context"
	"errors"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// Assemble gathers a source, its study packages, and the learner's progress
// into an Export. Sections without a generated study package are simply
// absent from Studies. Propagates store.ErrNotFound for an unknown source.
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
	for _, sec := range src.Sections {
		study, err := st.GetSectionStudy(ctx, sec.ID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			return nil, err
		}
		exp.Studies[sec.ID] = study
		prog, err := st.GetSectionProgress(ctx, sec.ID)
		if err != nil {
			return nil, err
		}
		if len(prog) > 0 {
			exp.Progress[sec.ID] = prog
		}
	}
	return exp, nil
}
