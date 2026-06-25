// Package study generates study packages, grades answers, and computes gating.
package study

import "github.com/devandbenz/tanaka/internal/model"

// ComputeUnlocked returns, for each section in order, whether it is reachable.
// Section 0 is always reachable; section i>0 is reachable iff section i-1 is
// passed or skipped.
func ComputeUnlocked(statuses []string) []bool {
	out := make([]bool, len(statuses))
	for i := range statuses {
		if i == 0 {
			out[i] = true
			continue
		}
		prev := statuses[i-1]
		out[i] = prev == model.StatusPassed || prev == model.StatusSkipped
	}
	return out
}
