// Package build generates implementation exercises (build plans), grades
// progress by running tests, and provides hints.
package build

import (
	"fmt"
	"path/filepath"
	"strings"
)

// SafeRelPath rejects paths that are empty, absolute, or escape the workspace.
func SafeRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if filepath.IsAbs(p) {
		return fmt.Errorf("absolute path not allowed: %s", p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace: %s", p)
	}
	return nil
}
