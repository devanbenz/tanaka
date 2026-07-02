// Package study generates study packages, grades answers, and computes gating.
package study

import (
	"context"

	"github.com/devandbenz/tanaka/internal/model"
	"github.com/devandbenz/tanaka/internal/store"
)

// ComputeUnlocked returns, for each section in order, whether it is reachable.
// Section 0 is always reachable; section i>0 is reachable iff section i-1 is
// passed or skipped, or the section itself is already passed/skipped/unlocked.
// PassAndUnlockNext marks the section passed and unlocks the following
// section (by source order) if it is currently locked.
func PassAndUnlockNext(ctx context.Context, st store.Store, sectionID string) error {
	return setAndUnlockNext(ctx, st, sectionID, model.StatusPassed)
}

// SkipAndUnlockNext marks the section skipped and unlocks the following
// section (by source order) if it is currently locked.
func SkipAndUnlockNext(ctx context.Context, st store.Store, sectionID string) error {
	return setAndUnlockNext(ctx, st, sectionID, model.StatusSkipped)
}

func setAndUnlockNext(ctx context.Context, st store.Store, sectionID, status string) error {
	sec, err := st.GetSection(ctx, sectionID)
	if err != nil {
		return err
	}
	if err := st.SetSectionStatus(ctx, sectionID, status); err != nil {
		return err
	}
	src, err := st.GetSource(ctx, sec.SourceID)
	if err != nil {
		return err
	}
	next := sec.Idx + 1
	if next < len(src.Sections) {
		statuses, err := st.GetSectionStatuses(ctx, src.ID)
		if err != nil {
			return err
		}
		nextID := src.Sections[next].ID
		if statuses[nextID] == model.StatusLocked {
			return st.SetSectionStatus(ctx, nextID, model.StatusUnlocked)
		}
	}
	return nil
}

func ComputeUnlocked(statuses []string) []bool {
	out := make([]bool, len(statuses))
	for i, status := range statuses {
		reached := status == model.StatusPassed || status == model.StatusSkipped || status == model.StatusUnlocked
		if i == 0 || reached {
			out[i] = true
			continue
		}
		prev := statuses[i-1]
		out[i] = prev == model.StatusPassed || prev == model.StatusSkipped
	}
	return out
}
