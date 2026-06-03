// Package scaffold implements the .ralph/ directory creation and file
// writing logic used by `ralph init`. The cobra command (cli/init.go)
// parses flags and calls Scaffold; this package does the I/O.
package scaffold

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klampa/ralph-cli/internal/state"
)

// Options configures a Scaffold invocation. Field semantics match the
// cobra flags on `ralph init` (SPEC §3.1).
type Options struct {
	Force       bool // overwrite existing .ralph/ files
	ResetState  bool // with --force, also overwrite ralph.json
	NoTemplates bool // skip copying the 6 template files
	NoGitignore bool // don't append .ralph/ to .gitignore
	RalphVersion string // version string to stamp into ralph.json
}

// Result is what Scaffold produced, useful for tests and for the
// "Created .ralph/ in <path>" success message.
type Result struct {
	Created       []string // files/dirs newly written
	Preserved     []string // files that already existed and were kept (only when !Force)
	Skipped       []string // files skipped due to --no-templates
	StateFilePath string   // absolute path to ralph.json
}

// Scaffold creates .ralph/ inside root and writes the 6 template files,
// the initial ralph.json state file, the empty sessions/ directory, and
// updates .gitignore (unless --no-gitignore).
//
// Pre-conditions (caller must check before calling):
//   - root is an absolute path to a directory
//   - root contains a .git/ subdirectory
//   - the harness flag value has been validated as "claude" (or accepted)
//
// Behavior on existing .ralph/:
//   - if !Force: returns ExitConfigInvalid-style error (caller maps to
//     exit 6). This matches the "does not clobber" acceptance criterion.
//   - if Force: overwrites the 6 template files. Preserves ralph.json
//     unless ResetState is also set.
func Scaffold(root string, opts Options) (Result, error) {
	var res Result
	ralphDir := filepath.Join(root, ".ralph")

	// 1. Refuse to clobber unless --force.
	if _, err := os.Stat(ralphDir); err == nil {
		if !opts.Force {
			return res, fmt.Errorf(".ralph/ already exists in %s; pass --force to overwrite", root)
		}
	} else if !os.IsNotExist(err) {
		return res, fmt.Errorf("stat .ralph/: %w", err)
	}

	// 2. Create .ralph/ directory.
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		return res, fmt.Errorf("create .ralph/: %w", err)
	}
	res.Created = append(res.Created, ralphDir)

	// 3. Copy the 6 template files (unless --no-templates).
	if !opts.NoTemplates {
		for _, bf := range BundledFiles {
			dst := filepath.Join(ralphDir, bf.Dest)
			data, err := Read(bf.FSPath)
			if err != nil {
				return res, fmt.Errorf("read embedded %q: %w", bf.FSPath, err)
			}
			// In --force mode, overwrite. Otherwise skip if exists.
			if _, err := os.Stat(dst); err == nil && !opts.Force {
				res.Preserved = append(res.Preserved, dst)
				continue
			}
			if err := os.WriteFile(dst, data, bf.Mode); err != nil {
				return res, fmt.Errorf("write %q: %w", dst, err)
			}
			// os.WriteFile honors the mode, so no separate chmod needed.
			res.Created = append(res.Created, dst)
		}
	} else {
		for _, bf := range BundledFiles {
			res.Skipped = append(res.Skipped, filepath.Join(ralphDir, bf.Dest))
		}
	}

	// 4. Write ralph.json (or preserve existing unless --reset-state).
	statePath := state.Path(root)
	stateExists := false
	if _, err := os.Stat(statePath); err == nil {
		stateExists = true
	}
	if !stateExists || (opts.Force && opts.ResetState) {
		st := state.Default()
		st.RalphVersion = opts.RalphVersion
		st.CreatedAt = time.Now().UTC()
		if err := state.Save(root, st); err != nil {
			return res, fmt.Errorf("save ralph.json: %w", err)
		}
		res.Created = append(res.Created, statePath)
	} else {
		res.Preserved = append(res.Preserved, statePath)
	}
	res.StateFilePath = statePath

	// 5. Create empty sessions/ directory (reserved for v0.2).
	sessionsDir := filepath.Join(ralphDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		return res, fmt.Errorf("create .ralph/sessions/: %w", err)
	}
	res.Created = append(res.Created, sessionsDir)

	// 6. Update .gitignore (unless --no-gitignore).
	if !opts.NoGitignore {
		if err := ensureGitignore(root); err != nil {
			return res, fmt.Errorf("update .gitignore: %w", err)
		}
	}

	// 7. Untrack any .ralph/ files that were already tracked in git.
	// Issue #4 (v0.1.2): without this, `git add -A` after init would stage
	// the .gitignore change but the previously-tracked .ralph/ files would
	// remain tracked. Best-effort: a `git rm` failure (no git, no
	// tracking, etc.) is non-fatal — the .gitignore + init files we
	// just wrote still accomplish the user's goal.
	if !opts.NoGitignore {
		// Best-effort: never block init on a git-tracking cleanup.
		_ = untrackRalph(root)
	}

	return res, nil
}

// ensureGitignore appends ".ralph/" to <root>/.gitignore, creating the
// file if it doesn't exist. If .gitignore already contains a .ralph/
// entry (in any form — .ralph, .ralph/, ./.ralph/, etc.), no-op.
func ensureGitignore(root string) error {
	giPath := filepath.Join(root, ".gitignore")
	existing, err := os.ReadFile(giPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range splitLines(string(existing)) {
		trimmed := trimWhitespace(line)
		if trimmed == ".ralph" || trimmed == ".ralph/" || trimmed == "./.ralph" || trimmed == "./.ralph/" {
			return nil // already covered
		}
	}
	// Append. Make sure the file ends with a newline before we add to it.
	content := string(existing)
	if len(content) > 0 && content[len(content)-1] != '\n' {
		content += "\n"
	}
	content += "\n# ralph-cli state (added by ralph init)\n.ralph/\n"
	return os.WriteFile(giPath, []byte(content), 0o644)
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func trimWhitespace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

// untrackRalph removes .ralph/ from the git index (without deleting the
// files on disk) when it has been previously tracked.
//
// Issue #4 (v0.1.2): if a user runs `ralph init` in a repo that already
// has .ralph/ files committed (e.g. an existing ralph project), the
// .gitignore entry added by Scaffold alone does not untrack them — the
// files stay in the index, and `git add -A` keeps staging changes to
// them. `git rm -r --cached .ralph/` removes them from the index while
// leaving the on-disk copies intact.
//
// Best-effort: returns nil on any failure (no .git, no .ralph/ in index,
// etc.). Callers should not block init on this.
func untrackRalph(root string) error {
	// Only attempt if this is actually a git repo. Caller already checks
	// for .git/, but re-check defensively.
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		return nil
	}
	// Only attempt if .ralph/ exists on disk. After Scaffold it always
	// does, but be defensive in case the caller is using a custom flow.
	if _, err := os.Stat(filepath.Join(root, ".ralph")); err != nil {
		return nil
	}
	// `git ls-files` lists tracked files. If none match .ralph/, skip.
	ls := exec.Command("git", "ls-files", ".ralph/")
	ls.Dir = root
	out, err := ls.Output()
	if err != nil {
		return nil // not a git repo or git unavailable — ignore
	}
	if strings.TrimSpace(string(out)) == "" {
		return nil // nothing tracked under .ralph/, nothing to do
	}
	// Untrack. We ignore the error: `git rm -r --cached` may fail on
	// edge cases (submodule, etc.) and the user can clean up manually.
	rm := exec.Command("git", "rm", "-r", "--cached", ".ralph/")
	rm.Dir = root
	// Silence stderr/stdout; the user will see the resulting "deleted"
	// lines on the next `git status` anyway.
	if out, err := rm.CombinedOutput(); err != nil {
		return fmt.Errorf("git rm --cached .ralph/: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}


