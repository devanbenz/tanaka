package store

import (
	"context"
	"errors"
	"time"

	"github.com/devandbenz/tanaka/internal/model"
)

// ExportSource assembles a source, its ordered sections, and each section's
// study package into a transfer Sheet. Sections without a generated study
// package have a nil Study. Learner progress is never included.
func (s *sqliteStore) ExportSource(ctx context.Context, id string) (*model.Sheet, error) {
	src, err := s.GetSource(ctx, id)
	if err != nil {
		return nil, err
	}
	sheet := &model.Sheet{
		Format:     model.SheetFormat,
		Version:    model.SheetVersion,
		ExportedAt: time.Now().Unix(),
		Source: model.SheetSource{
			Title:  src.Title,
			Origin: src.Origin,
		},
	}
	for _, sec := range src.Sections {
		ss := model.SheetSection{Idx: sec.Idx, Title: sec.Title, Markdown: sec.Markdown}
		study, err := s.GetSectionStudy(ctx, sec.ID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err == nil {
			ss.Study = &model.SheetStudy{Summary: study.Summary, KeyConcepts: study.KeyConcepts}
			for _, q := range study.Questions {
				ss.Study.Questions = append(ss.Study.Questions, model.SheetQuestion{
					Idx: q.Idx, Kind: q.Kind, Prompt: q.Prompt, Options: q.Options,
					CorrectIndex: q.CorrectIndex, Rubric: q.Rubric, Explanation: q.Explanation,
				})
			}
		}
		sheet.Source.Sections = append(sheet.Source.Sections, ss)
	}
	return sheet, nil
}
