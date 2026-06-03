package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klampa/ralph-cli/internal/state"
)

// makeGitRepo creates a temp dir with a .git/ subdirectory.
// Returns the temp dir (caller should defer cleanup via t.TempDir instead).
func makeGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestScaffoldFreshRepo(t *testing.T) {
	dir := makeGitRepo(t)

	res, err := Scaffold(dir, Options{RalphVersion: "0.1.0"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// .ralph/ should exist.
	if _, err := os.Stat(filepath.Join(dir, ".ralph")); err != nil {
		t.Errorf(".ralph/ missing: %v", err)
	}

	// All 6 templates should be present and non-empty.
	for _, bf := range BundledFiles {
		dst := filepath.Join(dir, ".ralph", bf.Dest)
		info, err := os.Stat(dst)
		if err != nil {
			t.Errorf("template %q missing: %v", bf.Dest, err)
			continue
		}
		if info.Size() == 0 {
			t.Errorf("template %q is empty", bf.Dest)
		}
		// Check executable bit on loop.sh.
		if bf.Dest == "loop.sh" {
			if info.Mode().Perm()&0o111 == 0 {
				t.Errorf("loop.sh is not executable (mode %o)", info.Mode().Perm())
			}
		}
	}

	// ralph.json should be present and parseable.
	st, err := state.Load(dir)
	if err != nil {
		t.Fatalf("state.Load: %v", err)
	}
	if st.RalphVersion != "0.1.0" {
		t.Errorf("RalphVersion: got %q, want %q", st.RalphVersion, "0.1.0")
	}
	if st.SchemaVersion != 1 {
		t.Errorf("SchemaVersion: got %d, want 1", st.SchemaVersion)
	}
	if st.Harness != "claude" {
		t.Errorf("Harness: got %q, want %q", st.Harness, "claude")
	}
	if st.Config.MaxIterations != 50 {
		t.Errorf("Config.MaxIterations: got %d, want 50", st.Config.MaxIterations)
	}

	// sessions/ should be empty.
	sessDir := filepath.Join(dir, ".ralph", "sessions")
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		t.Errorf("sessions/: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("sessions/ should be empty, got %d entries", len(entries))
	}

	// .gitignore should contain .ralph/.
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !contains(string(gi), ".ralph/") {
		t.Errorf(".gitignore should contain .ralph/, got:\n%s", string(gi))
	}

	// Result struct sanity.
	if res.StateFilePath == "" {
		t.Error("Result.StateFilePath empty")
	}
	if len(res.Created) < 7 { // .ralph + 6 templates + ralph.json + sessions
		t.Errorf("expected at least 7 created entries, got %d", len(res.Created))
	}
}

func TestScaffoldNoClobberWithoutForce(t *testing.T) {
	dir := makeGitRepo(t)

	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	// Second scaffold without --force should error.
	_, err := Scaffold(dir, Options{})
	if err == nil {
		t.Fatal("expected error on second scaffold without --force")
	}
}

func TestScaffoldForceOverwrites(t *testing.T) {
	dir := makeGitRepo(t)

	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	// Modify ralph.json directly.
	st, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	st.LoopCount = 99
	st.LastSessionID = "ses_xyz"
	if err := state.Save(dir, st); err != nil {
		t.Fatal(err)
	}

	// Re-scaffold with --force but NOT --reset-state. ralph.json must be preserved.
	if _, err := Scaffold(dir, Options{Force: true}); err != nil {
		t.Fatalf("force scaffold: %v", err)
	}
	st2, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st2.LoopCount != 99 {
		t.Errorf("LoopCount: got %d, want 99 (force w/o --reset-state should preserve)", st2.LoopCount)
	}
	if st2.LastSessionID != "ses_xyz" {
		t.Errorf("LastSessionID: got %q, want preserved", st2.LastSessionID)
	}

	// Now with --reset-state too — ralph.json should go back to defaults.
	if _, err := Scaffold(dir, Options{Force: true, ResetState: true}); err != nil {
		t.Fatalf("force+reset scaffold: %v", err)
	}
	st3, err := state.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st3.LoopCount != 0 {
		t.Errorf("LoopCount after --reset-state: got %d, want 0", st3.LoopCount)
	}
}

func TestScaffoldNoTemplates(t *testing.T) {
	dir := makeGitRepo(t)
	res, err := Scaffold(dir, Options{NoTemplates: true})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if len(res.Skipped) != 6 {
		t.Errorf("expected 6 skipped files, got %d", len(res.Skipped))
	}
	// Templates should NOT exist.
	for _, bf := range BundledFiles {
		if _, err := os.Stat(filepath.Join(dir, ".ralph", bf.Dest)); err == nil {
			t.Errorf("template %q should be absent with --no-templates", bf.Dest)
		}
	}
	// But ralph.json should still exist.
	if _, err := state.Load(dir); err != nil {
		t.Errorf("ralph.json should exist: %v", err)
	}
}

func TestScaffoldNoGitignore(t *testing.T) {
	dir := makeGitRepo(t)
	if _, err := Scaffold(dir, Options{NoGitignore: true}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Errorf(".gitignore should not exist with --no-gitignore, got err=%v", err)
	}
}

func TestScaffoldGitignoreIdempotent(t *testing.T) {
	dir := makeGitRepo(t)
	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("first: %v", err)
	}
	gi1, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(dir, Options{Force: true}); err != nil {
		t.Fatalf("second: %v", err)
	}
	gi2, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gi1) != string(gi2) {
		t.Errorf(".gitignore changed between scaffolds:\n--first--\n%s\n--second--\n%s", string(gi1), string(gi2))
	}
}

func TestScaffoldNotADirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a *file* where .ralph/ would go — actually, let's point root at a file.
	root := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(root, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Scaffold(root, Options{})
	if err == nil {
		t.Fatal("expected error scaffolding under a non-directory root")
	}
}

func TestScaffoldPreservesExistingGitignoreEntry(t *testing.T) {
	dir := makeGitRepo(t)
	// Pre-populate .gitignore with .ralph/ entry.
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"),
		[]byte("node_modules\n.ralph/\nbuild/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, line := range splitLines(string(gi)) {
		if trimWhitespace(line) == ".ralph/" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 .ralph/ line, got %d:\n%s", count, string(gi))
	}
}

func TestScaffoldUntracksPreviouslyTrackedRalph(t *testing.T) {
	dir := makeGitRepo(t)
	// Init a real git repo (not just .git/ as a dir), commit a .ralph/ file,
	// then run Scaffold — the file should end up untracked in the index.
	run := func(name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
		return string(out)
	}
	run("git", "init", "-q")
	run("git", "config", "user.email", "t@t")
	run("git", "config", "user.name", "test")
	// First scaffold to create .ralph/, then commit it.
	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("first scaffold: %v", err)
	}
	run("git", "add", "-A")
	run("git", "commit", "-q", "-m", "initial: tracked .ralph/ files")
	// Now re-scaffold with --force. untrackRalph should remove the
	// .ralph/ files from the index without deleting them on disk.
	if _, err := Scaffold(dir, Options{Force: true}); err != nil {
		t.Fatalf("force scaffold: %v", err)
	}
	// ls-files --error-unmatch returns non-zero if the file is NOT tracked.
	ls := exec.Command("git", "ls-files", ".ralph/PROMPT_build.md")
	ls.Dir = dir
	if out, err := ls.Output(); err == nil && len(strings.TrimSpace(string(out))) > 0 {
		t.Errorf(".ralph/PROMPT_build.md still tracked after force scaffold:\n%s", out)
	}
	// But the file must still exist on disk.
	if _, err := os.Stat(filepath.Join(dir, ".ralph", "PROMPT_build.md")); err != nil {
		t.Errorf(".ralph/PROMPT_build.md missing from disk: %v", err)
	}
	// And status must be clean: only the new .gitignore from this run
	// should show up (the .ralph/ files were already in HEAD, so removing
	// them from the index is recorded as a delete which the user can then
	// commit; the .gitignore from before is gone, the new one staged).
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = dir
	statusOut, _ := statusCmd.Output()
	hasRalphInStatus := false
	for _, line := range strings.Split(string(statusOut), "\n") {
		if strings.Contains(line, ".ralph/") {
			hasRalphInStatus = true
			break
		}
	}
	if hasRalphInStatus {
		t.Errorf("git status should not show .ralph/ as modified after untrack, got:\n%s", statusOut)
	}
}

func TestScaffoldUntrackRalphNoOpOnFreshRepo(t *testing.T) {
	// Fresh init with no previously-tracked .ralph/ — untrackRalph is a
	// no-op, no error, no git commands run.
	dir := makeGitRepo(t)
	// Real git init so untrackRalph's `git ls-files` works.
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "test")
	if _, err := Scaffold(dir, Options{}); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	// Status should be clean: .gitignore added (staged), .ralph/ created
	// but not tracked. No untrackRalph failure.
	stCmd := exec.Command("git", "status", "--porcelain")
	stCmd.Dir = dir
	out, _ := stCmd.Output()
	if strings.Contains(string(out), ".ralph/") {
		t.Errorf("fresh init should not have .ralph/ in git status:\n%s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
