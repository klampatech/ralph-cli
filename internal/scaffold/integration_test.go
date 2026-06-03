//go:build integration

// Integration tests for ralph-cli. Run with: go test -tags=integration ./...
//
// These tests build the actual ralph binary and run it against a temp
// git repo + a fake `claude` shell script on PATH. They verify the full
// CLI behavior end-to-end without needing a real Claude Code install.

package scaffold_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ralphBin returns the path to the freshly-built ralph binary. The
// integration test setup compiles it into t.TempDir() once per test.
func ralphBin(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "ralph")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/ralph")
	cmd.Dir = repoRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// tests are at the package root; walk up until we find go.mod
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find repo root from %s", wd)
		}
		dir = parent
	}
}

// writeFakeClaude installs a fake `claude` shell script on PATH that
// records its argv and stdin, then returns 0. Returns the path to the
// log file for assertions.
func writeFakeClaude(t *testing.T) (logPath, stdinPath string) {
	t.Helper()
	binDir := t.TempDir()
	logPath = filepath.Join(binDir, "argv.log")
	stdinPath = filepath.Join(binDir, "stdin.log")
	t.Setenv("RALPH_FAKE_LOG", logPath)
	t.Setenv("RALPH_FAKE_STDIN", stdinPath)
	script := filepath.Join(binDir, "claude")
	scriptBody := `#!/bin/sh
for a in "$@"; do
  printf '%s\n' "$a" >> "$RALPH_FAKE_LOG"
done
cat >> "$RALPH_FAKE_STDIN"
exit "${RALPH_FAKE_EXIT:-0}"
`
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}
	// Prepend binDir to PATH.
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)
	return logPath, stdinPath
}

func TestIntegrationInit(t *testing.T) {
	bin := ralphBin(t)
	// Create a temp git repo.
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))

	cmd := exec.Command(bin, "init", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ralph init: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Created .ralph/") {
		t.Errorf("expected 'Created .ralph/' in output, got:\n%s", out)
	}
	// Verify .ralph/ + 6 templates + ralph.json + sessions/ all present.
	for _, want := range []string{
		".ralph/PROMPT_build.md",
		".ralph/PROMPT_plan.md",
		".ralph/PROMPT_reverse_engineer_specs.md",
		".ralph/AGENTS.md",
		".ralph/IMPLEMENTATION_PLAN.md",
		".ralph/loop.sh",
		".ralph/ralph.json",
		".ralph/sessions",
		".gitignore",
	} {
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("expected %q after init, got: %v", want, err)
		}
	}
}

func TestIntegrationStatusJSON(t *testing.T) {
	bin := ralphBin(t)
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))
	runRalph(t, bin, "init", dir)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "status", "--json", dir)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ralph status --json: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	// Raw state fields (v0.1.1 contract).
	for _, want := range []string{`"schema_version": 1`, `"harness": "claude"`, `"loop_count": 0`} {
		if !strings.Contains(out, want) {
			t.Errorf("status --json missing %q in:\n%s", want, out)
		}
	}
	// v0.1.2 derived fields (issue #7). last_commit_at is conditional
	// (only present when there's a commit), so we don't assert on it.
	for _, want := range []string{
		`"current_task"`,
		`"next_task"`,
		`"progress_pct"`,
		`"iterations_remaining"`,
		`"plan_exhausted"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status --json (v0.1.2) missing derived field %q in:\n%s", want, out)
		}
	}
}

// TestIntegrationStatusExplicitPathMissing is a smoke test for issue #6:
// `ralph status <path>` must error out cleanly when <path> has no
// .ralph/, instead of silently using a parent .ralph/.
func TestIntegrationStatusExplicitPathMissing(t *testing.T) {
	bin := ralphBin(t)
	parent := t.TempDir()
	mkdir(t, filepath.Join(parent, ".git"))
	runRalph(t, bin, "init", parent)
	// Make a child dir with NO .ralph/ inside.
	child := filepath.Join(parent, "no-ralph-child")
	mkdir(t, child)
	// Status with explicit child path → must error (exit 6).
	cmd := exec.Command(bin, "status", child)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("ralph status %s with no .ralph/ should error; got: %s", child, out)
	}
	if !strings.Contains(string(out), "no .ralph/") {
		t.Errorf("error should mention 'no .ralph/', got: %s", out)
	}
}

func TestIntegrationPlanInvokesClaude(t *testing.T) {
	bin := ralphBin(t)
	logPath, stdinPath := writeFakeClaude(t)
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))
	runRalph(t, bin, "init", dir)

	// Clear the abort sentinel that ralph init may have set in a prior test.
	_ = os.Remove(filepath.Join(dir, ".ralph", "ABORT_REQUESTED"))

	// ralph plan --max-iterations 1 --no-push
	cmd := exec.Command(bin, "plan", "--max-iterations", "1", "--no-push", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ralph plan: %v\n%s", err, out)
	}

	// Verify claude was invoked with the expected argv (modulo --version
	// which is also called by preflight). The SPEC §6.1 argv appears
	// verbatim, in order, somewhere in the log.
	argv, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	argvLines := strings.Split(strings.TrimRight(string(argv), "\n"), "\n")
	wantArgv := []string{
		"-p",
		"--dangerously-skip-permissions",
		"--output-format=stream-json",
		"--include-partial-messages",
		"--model",
		"opus",
		"--verbose",
	}
	// Find the plan invocation in the log: it's the run that includes
	// "-p" as the first non-version flag.
	planStart := -1
	for i, line := range argvLines {
		if line == "-p" {
			planStart = i
			break
		}
	}
	if planStart < 0 {
		t.Fatalf("did not find plan invocation in argv log:\n%s", argv)
	}
	if planStart+len(wantArgv) > len(argvLines) {
		t.Fatalf("plan argv truncated: start=%d, log=%d lines", planStart, len(argvLines))
	}
	for i, want := range wantArgv {
		if got := argvLines[planStart+i]; got != want {
			t.Errorf("argv[%d] (from plan start): got %q, want %q", planStart+i, got, want)
		}
	}

	// Verify stdin contains the PROMPT_plan.md content + Ralph State.
	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stdin), "## Ralph State") {
		t.Error("stdin missing '## Ralph State' section")
	}
	if !strings.Contains(string(stdin), "Loop iteration: 0") {
		t.Error("stdin missing 'Loop iteration: 0'")
	}
}

func TestIntegrationAbortSentinel(t *testing.T) {
	bin := ralphBin(t)
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))
	runRalph(t, bin, "init", dir)

	cmd := exec.Command(bin, "abort", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ralph abort: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ralph", "ABORT_REQUESTED")); err != nil {
		t.Errorf("ABORT_REQUESTED not created: %v", err)
	}

	// Now plan should exit 2 because sentinel is present.
	cmd = exec.Command(bin, "plan", "--max-iterations", "1", "--no-push", dir)
	cmd.Env = append(os.Environ(), "PATH="+os.Getenv("PATH"))
	out, _ := cmd.CombinedOutput()
	_ = out
	if cmd.ProcessState.ExitCode() != 2 {
		t.Errorf("plan with abort sentinel: got exit %d, want 2 (stderr+stdout: %s)",
			cmd.ProcessState.ExitCode(), out)
	}
}

func TestIntegrationInitRejectsNonGit(t *testing.T) {
	bin := ralphBin(t)
	dir := t.TempDir() // no .git/
	cmd := exec.Command(bin, "init", dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("ralph init in non-git should fail")
	}
	if code := cmd.ProcessState.ExitCode(); code != 5 {
		t.Errorf("exit code: got %d, want 5 (ExitNotInGit)", code)
	}
}

func TestIntegrationInitRejectsBadHarness(t *testing.T) {
	bin := ralphBin(t)
	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, ".git"))
	cmd := exec.Command(bin, "init", "--harness", "pi", dir)
	if err := cmd.Run(); err == nil {
		t.Fatal("ralph init --harness pi should fail")
	}
	if code := cmd.ProcessState.ExitCode(); code != 6 {
		t.Errorf("exit code: got %d, want 6 (ExitConfigInvalid)", code)
	}
}

// helpers
func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func runRalph(t *testing.T, bin string, args ...string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ralph %v: %v\n%s", args, err, out)
	}
}
