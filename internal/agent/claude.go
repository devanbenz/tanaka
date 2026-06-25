package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Claude invokes the `claude` CLI in headless print mode.
type Claude struct {
	binary string
}

// NewClaude returns a Claude invoker. If binary is empty, "claude" is used.
func NewClaude(binary string) *Claude {
	if binary == "" {
		binary = "claude"
	}
	return &Claude{binary: binary}
}

// Check verifies the CLI is runnable.
func (c *Claude) Check(ctx context.Context) error {
	if err := exec.CommandContext(ctx, c.binary, "--version").Run(); err != nil {
		return fmt.Errorf("claude CLI not runnable (%q): %w; is it installed and on PATH?", c.binary, err)
	}
	return nil
}

// envelope is the subset of `claude --output-format json` output we read.
type envelope struct {
	StructuredOutput json.RawMessage `json:"structured_output"`
	Result           string          `json:"result"`
}

// Invoke runs the job and returns its structured_output payload.
func (c *Claude) Invoke(ctx context.Context, job Job) (json.RawMessage, error) {
	// Note: --bare is intentionally omitted. It forces API-key auth and breaks
	// Claude subscription/OAuth. --output-format json still yields a single
	// machine-readable JSON object for the envelope decode below.
	args := []string{"-p", job.Prompt, "--output-format", "json"}
	if job.Schema != "" {
		args = append(args, "--json-schema", job.Schema)
	}
	cmd := exec.CommandContext(ctx, c.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("claude invoke: %w; stderr: %s", err, stderr.String())
	}
	var env envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		return nil, fmt.Errorf("parse claude output: %w; raw: %s", err, stdout.String())
	}
	if len(env.StructuredOutput) == 0 || string(env.StructuredOutput) == "null" {
		return nil, fmt.Errorf("claude returned no structured_output; result: %s", env.Result)
	}
	return env.StructuredOutput, nil
}
