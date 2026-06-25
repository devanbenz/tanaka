package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFrameLineContainsParts(t *testing.T) {
	got := frameLine("┌(・o・)┘", "structuring", 3*time.Second)
	for _, want := range []string{"┌(・o・)┘", "structuring", "3s"} {
		if !strings.Contains(got, want) {
			t.Fatalf("frameLine = %q, missing %q", got, want)
		}
	}
	if !strings.HasPrefix(got, "\r") {
		t.Fatalf("frameLine should start with carriage return, got %q", got)
	}
}

func TestWorkFramesNonEmpty(t *testing.T) {
	if len(workFrames) == 0 {
		t.Fatal("workFrames must not be empty")
	}
	for i, f := range workFrames {
		if f == "" {
			t.Fatalf("workFrames[%d] is empty", i)
		}
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
