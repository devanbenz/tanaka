package study

import "github.com/devandbenz/tanaka/internal/model"

// OrderedStatuses projects the status map onto the source's section order,
// defaulting missing entries to locked.
func OrderedStatuses(src *model.Source, statuses map[string]string) []string {
	out := make([]string, len(src.Sections))
	for i, sec := range src.Sections {
		if st, ok := statuses[sec.ID]; ok {
			out[i] = st
		} else {
			out[i] = model.StatusLocked
		}
	}
	return out
}

// CurrentSectionIdx returns the index of the first section that is not passed or
// skipped. If all are done, it returns the last index. Empty input returns 0.
func CurrentSectionIdx(statuses []string) int {
	for i, s := range statuses {
		if s != model.StatusPassed && s != model.StatusSkipped {
			return i
		}
	}
	if len(statuses) == 0 {
		return 0
	}
	return len(statuses) - 1
}
