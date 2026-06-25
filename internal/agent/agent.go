// Package agent is the single boundary for invoking the coding agent.
// No other package may exec `claude` directly.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Job is one agent request. Large or binary content must travel via Stdin (or be
// read by the agent itself via a tool) — never embedded in Prompt, because Prompt
// becomes a command-line argument and argv cannot contain NUL bytes or huge data.
type Job struct {
	Prompt       string
	Schema       string
	Stdin        []byte   // optional: piped to the agent's stdin
	AllowedTools []string // optional: tools the agent may use (e.g. Read, WebFetch)
}

// Invoker runs a Job and returns the structured-output object as raw JSON.
type Invoker interface {
	Check(ctx context.Context) error
	Invoke(ctx context.Context, job Job) (json.RawMessage, error)
}

// Fake is an in-memory Invoker for tests. It matches a Job by finding the first
// Responses key that is a substring of job.Prompt.
type Fake struct {
	Responses map[string]json.RawMessage
	Err       error
	CheckErr  error
	Calls     []Job
}

// Check returns CheckErr (nil by default, simulating a healthy CLI).
func (f *Fake) Check(_ context.Context) error { return f.CheckErr }

// Invoke records the call and returns the matching canned response.
func (f *Fake) Invoke(_ context.Context, job Job) (json.RawMessage, error) {
	f.Calls = append(f.Calls, job)
	if f.Err != nil {
		return nil, f.Err
	}
	for key, resp := range f.Responses {
		if strings.Contains(job.Prompt, key) {
			return resp, nil
		}
	}
	return nil, fmt.Errorf("fake: no response matching prompt %q", job.Prompt)
}
