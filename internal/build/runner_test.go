package build

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestCommandFor(t *testing.T) {
	cases := map[string]string{"rust": "cargo", "go": "go", "python": "pytest", "c": "make", "cpp": "make"}
	for lang, first := range cases {
		cmd, ok := commandFor(lang)
		if !ok || cmd[0] != first {
			t.Fatalf("commandFor(%q) = %v,%v want first %q", lang, cmd, ok, first)
		}
	}
	if _, ok := commandFor("haskell"); ok {
		t.Fatal("commandFor(haskell) should be false")
	}
}

func TestExecRunnerRunErrorOnMissingBinary(t *testing.T) {
	r := &ExecRunner{Timeout: 5 * time.Second, override: []string{"definitely-not-a-real-binary-xyz"}}
	res, err := r.Run(context.Background(), t.TempDir(), "go")
	if err != nil {
		t.Fatalf("Run returned err: %v", err)
	}
	if !res.RunError {
		t.Fatalf("expected RunError for missing binary, got %+v", res)
	}
}

func TestExecRunnerPassAndFail(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	ws := t.TempDir()
	// Passing command.
	pass := &ExecRunner{Timeout: 5 * time.Second, override: []string{"sh", "-c", "echo ok; exit 0"}}
	res, err := pass.Run(context.Background(), ws, "go")
	if err != nil || !res.Passed || res.RunError {
		t.Fatalf("pass: res=%+v err=%v", res, err)
	}
	if !contains(res.Output, "ok") {
		t.Fatalf("output missing stdout: %q", res.Output)
	}
	// Failing command (non-zero exit, but ran fine).
	fail := &ExecRunner{Timeout: 5 * time.Second, override: []string{"sh", "-c", "echo boom; exit 1"}}
	res, err = fail.Run(context.Background(), ws, "go")
	if err != nil || res.Passed || res.RunError {
		t.Fatalf("fail: res=%+v err=%v", res, err)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
