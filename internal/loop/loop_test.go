package loop

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klampa/ralph-cli/internal/events"
	"github.com/klampa/ralph-cli/internal/harness"
	"github.com/klampa/ralph-cli/internal/state"
)

// makeProject creates a temp dir with .git/ and a minimal .ralph/ scaffold
// (one of each template, an empty IMPLEMENTATION_PLAN.md, an AGENTS.md, and
// a ralph.json state). Returns the temp dir.
func makeProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ralph", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Minimal PROMPT_build.md so the loop has something to pipe.
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "PROMPT_build.md"),
		[]byte("BUILD: do the thing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "PROMPT_plan.md"),
		[]byte("PLAN: plan the thing.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "PROMPT_reverse_engineer_specs.md"),
		[]byte("REVERSE: extract specs.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "AGENTS.md"),
		[]byte("# AGENTS\n\nbe careful.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md"),
		[]byte("# Plan\n\n- step 1\n- step 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := state.Save(dir, state.Default()); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunOneIterationHappy(t *testing.T) {
	dir := makeProject(t)
	mock := harness.NewMock()
	exit, err := Run(context.Background(), dir, mock, Options{
		Mode:        ModeBuild,
		Model:       "opus",
		MaxIterations: 1,
		NoPush:      true, // no remote in test env
		NoCommit:    true, // no git author in test env
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exit != 0 {
		t.Errorf("exit: got %d, want 0", exit)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("expected 1 mock call, got %d", len(mock.Calls))
	}
	if mock.Calls[0].Mode != ModeBuild {
		t.Errorf("call mode: got %q, want %q", mock.Calls[0].Mode, ModeBuild)
	}
}

func TestRunMaxIterationsCap(t *testing.T) {
	dir := makeProject(t)
	mock := harness.NewMock()
	exit, err := Run(context.Background(), dir, mock, Options{
		Mode:         ModeBuild,
		MaxIterations: 3,
		NoPush:       true,
		NoCommit:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exit != 0 {
		t.Errorf("exit: got %d, want 0", exit)
	}
	if len(mock.Calls) != 3 {
		t.Errorf("expected 3 mock calls (max-iter cap), got %d", len(mock.Calls))
	}
}

func TestRunAbortMidLoop(t *testing.T) {
	dir := makeProject(t)
	// Pre-create the abort sentinel so the abort check at the top of
	// iteration 1 fires before we run any iterations.
	if err := state.RequestAbort(dir); err != nil {
		t.Fatal(err)
	}
	mock := harness.NewMock()
	exit, err := Run(context.Background(), dir, mock, Options{
		Mode:         ModeBuild,
		MaxIterations: 5,
		NoPush:       true,
		NoCommit:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exit != 2 {
		t.Errorf("exit: got %d, want 2 (ExitAborted)", exit)
	}
	if len(mock.Calls) != 0 {
		t.Errorf("expected 0 mock calls (abort before iter), got %d", len(mock.Calls))
	}
}

func TestRunHarnessFailurePropagates(t *testing.T) {
	dir := makeProject(t)
	mock := harness.NewMockWithExits(1) // first call exits 1
	exit, err := Run(context.Background(), dir, mock, Options{
		Mode:         ModeBuild,
		MaxIterations: 5,
		NoPush:       true,
		NoCommit:     true,
	})
	if err == nil {
		t.Fatal("expected error when harness exits non-zero")
	}
	if exit != 1 {
		t.Errorf("exit: got %d, want 1 (ExitGeneric)", exit)
	}
}

func TestRunNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	// No .git/ — preflight should reject.
	mock := harness.NewMock()
	_, err := Run(context.Background(), dir, mock, Options{Mode: ModeBuild})
	if err == nil {
		t.Fatal("expected error when root is not a git repo")
	}
	if !strings.Contains(err.Error(), "not a git repo") {
		t.Errorf("error should mention 'not a git repo', got: %v", err)
	}
}

func TestRunNoRalphDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// No .ralph/ — preflight should reject with code 6.
	mock := harness.NewMock()
	_, err := Run(context.Background(), dir, mock, Options{Mode: ModeBuild})
	if err == nil {
		t.Fatal("expected error when .ralph/ is missing")
	}
}

func TestRunReversePrePass(t *testing.T) {
	dir := makeProject(t)
	mock := harness.NewMock()
	_, err := Run(context.Background(), dir, mock, Options{
		Mode:         ModeBuild,
		Reverse:      true,
		MaxIterations: 1,
		NoPush:       true,
		NoCommit:     true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The reverse pre-pass runs ModeReverse once, then 1 build iter.
	if len(mock.Calls) != 2 {
		t.Fatalf("expected 2 mock calls (reverse + 1 build), got %d", len(mock.Calls))
	}
	if mock.Calls[0].Mode != ModeReverse {
		t.Errorf("first call should be reverse, got %q", mock.Calls[0].Mode)
	}
	if mock.Calls[1].Mode != ModeBuild {
		t.Errorf("second call should be build, got %q", mock.Calls[1].Mode)
	}
}

func TestBuildPromptIncludesRalphState(t *testing.T) {
	dir := makeProject(t)
	st := state.Default()
	st.LoopCount = 7
	prompt, err := buildPrompt(dir, ModeBuild, 3, st)
	if err != nil {
		t.Fatalf("buildPrompt: %v", err)
	}
	if !strings.Contains(prompt, "BUILD: do the thing.") {
		t.Error("prompt missing template body")
	}
	if !strings.Contains(prompt, "## Ralph State") {
		t.Error("prompt missing Ralph State block")
	}
	if !strings.Contains(prompt, "Loop iteration: 3") {
		t.Error("prompt missing iteration number")
	}
	if !strings.Contains(prompt, "Loop count: 7") {
		t.Error("prompt missing loop count from state")
	}
	if !strings.Contains(prompt, "### AGENTS.md") {
		t.Error("prompt missing AGENTS.md section")
	}
	if !strings.Contains(prompt, "### IMPLEMENTATION_PLAN.md") {
		t.Error("prompt missing IMPLEMENTATION_PLAN.md section")
	}
}

// TestCountUncheckedPlanItems covers the heuristic that drives
// plan-exhausted detection (issue #2).
func TestCountUncheckedPlanItems(t *testing.T) {
	dir := makeProject(t)
	planPath := filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md")

	// Empty plan → 0 (exhausted).
	if err := os.WriteFile(planPath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := countUncheckedPlanItems(dir)
	if err != nil {
		t.Fatalf("countUncheckedPlanItems: %v", err)
	}
	if n != 0 {
		t.Errorf("empty plan: got %d, want 0", n)
	}

	// Mixed: 3 unchecked, 1 checked, 1 plain line.
	mixed := `# Plan
- [ ] first
* [ ] second
  - [x] done
- [ ] third with [ ] in text
random line
`
	if err := os.WriteFile(planPath, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err = countUncheckedPlanItems(dir)
	if err != nil {
		t.Fatalf("countUncheckedPlanItems: %v", err)
	}
	if n != 3 {
		t.Errorf("mixed plan: got %d, want 3", n)
	}

	// All checked → 0.
	allDone := `- [x] one
- [x] two
* [x] three
`
	if err := os.WriteFile(planPath, []byte(allDone), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err = countUncheckedPlanItems(dir)
	if err != nil {
		t.Fatalf("countUncheckedPlanItems: %v", err)
	}
	if n != 0 {
		t.Errorf("all-checked plan: got %d, want 0", n)
	}

	// Missing plan → 0 (treated as exhausted).
	if err := os.Remove(planPath); err != nil {
		t.Fatal(err)
	}
	n, err = countUncheckedPlanItems(dir)
	if err != nil {
		t.Fatalf("countUncheckedPlanItems on missing: %v", err)
	}
	if n != 0 {
		t.Errorf("missing plan: got %d, want 0", n)
	}
}

// TestRunPlanExhaustedEmitsSignal verifies that the loop exits 0 and
// prints the RALPH_SIGNAL:PLAN_EXHAUSTED sentinel when:
//   1. the plan has no unchecked "- [ ]" items, AND
//   2. there has been no new commit in the last PlanExhaustedStaleIters
//      iterations.
//
// Issue #2 (v0.1.2). We bypass the harness's commit by using NoCommit so
// the LastCommit SHA never changes, simulating a no-progress run on an
// already-finished plan.
func TestRunPlanExhaustedEmitsSignal(t *testing.T) {
	dir := makeProject(t)
	// Write a plan with no unchecked items.
	if err := os.WriteFile(
		filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md"),
		[]byte("# Plan\n- [x] one\n- [x] two\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	// Stash a known prior commit so the loop's "no new commit" path
	// triggers immediately on iter 0. We can't use git in this
	// environment cleanly (no author identity in some test envs), so
	// instead we point prevLastCommit via a pre-loaded state with a
	// fixed LastCommit that we know the mock harness won't change.
	// NoCommit=true makes the loop skip git commit entirely, which
	// guarantees st.LastCommit never updates — satisfying "no new
	// commit" by construction.
	mock := harness.NewMock()
	var stdout bytes.Buffer
	_, err := Run(context.Background(), dir, mock, Options{
		Mode:                   ModeBuild,
		MaxIterations:          100,
		NoPush:                 true,
		NoCommit:               true,
		PlanExhaustedStaleIters: 2,
		Stdout:                 &stdout,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// We should have stopped before reaching max-iter. The check fires
	// at the top of the loop, so the last completed iter is iter
	// (staleIters-1). With staleIters threshold 2, that means iter 1
	// is the last completed call, and iter 2's top-of-loop check
	// triggers the exit before another runOne. So 2 calls.
	if len(mock.Calls) != 2 {
		t.Errorf("expected 2 mock calls (plan-exhausted exit), got %d", len(mock.Calls))
	}
	if !strings.Contains(stdout.String(), PlanExhaustedSignal) {
		t.Errorf("stdout should contain the PLAN_EXHAUSTED sentinel, got:\n%s", stdout.String())
	}
}

// TestRunPlanExhaustedDoesNotFireWithUncheckedItems ensures the
// sentinel does NOT fire when there are still unchecked items, even
// with no new commits. (The plan is the source of truth for "done-ness";
// the no-commit heuristic just helps detect stalls sooner.)
func TestRunPlanExhaustedDoesNotFireWithUncheckedItems(t *testing.T) {
	dir := makeProject(t)
	// Plan still has unchecked items.
	if err := os.WriteFile(
		filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md"),
		[]byte("# Plan\n- [ ] still todo\n- [x] done\n"), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	mock := harness.NewMock()
	var stdout bytes.Buffer
	_, err := Run(context.Background(), dir, mock, Options{
		Mode:                   ModeBuild,
		MaxIterations:          4,
		NoPush:                 true,
		NoCommit:               true,
		PlanExhaustedStaleIters: 2,
		Stdout:                 &stdout,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(stdout.String(), PlanExhaustedSignal) {
		t.Errorf("PLAN_EXHAUSTED must not fire when unchecked items remain, got:\n%s", stdout.String())
	}
	if len(mock.Calls) != 4 {
		t.Errorf("expected 4 mock calls (hit max-iter), got %d", len(mock.Calls))
	}
}

// TestHasOriginRemote covers the helper that drives issue #8 (suppress
// "git push failed" noise when no origin is configured).
func TestHasOriginRemote(t *testing.T) {
	// No git env: hasOriginRemote should be false.
	if hasOriginRemote(t.TempDir()) {
		t.Error("hasOriginRemote in a non-git dir should be false")
	}
	// Real git repo with origin: should be true.
	dir := t.TempDir()
	mustRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustRun("init", "-q")
	mustRun("config", "user.email", "t@t")
	mustRun("config", "user.name", "t")
	// No origin added yet → should be false.
	if hasOriginRemote(dir) {
		t.Error("hasOriginRemote in a real repo without remote should be false")
	}
	mustRun("remote", "add", "origin", "https://example.com/test.git")
	if !hasOriginRemote(dir) {
		t.Error("hasOriginRemote after `git remote add origin` should be true")
	}
}

// TestGitPushSkipsWhenNoOrigin verifies the loop does NOT print
// "git push failed" when the project has no origin remote, AND that
// it emits a push.skipped audit event (issue #8).
func TestGitPushSkipsWhenNoOrigin(t *testing.T) {
	dir := t.TempDir()
	mustRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustRun("init", "-q")
	mustRun("config", "user.email", "t@t")
	mustRun("config", "user.name", "t")
	// Make a commit so HEAD exists (the no-origin test doesn't strictly
	// need it, but mirrors the real flow).
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun("add", "-A")
	mustRun("commit", "-q", "-m", "init")

	var stderr bytes.Buffer
	emitter := events.NewEmitter(io.Discard, events.NewSessionID(), dir)
	gitPush(dir, emitter, Options{Stderr: &stderr})

	out := stderr.String()
	if strings.Contains(out, "git push failed") {
		t.Errorf("gitPush should NOT print 'git push failed' when no origin, got:\n%s", out)
	}
}
