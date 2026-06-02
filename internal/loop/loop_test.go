package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
