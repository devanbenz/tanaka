// Package study generates study packages, grades answers, and computes gating.
package study

import "github.com/devandbenz/tanaka/internal/model"

// ComputeUnlocked returns, for each section in order, whether it is reachable.
// Section 0 is always reachable; section i>0 is reachable iff section i-1 is
// passed or skipped, or the section itself is already passed/skipped/unlocked.
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
