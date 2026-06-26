package build

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Result is the outcome of running a build's tests.
type Result struct {
	Passed   bool
	Output   string
	RunError bool // the command could not run (e.g. toolchain missing), distinct from a test failure
}

// Runner runs a build's tests in a workspace.
type Runner interface {
	Run(ctx context.Context, workspace, language string) (Result, error)
}

var commands = map[string][]string{
	"rust":   {"cargo", "test"},
	"go":     {"go", "test", "./..."},
	"python": {"pytest"},
	"c":      {"make", "test"},
	"cpp":    {"make", "test"},
}

func commandFor(language string) ([]string, bool) {
	c, ok := commands[language]
	return append([]string(nil), c...), ok
}

// ExecRunner runs the per-language test command as a subprocess.
type ExecRunner struct {
	Timeout  time.Duration
	override []string // test hook: when set, used instead of the language command
}

// NewExecRunner returns an ExecRunner with a 90s timeout.
func NewExecRunner() *ExecRunner { return &ExecRunner{Timeout: 90 * time.Second} }

func (r *ExecRunner) Run(ctx context.Context, workspace, language string) (Result, error) {
	cmdParts := r.override
	if cmdParts == nil {
		var ok bool
		cmdParts, ok = commandFor(language)
		if !ok {
			return Result{RunError: true, Output: "unsupported language: " + language}, nil
		}
	}
	to := r.Timeout
	if to == 0 {
		to = 90 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()
	cmd := exec.CommandContext(cctx, cmdParts[0], cmdParts[1:]...)
	cmd.Dir = workspace
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err == nil {
		return Result{Passed: true, Output: out}, nil
	}
	// Distinguish "could not run" from "ran and failed tests".
	if errors.Is(cctx.Err(), context.DeadlineExceeded) {
		return Result{RunError: true, Output: out + "\n(test run timed out)"}, nil
	}
	if errors.Is(cctx.Err(), context.Canceled) {
		return Result{RunError: true, Output: out + "\n(test run canceled)"}, nil
	}
	var notFound *exec.Error
	if errors.As(err, &notFound) {
		return Result{RunError: true, Output: "could not run tests: " + err.Error()}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{Passed: false, Output: out}, nil
	}
	return Result{RunError: true, Output: out + "\n" + err.Error()}, nil
}

// FakeRunner is a Runner for tests.
type FakeRunner struct {
	Result Result
	Err    error
	Calls  int
}

func (f *FakeRunner) Run(_ context.Context, _, _ string) (Result, error) {
	f.Calls++
	return f.Result, f.Err
}
