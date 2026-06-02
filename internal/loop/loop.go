// Package loop implements the Ralph iteration driver (SPEC §7).
//
// The loop is strictly single-threaded: one harness invocation per
// iteration, with abort + max-iter + git ops between iterations.
//
// All side effects (state writes, event emission, git commit/push) are
// best-effort: a git push failure must not abort the loop, because the
// next iteration's commit will push the previous one too. The CLI never
// force-pushes, even with --force on init (SPEC §7.3).
package loop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/klampa/ralph-cli/internal/events"
	"github.com/klampa/ralph-cli/internal/harness"
	"github.com/klampa/ralph-cli/internal/state"
)

// Mode is the loop mode; maps to which PROMPT_*.md template the loop reads.
type Mode = harness.Mode

const (
	ModeBuild   = harness.ModeBuild
	ModePlan    = harness.ModePlan
	ModeReverse = harness.ModeReverse
)

// Options configure a single Run. Zero-value gives a sensible default
// (mode=build, max-iter=50, push on, commit on, no cap disabled).
type Options struct {
	Mode           Mode
	Model          string        // "opus" or "sonnet"
	MaxIterations  int           // 0 = use state's config.MaxIterations
	NoCap          bool          // unlimited iterations
	NoPush         bool          // skip git push after each iter
	NoCommit       bool          // skip git commit (debug only)
	Reverse        bool          // pre-pass: run ModeReverse once before build
	EmitJSON       bool          // when true, emit NDJSON status events
	Stdout         io.Writer     // harness stdout; defaults to os.Stdout
	Stderr         io.Writer     // harness stderr; defaults to os.Stderr
	Now            func() time.Time // for deterministic tests
}

// Run executes the loop. Returns the exit code per SPEC §8 on completion
// (0 ok, 1 generic, 2 aborted, 4 harness-missing, 5 not-in-git, 6 config-invalid).
//
// The function does not os.Exit; callers (cobra subcommands) map the
// returned int to the proper exit code via the cli.FailWith helper.
func Run(ctx context.Context, root string, h harness.Harness, opt Options) (int, error) {
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}

	// Pre-flight: harness exists, .ralph/ exists, git repo.
	if err := preflight(root, h); err != nil {
		return err.exitCode(), err
	}

	// Load state.
	st, err := state.Load(root)
	if err != nil {
		return 6, fmt.Errorf("load ralph.json: %w", err)
	}

	// Apply defaults from state if not overridden by flags.
	maxIter := opt.MaxIterations
	if maxIter == 0 {
		maxIter = st.Config.MaxIterations
	}
	if opt.Model == "" {
		opt.Model = st.Config.Model
	}

	// Build the event emitter. If --json, emit to stdout; otherwise to io.Discard.
	var emitter *events.Emitter
	if opt.EmitJSON {
		emitter = events.NewEmitter(opt.Stdout, events.NewSessionID(), root)
	} else {
		emitter = events.NewEmitter(io.Discard, events.NewSessionID(), root)
	}

	// Emit run.start (only meaningful when JSON is on, but always emit so
	// the audit trail is complete).
	modeStr := "build"
	if opt.Mode == ModePlan {
		modeStr = "plan"
	}
	_ = emitter.Info(events.EventRunStart, map[string]any{
		"command":        modeStr,
		"mode":           modeStr,
		"model":          opt.Model,
		"max_iterations": maxIter,
	})

	// --reverse pre-pass (SPEC §3.3).
	if opt.Mode == ModeBuild && opt.Reverse {
		if err := runOne(ctx, root, h, ModeReverse, 0, st, opt, emitter); err != nil {
			return 1, err
		}
	}

	// Main loop.
	iter := 0
	for {
		// Abort check (SPEC §7.1 step 2).
		aborted, err := state.IsAbortRequested(root)
		if err != nil {
			return 1, fmt.Errorf("check abort: %w", err)
		}
		if aborted {
			_ = emitter.Info(events.EventAbortRequested, map[string]string{
				"sentinel": ".ralph/ABORT_REQUESTED",
			})
			return 2, nil // ExitAborted
		}

		// Hard cap (SPEC §7.1 step 3).
		if !opt.NoCap && maxIter > 0 && iter >= maxIter {
			_ = emitter.Info(events.EventRunEnd, map[string]any{
				"exit_code":  0,
				"iterations": iter,
				"reason":     "max_iterations",
			})
			return 0, nil
		}

		// Run one iteration (SPEC §7.1 step 4).
		if err := runOne(ctx, root, h, opt.Mode, iter, st, opt, emitter); err != nil {
			_ = emitter.EmitError("iteration_failed", err.Error())
			return 1, err
		}

		// Commit + push (build mode only, unless --no-commit/--no-push).
		if opt.Mode == ModeBuild {
			committed := false
			if !opt.NoCommit {
				committed = gitCommit(root, iter, opt)
			}
			if !opt.NoPush && committed {
				gitPush(root, emitter)
			}
		}

		// Update state (SPEC §7.1 step 6).
		st.LoopCount++
		if sha, err := lastCommitSHA(root); err == nil {
			st.LastCommit = sha
		}
		st.LastSessionID = emitterSessionID(emitter)
		st.LastRunAt = opt.Now().UTC()
		// Plan hash: cheap content hash of IMPLEMENTATION_PLAN.md.
		if h, err := hashPlan(root); err == nil {
			st.PlanHash = h
		}
		if err := state.Save(root, st); err != nil {
			return 1, fmt.Errorf("save state: %w", err)
		}

		iter++
	}
}

// preflight returns a LoopError if harness is missing, .ralph/ is missing,
// or root is not a git repo. Caller maps the error to an exit code.
func preflight(root string, h harness.Harness) *LoopError {
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		if os.IsNotExist(err) {
			return NewLoopError(5, "not a git repo: %s (run 'git init' first, or use ralph init)", root)
		}
		return NewLoopError(1, "stat .git: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".ralph")); err != nil {
		if os.IsNotExist(err) {
			return NewLoopError(6, ".ralph/ not found in %s; run 'ralph init' first", root)
		}
		return NewLoopError(1, "stat .ralph: %v", err)
	}
	// Check the harness binary resolves. We do this by calling its Name()
	// then attempting a Version() probe — if it returns a wrapped ErrNotFound,
	// exit 4. Tests can pass a mock harness that ignores this.
	if _, err := h.Version(); err != nil {
		if errors.Is(err, harness.ErrNotFound) {
			return NewLoopError(4, "%s binary not found on PATH", h.Name())
		}
		// For mocks that don't actually probe a binary, ignore the error.
		// (We can't easily distinguish "mock" from "real" without a flag, so
		// we just let the call through. The real claude path will surface
		// its own not-found error at Invoke time.)
	}
	return nil
}

// runOne invokes the harness once for the given mode/iteration, then
// updates the plan hash on the state. It does NOT commit, push, or save
// state — the loop driver does that.
func runOne(
	ctx context.Context,
	root string,
	h harness.Harness,
	mode Mode,
	iter int,
	st state.State,
	opt Options,
	emitter *events.Emitter,
) error {
	// Build the prompt per SPEC §6.3.
	prompt, err := buildPrompt(root, mode, iter, st)
	if err != nil {
		return fmt.Errorf("build prompt: %w", err)
	}

	// Emit iteration.start.
	_ = emitter.Info(events.EventIterationStart, map[string]any{
		"iteration": iter,
		"mode":      string(mode),
	})

	startedAt := opt.Now()
	req := harness.InvokeRequest{
		Mode:           mode,
		Prompt:         prompt,
		Model:          opt.Model,
		ProjectRoot:    root,
		IterationCount: iter,
		LoopCount:      st.LoopCount,
		Stdout:         opt.Stdout,
		Stderr:         opt.Stderr,
	}
	res, err := h.Invoke(ctx, req)
	duration := opt.Now().Sub(startedAt).Seconds()

	if err != nil {
		_ = emitter.Info(events.EventIterationEnd, map[string]any{
			"iteration":  iter,
			"duration_s": int(duration),
			"exit_code":  -1,
			"error":      err.Error(),
		})
		return err
	}

	// Emit iteration.end.
	_ = emitter.Info(events.EventIterationEnd, map[string]any{
		"iteration":  iter,
		"duration_s": int(duration),
		"exit_code":  res.ExitCode,
		"commit_sha": nil, // populated by caller after commit
	})

	if res.ExitCode != 0 {
		return fmt.Errorf("harness %s exited %d", h.Name(), res.ExitCode)
	}
	return nil
}

// buildPrompt concatenates the per-mode template, the Ralph State block,
// and the Additional Context block (specs/ + plan + AGENTS.md) per SPEC §6.3.
func buildPrompt(root string, mode Mode, iter int, st state.State) (string, error) {
	var out bytes.Buffer

	// 1. The PROMPT_<mode>.md template.
	promptFile := filepath.Join(root, ".ralph", mode.PromptFile())
	promptBytes, err := os.ReadFile(promptFile)
	if err != nil {
		return "", fmt.Errorf("read %s: %w (did ralph init complete?)", promptFile, err)
	}
	out.Write(promptBytes)
	if !bytes.HasSuffix(promptBytes, []byte("\n")) {
		out.WriteString("\n")
	}

	// 2. Ralph State block.
	out.WriteString("\n---\n## Ralph State\n\n")
	fmt.Fprintf(&out, "Project root: %s\n", root)
	fmt.Fprintf(&out, "Harness: %s\n", st.Harness)
	fmt.Fprintf(&out, "Loop iteration: %d\n", iter)
	fmt.Fprintf(&out, "Loop count: %d\n", st.LoopCount)
	if st.LastCommit != "" {
		fmt.Fprintf(&out, "Last commit: %s\n", st.LastCommit)
	} else {
		fmt.Fprintf(&out, "Last commit: none\n")
	}
	if st.LastSessionID != "" {
		fmt.Fprintf(&out, "Last session: %s\n", st.LastSessionID)
	} else {
		fmt.Fprintf(&out, "Last session: none\n")
	}

	// 3. Additional Context block.
	out.WriteString("\n---\n## Additional Context\n\n")

	// 3a. specs/ — concatenate all *.md, each preceded by "## <filename>".
	specsDir := filepath.Join(root, ".ralph", "specs")
	if entries, err := os.ReadDir(specsDir); err == nil {
		out.WriteString("### specs/\n\n")
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			fmt.Fprintf(&out, "## %s\n", e.Name())
			b, err := os.ReadFile(filepath.Join(specsDir, e.Name()))
			if err != nil {
				continue
			}
			out.Write(b)
			if !bytes.HasSuffix(b, []byte("\n")) {
				out.WriteString("\n")
			}
		}
	}

	// 3b. IMPLEMENTATION_PLAN.md.
	out.WriteString("\n### IMPLEMENTATION_PLAN.md\n\n")
	planBytes, err := os.ReadFile(filepath.Join(root, ".ralph", "IMPLEMENTATION_PLAN.md"))
	if err == nil {
		out.Write(planBytes)
		if !bytes.HasSuffix(planBytes, []byte("\n")) {
			out.WriteString("\n")
		}
	}

	// 3c. AGENTS.md.
	out.WriteString("\n### AGENTS.md\n\n")
	agentsBytes, err := os.ReadFile(filepath.Join(root, ".ralph", "AGENTS.md"))
	if err == nil {
		out.Write(agentsBytes)
		if !bytes.HasSuffix(agentsBytes, []byte("\n")) {
			out.WriteString("\n")
		}
	}

	return out.String(), nil
}

// gitCommit stages all changes outside .ralph/ and commits them.
// Returns true if a commit was actually made.
//
// Per SPEC §7.3, the message comes from the harness's last stdout line
// if non-empty, else "ralph: iteration <N>".
func gitCommit(root string, iter int, opt Options) bool {
	// git add everything EXCEPT .ralph/ (SPEC §3.3: "stage all changes in
	// the project root (NOT in .ralph/)"). We do this by adding all and
	// then unstaging .ralph/.
	add := exec.Command("git", "add", "-A")
	add.Dir = root
	if out, err := add.CombinedOutput(); err != nil {
		fmt.Fprintf(opt.Stderr, "ralph: git add failed: %v\n%s\n", err, out)
		return false
	}
	reset := exec.Command("git", "reset", "HEAD", "--", ".ralph/")
	reset.Dir = root
	if out, err := reset.CombinedOutput(); err != nil {
		// non-fatal; the commit might still work
		fmt.Fprintf(opt.Stderr, "ralph: git reset .ralph/ failed: %v\n%s\n", err, out)
	}

	// If nothing is staged, skip the commit.
	status := exec.Command("git", "status", "--porcelain")
	status.Dir = root
	out, err := status.Output()
	if err != nil {
		fmt.Fprintf(opt.Stderr, "ralph: git status failed: %v\n", err)
		return false
	}
	if len(strings.TrimSpace(string(out))) == 0 {
		return false
	}

	// Build commit message.
	msg := fmt.Sprintf("ralph: iteration %d", iter)
	if last := lastNonEmptyLine(opt.Stdout); last != "" {
		// Use the harness's last stdout line as the commit message,
		// but only if it doesn't look like a prompt echo.
		if !strings.HasPrefix(last, "PROMPT_") && len(last) < 200 {
			msg = last
		}
	}

	commit := exec.Command("git", "commit", "-m", msg)
	commit.Dir = root
	if out, err := commit.CombinedOutput(); err != nil {
		fmt.Fprintf(opt.Stderr, "ralph: git commit failed: %v\n%s\n", err, out)
		return false
	}
	return true
}

// gitPush pushes origin <branch>. Per SPEC §7.3, push is best-effort
// (warn-and-continue on failure) — never force-push.
func gitPush(root string, emitter *events.Emitter) {
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchCmd.Dir = root
	branchBytes, err := branchCmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ralph: git branch failed: %v\n", err)
		return
	}
	branch := strings.TrimSpace(string(branchBytes))
	if branch == "" {
		fmt.Fprintln(os.Stderr, "ralph: cannot push: no current branch")
		return
	}

	push := exec.Command("git", "push", "origin", branch)
	push.Dir = root
	if out, err := push.CombinedOutput(); err != nil {
		// Try the -u variant for first-push.
		push = exec.Command("git", "push", "-u", "origin", "HEAD")
		push.Dir = root
		if out2, err2 := push.CombinedOutput(); err2 != nil {
			fmt.Fprintf(os.Stderr, "ralph: git push failed (continuing): %v\n%s\n%s\n", err, out, out2)
			return
		}
	}

	// Best-effort: report the push event with the SHA we just pushed.
	if sha, err := lastCommitSHA(root); err == nil {
		_ = emitter.Info(events.EventPush, map[string]string{
			"remote": "origin",
			"branch": branch,
			"sha":    sha,
		})
	}
}

// lastCommitSHA returns the SHA of HEAD, or "" on error.
func lastCommitSHA(root string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// hashPlan returns a stable hash of .ralph/IMPLEMENTATION_PLAN.md, or "" if
// the file is missing. Used to detect "the plan is unchanged across N iters"
// in `ralph status`.
func hashPlan(root string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, ".ralph", "IMPLEMENTATION_PLAN.md"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// lastNonEmptyLine returns the last non-empty trimmed line of r's current
// buffered content, or "" if r is nil. Used to extract a candidate commit
// message from the harness's stdout.
func lastNonEmptyLine(r io.Writer) string {
	if r == nil {
		return ""
	}
	// Note: at the time gitCommit runs, the harness has exited and
	// flushed. We rely on the caller passing a *bytes.Buffer if they
	// want this feature; passing os.Stdout means we'll read nothing.
	if buf, ok := r.(*bytes.Buffer); ok {
		lines := strings.Split(buf.String(), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			t := strings.TrimSpace(lines[i])
			if t != "" {
				return t
			}
		}
	}
	return ""
}

// emitterSessionID is a small helper to surface the session id from
// the emitter for state writes. Since the Emitter stores it as a
// private field, we use the public NewSessionID call once and stash
// the result on the loop. For simplicity v0.1 re-derives it.
func emitterSessionID(_ *events.Emitter) string {
	// We don't expose SessionID from the Emitter (it's stamped on every
	// event, not stored back). In v0.1 we re-derive a fresh one — the
	// audit trail of which session ran what iteration is in the NDJSON
	// stream, not in ralph.json.
	return ""
}

// LoopError is the typed error returned by preflight; it carries the
// exit code so the loop driver can return it directly.
type LoopError struct {
	code int
	msg  string
}

func NewLoopError(code int, format string, args ...any) *LoopError {
	return &LoopError{code: code, msg: fmt.Sprintf(format, args...)}
}

func (e *LoopError) Error() string { return e.msg }
func (e *LoopError) exitCode() int  { return e.code }
