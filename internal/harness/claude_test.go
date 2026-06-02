package harness

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeClaudeScript is a minimal shell script that records its argv
// (one per line) to $FAKE_CLAUDE_LOG.argv, its stdin to $FAKE_CLAUDE_STDIN,
// and its invoked-binary-path to $FAKE_CLAUDE_LOG.bin.
// Exit code is controlled by $FAKE_CLAUDE_EXIT (default 0).
const fakeClaudeScript = `#!/bin/sh
for a in "$@"; do
  printf '%s\n' "$a" >> "$FAKE_CLAUDE_LOG.argv"
done
cat >> "$FAKE_CLAUDE_STDIN"
printf '%s' "$0" > "$FAKE_CLAUDE_LOG.bin"
exit "${FAKE_CLAUDE_EXIT:-0}"
`

func setupFakeClaude(t *testing.T) (binDir, logPath, stdinPath string) {
	t.Helper()
	tmp := t.TempDir()
	binDir = filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(binDir, "claude")
	if err := os.WriteFile(script, []byte(fakeClaudeScript), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath = filepath.Join(tmp, "log")
	stdinPath = filepath.Join(tmp, "stdin")
	if err := os.WriteFile(logPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stdinPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_LOG", logPath)
	t.Setenv("FAKE_CLAUDE_STDIN", stdinPath)
	return binDir, logPath, stdinPath
}

func TestClaudeCodeName(t *testing.T) {
	c := NewClaudeCode()
	if got := c.Name(); got != "claude" {
		t.Errorf("Name: got %q, want %q", got, "claude")
	}
}

func TestClaudeCodeInvokeExactArgv(t *testing.T) {
	// SPEC §6.1: argv must be exactly this set, in this order (modulo
	// the model slot). Assert it line-for-line.
	binDir, logPath, _ := setupFakeClaude(t)
	c := &ClaudeCode{BinaryPath: filepath.Join(binDir, "claude")}

	var stdout, stderr io.Writer = io.Discard, io.Discard
	_, err := c.Invoke(context.Background(), InvokeRequest{
		Mode:        ModeBuild,
		Prompt:      "hello world",
		Model:       "opus",
		ProjectRoot: "/tmp/proj",
		Stdout:      stdout,
		Stderr:      stderr,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	got, err := os.ReadFile(logPath + ".argv")
	if err != nil {
		t.Fatalf("read argv log: %v", err)
	}
	gotLines := strings.Split(strings.TrimRight(string(got), "\n"), "\n")

	wantLines := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--output-format=stream-json",
		"--include-partial-messages",
		"--model",
		"opus",
		"--verbose",
	}
	if len(gotLines) != len(wantLines) {
		t.Fatalf("argv length: got %d (%v), want %d (%v)", len(gotLines), gotLines, len(wantLines), wantLines)
	}
	for i, want := range wantLines {
		if gotLines[i] != want {
			t.Errorf("argv[%d]: got %q, want %q", i, gotLines[i], want)
		}
	}
}

func TestClaudeCodeInvokeStdinPrompt(t *testing.T) {
	binDir, _, stdinPath := setupFakeClaude(t)
	c := &ClaudeCode{BinaryPath: filepath.Join(binDir, "claude")}

	prompt := "PROMPT_HERE\nmulti-line\nstuff"
	var stdout, stderr io.Writer = io.Discard, io.Discard
	_, err := c.Invoke(context.Background(), InvokeRequest{
		Prompt: prompt,
		Model:  "sonnet",
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	got, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != prompt {
		t.Errorf("stdin: got %q, want %q", string(got), prompt)
	}
}

func TestClaudeCodeInvokeExitCodePassThrough(t *testing.T) {
	binDir, _, _ := setupFakeClaude(t)
	t.Setenv("FAKE_CLAUDE_EXIT", "7")
	c := &ClaudeCode{BinaryPath: filepath.Join(binDir, "claude")}

	var stdout, stderr io.Writer = io.Discard, io.Discard
	res, err := c.Invoke(context.Background(), InvokeRequest{
		Prompt: "x",
		Model:  "opus",
		Stdout: stdout,
		Stderr: stderr,
	})
	if err != nil {
		t.Fatalf("Invoke returned error for non-zero exit (should be in Result): %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("ExitCode: got %d, want 7", res.ExitCode)
	}
}

func TestClaudeCodeInvokeMissingBinary(t *testing.T) {
	c := &ClaudeCode{BinaryPath: "/nonexistent/claude-binary-xyz"}
	var stdout, stderr io.Writer = io.Discard, io.Discard
	_, err := c.Invoke(context.Background(), InvokeRequest{
		Prompt: "x",
		Model:  "opus",
		Stdout: stdout,
		Stderr: stderr,
	})
	if err == nil {
		t.Fatal("expected error when binary is missing")
	}
}

func TestClaudeCodeVersionMissingBinary(t *testing.T) {
	c := &ClaudeCode{BinaryPath: "/nonexistent/claude-binary-xyz"}
	_, err := c.Version()
	if err == nil {
		t.Fatal("expected error when binary is missing")
	}
}

func TestModePromptFile(t *testing.T) {
	cases := map[Mode]string{
		ModeBuild:   "PROMPT_build.md",
		ModePlan:    "PROMPT_plan.md",
		ModeReverse: "PROMPT_reverse_engineer_specs.md",
	}
	for m, want := range cases {
		if got := m.PromptFile(); got != want {
			t.Errorf("%s.PromptFile(): got %q, want %q", m, got, want)
		}
	}
}
