package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klampa/ralph-cli/internal/state"
)

// makeRalphProject creates a temp dir with a .git/ + .ralph/ scaffold
// (minimal: ralph.json, IMPLEMENTATION_PLAN.md with 3 checkboxes).
// Returns the dir. planItems is what gets written to the plan file.
func makeRalphProject(t *testing.T, planContent string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ralph", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md"),
		[]byte(planContent), 0o644,
	); err != nil {
		t.Fatal(err)
	}
	st := state.Default()
	st.LoopCount = 3
	st.LastCommit = "" // not set by default
	st.LastRunAt = time.Date(2026, 6, 3, 14, 0, 0, 0, time.UTC)
	if err := state.Save(dir, st); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestFindRalphDirExplicitPathRequiresExactMatch covers issue #6:
// `ralph status <path>` must NOT silently fall back to a parent
// .ralph/ when the exact path has no .ralph/.
func TestFindRalphDirExplicitPathRequiresExactMatch(t *testing.T) {
	// Parent has .ralph/, child has none.
	parent := makeRalphProject(t, "- [ ] one\n- [x] two\n")
	child := filepath.Join(parent, "subdir", "no-ralph-here")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	// Default mode (no path arg) — allowWalkUp=true → should find parent's.
	got, err := findRalphDir(child, true)
	if err != nil {
		t.Fatalf("findRalphDir(walkUp=true): %v", err)
	}
	if got != parent {
		t.Errorf("findRalphDir(walkUp=true): got %q, want %q (parent)", got, parent)
	}
	// Explicit-path mode — allowWalkUp=false → must NOT find parent.
	_, err = findRalphDir(child, false)
	if err == nil {
		t.Error("findRalphDir(walkUp=false) on child: expected error, got nil")
	}
	if _, ok := err.(*RalphNotFoundError); !ok {
		t.Errorf("expected *RalphNotFoundError, got %T: %v", err, err)
	}
}

// TestFindRalphDirExactMatchWhenPresent ensures the explicit path is
// returned directly when .ralph/ is exactly there.
func TestFindRalphDirExactMatchWhenPresent(t *testing.T) {
	dir := makeRalphProject(t, "- [ ] one\n")
	got, err := findRalphDir(dir, false)
	if err != nil {
		t.Fatalf("findRalphDir(walkUp=false): %v", err)
	}
	if got != dir {
		t.Errorf("findRalphDir(walkUp=false): got %q, want %q", got, dir)
	}
}

// TestFindRalphDirWalkUpStopsAtFSRoot is a sanity check that the walk
// up doesn't loop forever and returns a clear error at the FS root.
func TestFindRalphDirWalkUpStopsAtFSRoot(t *testing.T) {
	// /tmp on most systems is not the FS root, so a deep-enough walk
	// will hit /. We use a path with no .ralph/ in any ancestor.
	dir := t.TempDir()
	_, err := findRalphDir(dir, true)
	if err == nil {
		t.Fatal("expected error when no .ralph/ in any ancestor")
	}
	if _, ok := err.(*RalphNotFoundError); !ok {
		t.Errorf("expected *RalphNotFoundError, got %T", err)
	}
}

// TestReadPlanItems covers the plan-file parser used for the derived
// status fields.
func TestReadPlanItems(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := `# My Plan
- [ ] first todo
- [x] second done
* [ ] third
  - [X] fourth (uppercase)
- [x] fifth
plain text line
`
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "IMPLEMENTATION_PLAN.md"),
		[]byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	p := readPlanItems(dir)
	if got, want := len(p.unchecked), 2; got != want {
		t.Errorf("unchecked count: got %d, want %d", got, want)
	}
	if got, want := len(p.checked), 3; got != want {
		t.Errorf("checked count: got %d, want %d", got, want)
	}
	if p.unchecked[0] != "first todo" {
		t.Errorf("first unchecked: got %q, want %q", p.unchecked[0], "first todo")
	}
	if p.unchecked[1] != "third" {
		t.Errorf("second unchecked: got %q, want %q", p.unchecked[1], "third")
	}
}

// TestSummarizePlan covers the derived-field math.
func TestSummarizePlan(t *testing.T) {
	// 2 of 4 done → 50%.
	p := planItems{
		checked:   []string{"a", "b"},
		unchecked: []string{"c", "d"},
	}
	pct, current, next, exhausted := summarizePlan(p)
	if pct != 50.0 {
		t.Errorf("progress_pct: got %v, want 50.0", pct)
	}
	if current != "c" {
		t.Errorf("current_task: got %q, want %q", current, "c")
	}
	if next != "d" {
		t.Errorf("next_task: got %q, want %q", next, "d")
	}
	if exhausted {
		t.Error("plan should not be exhausted with 2 remaining")
	}

	// 4 of 4 done → 100%, exhausted.
	p2 := planItems{checked: []string{"a", "b", "c", "d"}}
	pct, _, _, exhausted = summarizePlan(p2)
	if pct != 100.0 {
		t.Errorf("progress_pct (all done): got %v, want 100.0", pct)
	}
	if !exhausted {
		t.Error("plan should be exhausted with 0 remaining")
	}

	// 0 of 1 → 0%, current set.
	p3 := planItems{unchecked: []string{"only"}}
	pct, current, next, exhausted = summarizePlan(p3)
	if pct != 0.0 {
		t.Errorf("progress_pct (one left): got %v, want 0.0", pct)
	}
	if current != "only" {
		t.Errorf("current: got %q, want %q", current, "only")
	}
	if next != "" {
		t.Errorf("next: got %q, want empty (only one left)", next)
	}
	if exhausted {
		t.Error("plan should not be exhausted with 1 remaining")
	}

	// Empty plan → 100%, but not "exhausted" (we don't know if it was
	// intentional or just uninitialized).
	p4 := planItems{}
	pct, _, _, exhausted = summarizePlan(p4)
	if pct != 100.0 {
		t.Errorf("progress_pct (empty): got %v, want 100.0", pct)
	}
	if exhausted {
		t.Error("empty plan should not be marked exhausted")
	}
}

// TestBuildStatusReport is the integration test for the --json derived
// fields (issue #7).
func TestBuildStatusReport(t *testing.T) {
	dir := makeRalphProject(t, "- [ ] first todo\n- [ ] second todo\n- [x] done\n")
	st := state.Default()
	st.LoopCount = 7
	st.Config.MaxIterations = 50
	st.LastCommit = "" // no git history in this test → no commit_at

	report := buildStatusReport(dir, st)
	if report == nil {
		t.Fatal("buildStatusReport returned nil")
	}
	// Initialized + project fields are always present.
	if got, want := report["initialized"], true; got != want {
		t.Errorf("initialized: got %v, want %v", got, want)
	}
	if report["project"] != dir {
		t.Errorf("project: got %v, want %v", report["project"], dir)
	}
	// Derived fields.
	if got, want := report["progress_pct"], 33.33; got != want {
		t.Errorf("progress_pct: got %v, want %v", got, want)
	}
	if got, want := report["current_task"], "first todo"; got != want {
		t.Errorf("current_task: got %v, want %v", got, want)
	}
	if got, want := report["next_task"], "second todo"; got != want {
		t.Errorf("next_task: got %v, want %v", got, want)
	}
	if got, want := report["iterations_remaining"], 43; got != want {
		t.Errorf("iterations_remaining: got %v, want %v", got, want)
	}
	if report["plan_exhausted"] != false {
		t.Errorf("plan_exhausted: got %v, want false", report["plan_exhausted"])
	}
	// last_commit_at should be ABSENT (not null) when there's no commit.
	if _, present := report["last_commit_at"]; present {
		t.Errorf("last_commit_at: should be absent when no LastCommit set, got %v",
			report["last_commit_at"])
	}
	// Counters.
	if got, want := report["plan_total"], 3; got != want {
		t.Errorf("plan_total: got %v, want %v", got, want)
	}
	if got, want := report["plan_done"], 1; got != want {
		t.Errorf("plan_done: got %v, want %v", got, want)
	}
	if got, want := report["plan_remaining"], 2; got != want {
		t.Errorf("plan_remaining: got %v, want %v", got, want)
	}
}

// TestBuildStatusReportIterationsRemainingClampsAtZero ensures the
// remaining counter never goes negative.
func TestBuildStatusReportIterationsRemainingClampsAtZero(t *testing.T) {
	dir := makeRalphProject(t, "- [ ] one\n")
	st := state.Default()
	st.LoopCount = 100
	st.Config.MaxIterations = 10
	report := buildStatusReport(dir, st)
	if got, want := report["iterations_remaining"], 0; got != want {
		t.Errorf("iterations_remaining: got %v, want %v (clamped)", got, want)
	}
}

// TestBuildStatusReportPlanExhausted is the exhausted-plan path: every
// checkbox is [x], so plan_exhausted must be true.
func TestBuildStatusReportPlanExhausted(t *testing.T) {
	dir := makeRalphProject(t, "- [x] one\n- [x] two\n")
	st := state.Default()
	report := buildStatusReport(dir, st)
	if report["plan_exhausted"] != true {
		t.Errorf("plan_exhausted: got %v, want true", report["plan_exhausted"])
	}
	if report["current_task"] != "" {
		t.Errorf("current_task: got %v, want empty (no unchecked)", report["current_task"])
	}
	if got, want := report["progress_pct"], 100.0; got != want {
		t.Errorf("progress_pct: got %v, want %v", got, want)
	}
}

// TestBuildStatusReportIsValidJSON is a small sanity check that the
// returned map marshals to valid JSON (so a buggy field type doesn't
// blow up the CLI on the user's terminal).
func TestBuildStatusReportIsValidJSON(t *testing.T) {
	dir := makeRalphProject(t, "- [ ] one\n- [x] two\n")
	report := buildStatusReport(dir, state.Default())
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// And round-trips back to a map.
	var back map[string]any
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if _, ok := back["progress_pct"]; !ok {
		t.Error("progress_pct missing from JSON")
	}
}

// TestIsNotFound covers the error helper used to distinguish
// "not initialized" (exit 0, friendly) from "init error" (exit 3).
func TestIsNotFound(t *testing.T) {
	if isNotFound(nil) {
		t.Error("isNotFound(nil) should be false")
	}
	if !isNotFound(&RalphNotFoundError{}) {
		t.Error("isNotFound(RalphNotFoundError) should be true")
	}
	if isNotFound(os.ErrNotExist) {
		t.Error("isNotFound(os.ErrNotExist) should be false (different sentinel)")
	}
}

// TestRalphNotFoundErrorMessage ensures the sentinel's Error() method
// includes the path so users know WHERE the missing .ralph/ was.
func TestRalphNotFoundErrorMessage(t *testing.T) {
	e := &RalphNotFoundError{path: "/some/path"}
	if !strings.Contains(e.Error(), "/some/path") {
		t.Errorf("error should include the path, got: %s", e.Error())
	}
}

// TestLastCommitISOTime covers the "last commit at" derivation.
// We need a real git repo because the helper shells out to `git log`.
func TestLastCommitISOTime(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
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
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-q", "-m", "init")
	// Get HEAD sha and ask the helper for its ISO date.
	shaCmd := exec.Command("git", "rev-parse", "HEAD")
	shaCmd.Dir = dir
	shaOut, err := shaCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	sha := strings.TrimSpace(string(shaOut))
	iso := lastCommitISOTime(dir, sha)
	if iso == "" {
		t.Fatal("lastCommitISOTime returned empty for a real commit")
	}
	// ISO-8601 dates start with YYYY-MM-DD; spot-check.
	if !strings.HasPrefix(iso, "20") || len(iso) < 10 {
		t.Errorf("lastCommitISOTime returned non-ISO date: %q", iso)
	}
	// Empty sha → empty result.
	if got := lastCommitISOTime(dir, ""); got != "" {
		t.Errorf("lastCommitISOTime(empty): got %q, want \"\"", got)
	}
	// Bogus sha → empty result (graceful).
	if got := lastCommitISOTime(dir, "deadbeef"); got != "" {
		t.Errorf("lastCommitISOTime(bogus): got %q, want \"\"", got)
	}
}
