package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/state"
)

var statusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "Print current ralph state (human-readable or --json)",
	Long: `Print current ralph state. Default: human-readable. --json:
machine-readable status with derived fields (see SPEC §9).

If no .ralph/ exists at the path, prints a helpful "not initialized"
message and exits 0.

Path resolution:
  - With an explicit path (e.g. "ralph status /path/to/proj"): the
    command looks for ".ralph/" at exactly that path. If missing,
    it errors with a clear message and exit 6 — it does NOT silently
    fall back to a parent .ralph/.
  - With no path (e.g. "ralph status"): the command walks up from
    the cwd looking for the nearest .ralph/.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().Bool("json", false, "Emit as JSON with derived fields (see SPEC §9)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	explicit := len(args) > 0
	target := "."
	if explicit {
		target = args[0]
	}
	absRoot, err := filepath.Abs(target)
	if err != nil {
		return FailWith(ExitInitFail, "resolve path %q: %v", target, err)
	}

	emitJSON, _ := cmd.Flags().GetBool("json")

	// Resolve the actual project root. When an explicit path was given,
	// findRalphDir with allowWalkUp=false errors cleanly if .ralph/
	// is not at the exact path. When no path was given, allowWalkUp=true
	// preserves the v0.1 behavior of walking up to the nearest .ralph/.
	resolvedRoot, err := findRalphDir(absRoot, !explicit)
	if err != nil {
		// When no .ralph/ is found:
		//   - With an explicit path arg, return a hard error (issue #6):
		//     the user told us WHERE to look, so silently using a
		//     parent is the bug. Exit 6 + clear message.
		//   - With no path arg (default mode, walks up), return a
		//     friendly "not initialized" message and exit 0 — the
		//     v0.1.1 behavior.
		if _, ok := err.(*RalphNotFoundError); ok {
			if explicit {
				return FailWith(ExitConfigInvalid,
					"no .ralph/ at %s; pass a path that has been initialized with 'ralph init' (issue #6: explicit paths no longer walk up)",
					absRoot)
			}
			if emitJSON {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"initialized": false,
					"project":     absRoot,
					"explicit":    explicit,
				})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(),
					"ralph: %s is not initialized (no .ralph/). Run 'ralph init' to scaffold.\n", absRoot)
			}
			return nil
		}
		// Other I/O error.
		return FailWith(ExitInitFail, "%v", err)
	}

	st, err := state.Load(resolvedRoot)
	if err != nil {
		return FailWith(ExitConfigInvalid, "load ralph.json: %v", err)
	}

	if emitJSON {
		return emitStatusJSON(cmd, resolvedRoot, st)
	}

	return emitStatusHuman(cmd, resolvedRoot, st)
}

// emitStatusJSON writes the v0.1.2 JSON output: the raw state PLUS
// derived fields useful for dashboards (issue #7).
//
// Derived fields:
//   - current_task:      first "- [ ] ..." line in IMPLEMENTATION_PLAN.md
//   - next_task:         second "- [ ] ..." line, or "" if only one left
//   - progress_pct:      0..100 float, percent of "- [x]" out of all checkboxes
//   - iterations_remaining: max_iterations - loop_count, clamped at 0
//   - last_commit_at:    ISO-8601 committer date of last_commit, or null
//   - plan_exhausted:    true when no unchecked items AND loop_count > 0
//   - initialized:       always true (we only get here when .ralph/ exists)
func emitStatusJSON(cmd *cobra.Command, root string, st state.State) error {
	out := buildStatusReport(root, st)
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// buildStatusReport assembles the v0.1.2 status report (state + derived).
// Exported for tests.
func buildStatusReport(root string, st state.State) map[string]any {
	planItems := readPlanItems(root)
	progressPct, currentTask, nextTask, planExhausted := summarizePlan(planItems)

	iterationsRemaining := 0
	if st.Config.MaxIterations > 0 {
		iterationsRemaining = st.Config.MaxIterations - st.LoopCount
		if iterationsRemaining < 0 {
			iterationsRemaining = 0
		}
	}

	var lastCommitAt *string
	if st.LastCommit != "" {
		if ts := lastCommitISOTime(root, st.LastCommit); ts != "" {
			lastCommitAt = &ts
		}
	}

	report := map[string]any{
		"initialized":          true,
		"project":              root,
		"current_task":         currentTask,
		"next_task":            nextTask,
		"progress_pct":         progressPct,
		"iterations_remaining": iterationsRemaining,
		"plan_exhausted":       planExhausted,
		"plan_total":           len(planItems.checked) + len(planItems.unchecked),
		"plan_done":            len(planItems.checked),
		"plan_remaining":       len(planItems.unchecked),
	}
	// Only add last_commit_at when we actually have a timestamp —
	// otherwise JSON would show "null" (still valid, but explicit
	// absence reads better for downstream tools).
	if lastCommitAt != nil {
		report["last_commit_at"] = *lastCommitAt
	}
	// We marshal the raw state then unmarshal it into a map so the
	// envelope mirrors the on-disk JSON exactly.
	raw, _ := json.Marshal(st)
	var stateMap map[string]any
	_ = json.Unmarshal(raw, &stateMap)
	for k, v := range stateMap {
		report[k] = v
	}
	return report
}

// planItems is a small struct holding the split between done / open
// tasks parsed from IMPLEMENTATION_PLAN.md.
type planItems struct {
	checked   []string // "- [x] ..." entries (without the "- [x] " prefix)
	unchecked []string // "- [ ] ..." entries (without the "- [ ] " prefix)
}

// readPlanItems parses the canonical plan file. Missing file → empty
// (treated as "nothing planned"). Errors are swallowed (best-effort;
// the status report is informational, not authoritative).
func readPlanItems(root string) planItems {
	var p planItems
	b, err := os.ReadFile(filepath.Join(root, ".ralph", "IMPLEMENTATION_PLAN.md"))
	if err != nil {
		return p
	}
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		switch {
		case strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "* [x] "):
			p.checked = append(p.checked, strings.TrimSpace(trimmed[6:]))
		case strings.HasPrefix(trimmed, "- [X] ") || strings.HasPrefix(trimmed, "* [X] "):
			p.checked = append(p.checked, strings.TrimSpace(trimmed[6:]))
		case strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "* [ ] "):
			p.unchecked = append(p.unchecked, strings.TrimSpace(trimmed[6:]))
		}
	}
	return p
}

// summarizePlan turns planItems into the (progressPct, current, next, exhausted) tuple.
func summarizePlan(p planItems) (float64, string, string, bool) {
	total := len(p.checked) + len(p.unchecked)
	if total == 0 {
		// No plan file or no checkboxes: report 100% (nothing to do)
		// but only mark exhausted when there was a plan at all.
		return 100.0, "", "", false
	}
	pct := float64(len(p.checked)) / float64(total) * 100.0
	// Round to 2 decimals for display.
	pct = float64(int(pct*100)) / 100.0
	current := ""
	next := ""
	if len(p.unchecked) > 0 {
		current = p.unchecked[0]
	}
	if len(p.unchecked) > 1 {
		next = p.unchecked[1]
	}
	exhausted := len(p.unchecked) == 0
	return pct, current, next, exhausted
}

// lastCommitISOTime returns the committer date of a commit SHA in
// ISO-8601 format (e.g. "2026-06-03T15:30:00Z"), or "" on any failure.
// Uses `git log -1 --format=%cI <sha>` so we don't depend on the
// commit being reachable from HEAD (state.LastCommit can be older
// than the loop's current branch tip if the user reset).
func lastCommitISOTime(root, sha string) string {
	if sha == "" {
		return ""
	}
	cmd := exec.Command("git", "log", "-1", "--format=%cI", sha)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// emitStatusHuman writes the v0.1.x text output (used when --json is
// not passed). Adds the derived fields as well so the human output is
// also richer than the v0.1.1 version.
func emitStatusHuman(cmd *cobra.Command, root string, st state.State) error {
	projectName := filepath.Base(root)
	fmt.Fprintf(cmd.OutOrStdout(), "Project: %s\n", projectName)
	fmt.Fprintf(cmd.OutOrStdout(), "Harness: %s\n", st.Harness)
	fmt.Fprintf(cmd.OutOrStdout(), "Loop count: %d\n", st.LoopCount)
	if st.LastCommit != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Last commit: %s\n", st.LastCommit)
		if ts := lastCommitISOTime(root, st.LastCommit); ts != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Last commit at: %s\n", ts)
		}
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Last commit: none\n")
	}
	if st.LastSessionID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Last session: %s\n", st.LastSessionID)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Last session: none\n")
	}
	if st.PlanHash != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Plan hash: %s\n", st.PlanHash)
	}
	// Plan summary.
	planItems := readPlanItems(root)
	pct, current, next, exhausted := summarizePlan(planItems)
	fmt.Fprintf(cmd.OutOrStdout(),
		"Plan progress: %.2f%% (%d done, %d remaining)\n",
		pct, len(planItems.checked), len(planItems.unchecked),
	)
	if current != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Current task: %s\n", current)
	}
	if next != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Next task: %s\n", next)
	}
	if exhausted {
		fmt.Fprintln(cmd.OutOrStdout(), "Plan status: exhausted")
	}
	// .ralph/ size is a small nicety; report or omit.
	if size, err := dirSize(filepath.Join(root, ".ralph")); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), ".ralph/ size: %s\n", humanSize(size))
	}
	return nil
}

// findRalphDir returns the project root containing .ralph/.
//
// When allowWalkUp is true (default mode, no path arg), it walks up
// from start looking for the nearest .ralph/. The walk stops at the
// filesystem root.
//
// When allowWalkUp is false (an explicit path was given), it checks
// ONLY the start path and returns ErrRalphNotFound if missing.
// Returning an error here causes `ralph status <path>` to fail with
// a clear message instead of silently using a parent .ralph/.
// Issue #6 (v0.1.2).
func findRalphDir(start string, allowWalkUp bool) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", start, err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".ralph")); err == nil {
		return abs, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat .ralph/: %w", err)
	}
	if !allowWalkUp {
		return "", &RalphNotFoundError{path: abs}
	}
	// Walk up.
	cur := abs
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", &RalphNotFoundError{path: abs}
		}
		cur = parent
		if _, err := os.Stat(filepath.Join(cur, ".ralph")); err == nil {
			return cur, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat .ralph/: %w", err)
		}
	}
}

// RalphNotFoundError is returned by findRalphDir when no .ralph/ is
// found at the requested location. Sentinel-typed so callers can
// distinguish "not initialized" (exit 0 with a friendly message)
// from "init error" (exit 3).
type RalphNotFoundError struct{ path string }

func (e *RalphNotFoundError) Error() string {
	return fmt.Sprintf("no .ralph/ at %s", e.path)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*RalphNotFoundError); ok {
		return true
	}
	return false
}

// dirSize returns the total file size under path (recursive).
func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// humanSize returns a human-readable byte count.
func humanSize(n int64) string {
	const (
		k = 1024
		m = k * 1024
		g = m * 1024
	)
	switch {
	case n >= g:
		return fmt.Sprintf("%.1fG", float64(n)/float64(g))
	case n >= m:
		return fmt.Sprintf("%.1fM", float64(n)/float64(m))
	case n >= k:
		return fmt.Sprintf("%.1fK", float64(n)/float64(k))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
