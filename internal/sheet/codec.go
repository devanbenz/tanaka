// Package sheet encodes and decodes Tanaka question sheets: gzipped JSON with a
// versioned envelope, shared by the CLI and web import/export paths.
package sheet

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/devandbenz/tanaka/internal/model"
)

// Encode writes s as gzipped JSON. Format and Version are set to the current
// envelope values in a local copy; the caller's struct is not modified.
func Encode(w io.Writer, s *model.Sheet) error {
	out := *s
	out.Format = model.SheetFormat
	out.Version = model.SheetVersion
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetIndent("", "  ")
	if err := enc.Encode(&out); err != nil {
		zw.Close()
		return fmt.Errorf("encode sheet: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish gzip: %w", err)
	}
	return nil
}

// Decode reads a gzipped-JSON sheet and validates the envelope. It returns a
// descriptive error for corrupt gzip, invalid JSON, an unknown format, or an
// unsupported version.
func Decode(r io.Reader) (*model.Sheet, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("read sheet: not a valid gzip file: %w", err)
	}
	defer zr.Close()
	var s model.Sheet
	if err := json.NewDecoder(zr).Decode(&s); err != nil {
		return nil, fmt.Errorf("read sheet: invalid contents: %w", err)
	}
	if s.Format != model.SheetFormat {
		return nil, fmt.Errorf("not a Tanaka sheet (format %q)", s.Format)
	}
	if s.Version != model.SheetVersion {
		return nil, fmt.Errorf("unsupported sheet version %d (this tanaka supports %d)", s.Version, model.SheetVersion)
	}
	return &s, nil
}

// Filename returns a safe download filename for a sheet with the given source
// title: a lowercase hyphen-slug plus ".tanaka" (falls back to "sheet.tanaka").
func Filename(title string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "sheet"
	}
	return slug + ".tanaka"
}
