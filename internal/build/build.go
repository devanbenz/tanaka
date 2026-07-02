// Package build generates implementation exercises (build plans), grades
// progress by running tests, and provides hints.
package build

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// SafeRelPath rejects paths that are empty, absolute, or escape the
// workspace. Agent-generated paths are slash-separated and relative, so
// validation is uniform across platforms rather than deferring to the host
// OS: filepath.IsAbs alone would accept rooted ("/etc/passwd"),
// drive-relative ("C:evil"), and backslash ("..\escape") paths on Windows.
func SafeRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("empty path")
	}
	if strings.ContainsRune(p, '\\') {
		return fmt.Errorf("backslash in path not allowed: %s", p)
	}
	if len(p) >= 2 && p[1] == ':' {
		return fmt.Errorf("drive path not allowed: %s", p)
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("absolute path not allowed: %s", p)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path escapes workspace: %s", p)
	}
	return nil
}
