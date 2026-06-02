// Package harness defines the Harness interface and the canonical ClaudeCode
// implementation.
//
// Per SPEC §6.5, the Harness interface is the extension point for adding
// alternative coding-agent backends (pi, amp, codex, opencode) in v0.2.
// In v0.1 only "claude" is registered; passing any other value at the CLI
// level is rejected with ExitConfigInvalid before this package is reached.
package harness

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// Mode is the loop driver mode that selects which prompt file is used.
// Maps to the three PROMPT_*.md templates bundled into the binary.
type Mode string

const (
	ModeBuild  Mode = "build"  // ralph loop, PROMPT_build.md
	ModePlan   Mode = "plan"   // ralph plan, PROMPT_plan.md
	ModeReverse Mode = "reverse" // ralph loop --reverse pre-pass, PROMPT_reverse_engineer_specs.md
)

// PromptFile returns the embedded .ralph/<filename> for this mode.
// Pre-condition: mode is one of the three known values.
func (m Mode) PromptFile() string {
	switch m {
	case ModeBuild:
		return "PROMPT_build.md"
	case ModePlan:
		return "PROMPT_plan.md"
	case ModeReverse:
		return "PROMPT_reverse_engineer_specs.md"
	}
	return ""
}

// InvokeRequest is the input to Harness.Invoke.
//
// Model is "opus" or "sonnet" (the two values Claude Code v1.0+ accepts).
// IterationCount is the 0-indexed build iteration number (0 = first pass).
// LoopCount is the value from .ralph/ralph.json loop_count, used in the
// "## Ralph State" section of the built prompt (see SPEC §6.3).
type InvokeRequest struct {
	Mode           Mode
	Prompt         string        // the full prompt text to pipe on stdin
	Model          string        // "opus" or "sonnet"
	ProjectRoot    string        // absolute path; injected as RALPH_PROJECT_ROOT
	IterationCount int
	LoopCount      int
	Stdin          io.Reader     // optional override; if nil, defaults to strings.NewReader(Prompt)
	Stdout         io.Writer     // optional override; if nil, defaults to os.Stdout
	Stderr         io.Writer     // optional override; if nil, defaults to os.Stderr
	Command        string        // optional override of "claude" binary path (for tests)
}

// Result captures what the harness invocation did. The loop driver uses
// ExitCode to decide whether to continue, abort, or fail.
type Result struct {
	ExitCode int
	// Note: a richer implementation would also parse stream-json events from
	// stdout and surface them via the events package. v0.1 keeps it simple —
	// the harness is a black box that exits with a code.
}

// Harness is the contract every coding-agent backend must implement.
//
// Name is the value matched against the --harness CLI flag.
// Version returns the harness binary's version string (for `ralph status`).
// Invoke runs one iteration: pipes the prompt to stdin, captures exit code.
// Errors from Invoke are reserved for infrastructure failures (can't find
// binary, can't create process). Non-zero exit codes from the agent itself
// are returned via Result.ExitCode, not as errors.
type Harness interface {
	Name() string
	Version() (string, error)
	Invoke(ctx context.Context, req InvokeRequest) (Result, error)
}

// ErrNotFound is returned by ClaudeCode.Version when the binary is missing.
// The CLI maps this to ExitHarnessMissing (SPEC §8 exit code 4).
var ErrNotFound = fmt.Errorf("harness binary not found on PATH")

// LookPath is a tiny wrapper around exec.LookPath so callers and tests can
// stub it without pulling in a mocking framework. Returns the absolute path
// to the binary, or ErrNotFound.
func LookPath(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return p, nil
}
