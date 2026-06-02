package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// claudeArgv is the FIXED argv for invoking Claude Code from ralph-cli.
//
// Per SPEC §6.1, any change to these flags is a breaking change to the
// contract. Keep this in lock-step with SPEC §6.1.
//
// Order does not matter to Claude Code, but we keep it stable so tests can
// assert exact byte-for-byte argv matching.
var claudeArgv = []string{
	"claude",
	"-p",                                  // headless
	"--dangerously-skip-permissions",      // YOLO mode (sandbox required)
	"--output-format=stream-json",         // structured, parseable
	"--include-partial-messages",          // live streaming (for --json)
	"--model", "__MODEL__",                // placeholder; replaced at Invoke time
	"--verbose",                           // detailed execution log
}

// ClaudeCode is the Harness implementation for Anthropic's Claude Code CLI.
// See SPEC §6 for the full contract.
type ClaudeCode struct {
	// BinaryPath overrides the "claude" binary location. Empty means
	// resolve via exec.LookPath at Invoke time. Used by tests to point
	// at a fake harness script.
	BinaryPath string
}

// NewClaudeCode returns a ClaudeCode harness that resolves the "claude"
// binary on PATH at Invoke time.
func NewClaudeCode() *ClaudeCode {
	return &ClaudeCode{}
}

// Name implements Harness.
func (c *ClaudeCode) Name() string { return "claude" }

// Version runs `claude --version` and returns the stdout.
//
// Returns ErrNotFound (wrapped) if the binary is not on PATH.
func (c *ClaudeCode) Version() (string, error) {
	bin, err := c.binaryPath()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		return "", fmt.Errorf("claude --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Invoke runs one Claude Code iteration with the exact argv and env
// pinned in SPEC §6.1 and §6.2.
//
// The prompt is piped on stdin (per §6.3). stdout/stderr are passed
// through to the caller's writers (or os.Stdout/Stderr by default).
//
// Exit code 0 from claude means success. Non-zero is returned via
// Result.ExitCode — Invoke itself only returns errors for infrastructure
// failures (binary missing, can't start process).
func (c *ClaudeCode) Invoke(ctx context.Context, req InvokeRequest) (Result, error) {
	bin, err := c.binaryPath()
	if err != nil {
		return Result{}, err
	}

	argv := buildArgv(req.Model)

	stdin := req.Stdin
	if stdin == nil {
		stdin = strings.NewReader(req.Prompt)
	}
	stdout := req.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	stderr := req.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	cmd := exec.CommandContext(ctx, bin, argv[1:]...) // argv[0] is the binary itself
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	// Per SPEC §6.2: inject CLAUDE_CODE_ENTRYPOINT and RALPH_PROJECT_ROOT;
	// inherit everything else.
	cmd.Env = append(os.Environ(),
		"CLAUDE_CODE_ENTRYPOINT=ralph-cli",
		fmt.Sprintf("RALPH_PROJECT_ROOT=%s", req.ProjectRoot),
	)

	runErr := cmd.Run()
	exit := 0
	if runErr != nil {
		// exec.ExitError carries the exit code; any other error is a real failure.
		if ee, ok := runErr.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			return Result{}, fmt.Errorf("claude: %w", runErr)
		}
	}
	return Result{ExitCode: exit}, nil
}

// binaryPath returns c.BinaryPath if set, otherwise exec.LookPath("claude").
func (c *ClaudeCode) binaryPath() (string, error) {
	if c.BinaryPath != "" {
		if _, err := os.Stat(c.BinaryPath); err != nil {
			return "", fmt.Errorf("%w: %s", ErrNotFound, c.BinaryPath)
		}
		return c.BinaryPath, nil
	}
	return LookPath("claude")
}

// buildArgv returns the exact argv for a single Claude Code invocation,
// with the model substituted into the --model flag's argument slot.
func buildArgv(model string) []string {
	out := make([]string, len(claudeArgv))
	copy(out, claudeArgv)
	for i, a := range out {
		if a == "__MODEL__" {
			out[i] = model
		}
	}
	return out
}

// ArgvForTest exposes the un-keyed claudeArgv (with __MODEL__ placeholder)
// for tests that want to assert the exact pin from SPEC §6.1.
func ArgvForTest() []string {
	out := make([]string, len(claudeArgv))
	copy(out, claudeArgv)
	return out
}

// BufferedString returns a string view of a Reader's contents, used by
// tests to capture what was piped to stdin.
func BufferedString(r io.Reader) string {
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}
