package ui

import (
	"bytes"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

func TestFrameLineContainsParts(t *testing.T) {
	got := frameLine("┌(・o・)┘", "structuring", 3*time.Second, 0)
	for _, want := range []string{"┌(・o・)┘", "structuring", "3s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("frameLine = %q, missing %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "\r") {
		t.Fatalf("frameLine should start with carriage return, got %q", got)
	}
}

// visible strips the control pieces of a frame line, leaving what occupies
// columns on screen.
func visible(line string) string {
	line = strings.TrimPrefix(line, "\r")
	return strings.TrimSuffix(line, "\x1b[K")
}

// A message longer than the terminal is truncated so the frame line never
// wraps: wrapping breaks \r-based in-place animation and scrolls a new line
// per tick. Columns must be measured as display width, not runes — the
// kaomoji contain East Asian wide characters like ・ that occupy two columns.
func TestFrameLineTruncatesToTerminalWidth(t *testing.T) {
	long := "reading & structuring https://" + strings.Repeat("x", 200)
	for _, set := range kaomojiSet {
		for _, frame := range set {
			got := frameLine(frame, long, 3*time.Second, 80)
			vis := visible(got)
			if n := runewidth.StringWidth(vis); n >= 80 {
				t.Fatalf("frame %q line occupies %d columns, want < 80: %q", frame, n, vis)
			}
		}
	}
	// The kaomoji and the elapsed time must survive truncation.
	vis := visible(frameLine("(・_・)", long, 3*time.Second, 80))
	for _, want := range []string{"(・_・)", "(3s)", "…"} {
		if !strings.Contains(vis, want) {
			t.Fatalf("truncated line missing %q: %q", want, vis)
		}
	}
}

// Width 0 means "unknown": no truncation.
func TestFrameLineUnknownWidthDoesNotTruncate(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := frameLine("(・_・)", long, 0, 0)
	if !strings.Contains(got, long) {
		t.Fatalf("unknown width should not truncate, got %q", got)
	}
}

// A short message is left alone even when a width is known.
func TestFrameLineShortMessageUntouched(t *testing.T) {
	got := frameLine("(・_・)", "reading paper.pdf", 1*time.Second, 80)
	if !strings.Contains(got, "reading paper.pdf") || strings.Contains(got, "…") {
		t.Fatalf("short message should not be truncated, got %q", got)
	}
}

// Each frame clears to end of line so a shorter frame leaves no residue from
// a longer previous one.
func TestFrameLineClearsToEndOfLine(t *testing.T) {
	got := frameLine("(・_・)", "reading", time.Second, 80)
	if !strings.HasSuffix(got, "\x1b[K") {
		t.Fatalf("frame line should end with clear-to-EOL, got %q", got)
	}
}

func TestSpinnerNonTTYWritesPlainPhaseLines(t *testing.T) {
	var buf bytes.Buffer // not a *os.File, so not a TTY
	s := NewSpinner(&buf, "reading & structuring paper.pdf")
	s.Start()
	s.Stop("structured \"My Paper\"")
	out := buf.String()
	for _, want := range []string{"reading & structuring paper.pdf", "structured \"My Paper\"", doneFace} {
		if !strings.Contains(out, want) {
			t.Fatalf("output = %q, missing %q", out, want)
		}
	}
	// Non-TTY output must not animate with carriage returns.
	if strings.Contains(out, "\r") {
		t.Fatalf("non-TTY output should not contain carriage returns, got %q", out)
	}
}

func TestKaomojiSetNonEmpty(t *testing.T) {
	if len(kaomojiSet) == 0 {
		t.Fatal("kaomojiSet must not be empty")
	}
	for i, k := range kaomojiSet {
		if len(k) == 0 {
			t.Fatalf("kaomojiSet[%d] has no frames", i)
		}
		for j, f := range k {
			if f == "" {
				t.Fatalf("kaomojiSet[%d][%d] is empty", i, j)
			}
		}
	}
}

func TestNextKaomoji(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	// With >1 entries, never returns the current index.
	for i := 0; i < 100; i++ {
		got := nextKaomoji(2, 5, r)
		if got == 2 || got < 0 || got >= 5 {
			t.Fatalf("nextKaomoji returned %d", got)
		}
	}
	// With a single entry, returns 0.
	if got := nextKaomoji(0, 1, r); got != 0 {
		t.Fatalf("nextKaomoji(0,1) = %d, want 0", got)
	}
}

func TestSpinnerFailUsesFailFace(t *testing.T) {
	var buf bytes.Buffer
	s := NewSpinner(&buf, "reading")
	s.Start()
	s.Fail("ingest failed")
	out := buf.String()
	if !strings.Contains(out, failFace) || !strings.Contains(out, "ingest failed") {
		t.Fatalf("output = %q, want fail face and message", out)
	}
}
