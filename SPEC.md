# ralph-cli — SPEC

**Author:** Bert (Squad Architecture Agent) · Task 2 of 4
**Date:** 2026-06-02
**Status:** Implementation-ready · contracts pinned · scope: v0.1
**Target implementer:** Elmo (Task 3)
**Repo:** `~/projects/ralph-cli/` (autogpt.local / m5 NUC)
**Upstream research:** `RESEARCH.md` (Ernie, Task 1) — read sections 0, 5, 6, 8 for context

---

## 1. Purpose & Scope

`ralph-cli` is a globally-installable, single-binary CLI that wraps ClaytonFarr's
canonical Ralph file templates into a hidden `.ralph/` scaffold at any project
root and orchestrates Claude Code in a continuous, one-task-per-iteration loop.

**v0.1 ships:** `init`, `plan`, `loop`, `loop --reverse`, `status`, `abort`,
`--version`, `--help`, plus a stubbed `--harness` flag.

**v0.1 does NOT ship:** `doctor`, telemetry, Homebrew tap, multi-harness routing,
TUI, prd.json schema validation. See §13 for v0.2 deferrals.

**The CLI is a thin process launcher.** It stages files, invokes Claude Code
with the right flags, manages the loop, and emits status events. It does NOT
interpret Ralph's output, parse prd.json, manage sandboxes, or replace Claude
Code.

---

## 2. Architecture

### 2.1 Language & build

- **Go 1.22+** (use `go.mod` with `go 1.22` minimum)
- **Single static binary** per target: `darwin/amd64`, `darwin/arm64`,
  `linux/amd64`, `linux/arm64`
- No CGo. No cgo-required deps. Pure-Go stdlib + minimal 3rd-party.
- Module path: `github.com/klampa/ralph-cli`
- Entry point: `cmd/ralph/main.go`
- All non-`main` code lives under `internal/` (no public package surface)

### 2.2 Package layout

```
cmd/ralph/main.go              # entry point, calls internal/cli.Execute()

internal/
  cli/
    root.go                    # cobra root, global flags, --version, --help
    init.go                    # `ralph init` subcommand
    plan.go                    # `ralph plan` subcommand
    loop.go                    # `ralph loop` subcommand (incl. --reverse)
    status.go                  # `ralph status` subcommand
    abort.go                   # `ralph abort` subcommand
  scaffold/
    scaffold.go                # creates .ralph/ directory, writes files
    templates.go               # embed.FS accessor for bundled templates
  harness/
    harness.go                 # Harness interface { Name(), Invoke(ctx, ...) }
    claude.go                  # ClaudeCode implementation
  loop/
    loop.go                    # iteration loop driver, abort check, git ops
  state/
    state.go                   # .ralph/ralph.json read/write, schema, migrations
    abort.go                   # ABORT_REQUESTED sentinel create/check
  events/
    events.go                  # JSON status event types + Emitter
  version/
    version.go                 # -ldflags injected: Version, Commit, Date
  doctor/                      # STUB for v0.1 — package exists but is unused
    doctor.go                  # only referenced by a hidden `--doctor-debug` flag
                              # so Elmo can stub it without scope-creep
testdata/
  smoke/                       # minimal smoke-test fixture (§10)
    specs/01-hello.md
    src/hello.go
    src/hello_test.go
    .gitignore
```

**Why `internal/`:** enforce that no third-party Go code can import our
packages. Smaller blast radius if we change internals later.

**Why `doctor/` exists as a stub:** it reserves the package name so future
additions don't churn the tree. The package may be empty or a single `// TODO`
file. The hidden `--doctor-debug` flag is optional and exists only to avoid
"unused package" lint failures; omit it if Go doesn't complain.

### 2.3 Dependencies (3rd-party Go libs)

Pinned to majors; pin minor versions at implementation time:

- `github.com/spf13/cobra` v1.8+ — subcommand routing, help text, flag parsing
- `github.com/spf13/viper` v1.18+ — `.ralph/ralph.json` + optional `.ralphrc`
  reading (only if init needs to merge a global config; otherwise stdlib is
  enough — Elmo's call)

**No** JSON5 lib, **no** logging lib, **no** TUI lib, **no** web framework.
Stdlib `log/slog` for structured logging. Stdlib `encoding/json` for state.

---

## 3. Subcommand surface (the user-facing contract)

All subcommands accept an optional positional `[path]` argument (default: `.`).
The path is resolved to an absolute path before any operations.

```
ralph <command> [path] [flags]
```

### 3.1 `ralph init [path]`

Scaffold `.ralph/` at the project root.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool | false | Overwrite existing `.ralph/` files (preserves `ralph.json` state unless `--reset-state` also passed) |
| `--reset-state` | bool | false | With `--force`: also overwrite `ralph.json` (state goes to zero) |
| `--harness` | string | `claude` | Harness name. v0.1: only `claude` is accepted; any other value exits 6 (config-invalid) |
| `--no-templates` | bool | false | Skip copying the 6 template files (use when user has hand-edited them and wants to re-init only the state file) |
| `--no-gitignore` | bool | false | Don't add `.ralph/` to `.gitignore` (user wants to commit it) |

**Pre-conditions (exit 3 if violated):**
- Path must exist and be a directory
- Path must contain a `.git` directory OR user must pass `--allow-non-git` (defer this flag to v0.2; in v0.1, exit 5 if no `.git`)

**Behavior:**
1. Create `.ralph/` directory
2. Write the 6 template files (see §5) from `embed.FS` — skip if `--no-templates`
3. Write `.ralph/ralph.json` initial state (see §10)
4. Create empty `.ralph/sessions/` directory
5. If `.gitignore` exists and does not contain `.ralph/`, append `.ralph/` (unless `--no-gitignore`)
6. Print success: `Created .ralph/ in <path>`

### 3.2 `ralph plan [path]`

Run the planning loop: gap analysis, produce `IMPLEMENTATION_PLAN.md` from `specs/`.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | false | Emit JSON status events to stdout (see §9) |
| `--model` | string | `opus` | `opus` or `sonnet`; forwarded to Claude Code |
| `--max-iterations` | int | `1` | Plan mode is typically one-shot |
| `--no-push` | bool | false | Skip git push (plan mode rarely pushes) |

**Pre-conditions:**
- `.ralph/` must exist (exit 6 if not — suggests user run `ralph init` first)
- `claude` binary must be on `PATH` (exit 4 if not)
- Path must be a git repo (exit 5 if not)

**Behavior:** runs the loop driver (§7) with `PROMPT_plan.md` as the prompt
and mode=plan. Plan mode does NOT commit code changes — it only updates
`IMPLEMENTATION_PLAN.md`. The git push after each iteration is a no-op if no
commits were made (we check `git status --porcelain` first).

### 3.3 `ralph loop [path]`

Run the canonical build loop: one task per iteration, commit + push per pass.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | false | Emit JSON status events |
| `--model` | string | `opus` | `opus` or `sonnet` |
| `--max-iterations` | int | `50` | Hard cap; use `--no-cap` for unlimited |
| `--no-cap` | bool | false | Unlimited iterations (DANGEROUS — confirm interactively) |
| `--no-push` | bool | false | Skip git push after each iteration |
| `--no-commit` | bool | false | Skip git commit (iterate without saving — debug only) |
| `--reverse` | bool | false | Brownfield mode: first run `PROMPT_reverse_engineer_specs.md` to generate specs, then proceed with build |

**Pre-conditions:** same as `plan` (exit 4, 5, 6 codes).

**Behavior:** runs the loop driver (§7) with `PROMPT_build.md` as the prompt
and mode=build. After each iteration:
- If `--no-commit` is not set: stage all changes in the project root (NOT in
  `.ralph/`), commit with message from Claude Code's output
- If `--no-push` is not set AND a commit was made: `git push origin <branch>`

**`--reverse` semantics:** before the first build iteration, run one
`PROMPT_reverse_engineer_specs.md` iteration to generate `specs/*`. This
single reverse iteration does NOT commit code; it only writes specs. The
build loop starts at iteration 0 after the reverse pass.

### 3.4 `ralph status [path]`

Print current state. Default: human-readable. `--json`: machine-readable.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | false | Emit as JSON (see §9 schema) |

**Pre-conditions:** none (works on a fresh init too).

**Human-readable output (default):**

```
Project: <basename of path>
Harness: claude (v1.0.30)
Loop count: 17
Plan tasks: 4 remaining, 12 complete
Last commit: abc1234 (Add user auth)
Last session: ses_xyz
Plan hash: stable (12 iterations unchanged)
.ralph/ size: 24K
```

**JSON output:** see §9 for schema.

### 3.5 `ralph abort [path]`

Signal the running loop to exit gracefully at the end of the current iteration.

**Behavior:**
1. Touch `.ralph/ABORT_REQUESTED` (empty file)
2. Print: `Abort requested. Loop will exit after the current iteration.`

**Equivalent to:** sending `SIGINT` to the loop process. The sentinel file is
useful for control-plane aborts (e.g. a Paperclip adapter that doesn't have
PID access).

### 3.6 `ralph --version`

Print: `ralph <version> (<commit>, <date>)` where version is injected via
`-ldflags "-X github.com/klampa/ralph-cli/internal/version.Version=vX.Y.Z"`.

### 3.7 `ralph --help`

Standard cobra auto-generated help.

### 3.8 `ralph uninstall` (deferred to v0.2)

Not in v0.1. Documented as "manual uninstall: `rm ~/.local/bin/ralph`."

---

## 4. `.ralph/` directory layout

Created by `ralph init`. The user sees a clean project root; Ralph state is
hidden.

```
.ralph/
├── PROMPT_build.md                          # copied from embedded template
├── PROMPT_plan.md                           # "
├── PROMPT_reverse_engineer_specs.md         # "
├── AGENTS.md                                # generated minimal template
├── IMPLEMENTATION_PLAN.md                   # empty template
├── loop.sh                                  # copied, chmod 0755
├── ralph.json                               # state file (see §10)
├── ABORT_REQUESTED                          # sentinel; absent = no abort
└── sessions/                                # empty; reserved for v0.2
```

**`.gitignore`:** `ralph init` appends `.ralph/` to `.gitignore` by default
(creates the file if missing). The user may remove this entry to commit
template files for team-wide consistency. `ralph.json` and `sessions/` should
NEVER be committed — the user can add explicit exceptions:
`!/.ralph/PROMPT_*.md` to commit prompts but not state.

**`ABORT_REQUESTED` semantics:** empty file. Presence = abort requested.
The loop driver checks `os.Stat(".ralph/ABORT_REQUESTED")` at the top of each
iteration. The loop completes the current iteration cleanly, then exits with
code 2 (aborted).

---

## 5. Embedded templates (`embed.FS`)

The Go binary bundles 6 template files via `embed.FS`. These are copied
verbatim from `ClaytonFarr/ralph-playbook/files/`.

**Bundle strategy:**
```go
//go:embed templates/*.md templates/loop.sh templates/AGENTS.md
var templatesFS embed.FS
```

**Files to bundle (in `internal/scaffold/templates/`):**

| Source (in cloned playbook) | Destination (in .ralph/) | Chmod |
|------------------------------|---------------------------|-------|
| `files/PROMPT_build.md` | `PROMPT_build.md` | 0644 |
| `files/PROMPT_plan.md` | `PROMPT_plan.md` | 0644 |
| `files/PROMPT_reverse_engineer_specs.md` | `PROMPT_reverse_engineer_specs.md` | 0644 |
| `files/AGENTS.md` | `AGENTS.md` | 0644 |
| `files/IMPLEMENTATION_PLAN.md` | `IMPLEMENTATION_PLAN.md` | 0644 |
| `files/loop.sh` | `loop.sh` | 0755 |

**`PROMPT_specs.md` is NOT bundled** — it's an audit prompt the user invokes
manually, not part of the canonical loop. Document its existence in the
README but don't ship it.

**`parse_stream.js` and `loop_streamed.sh` are NOT bundled** — they're
optional colorizer / streaming variants. Defer to v0.2 (probably never;
`--output-format=stream-json` makes the JS obsolete).

**Versioning:** templates are versioned with the binary. The CLI's `Version`
constant matches the template bundle's expected version. `ralph init --force`
overwrites the user's local copies with the latest. The user can `diff` their
edits against the bundle manually if needed.

**Licensing:** the bundled templates are MIT-licensed (per
`ClaytonFarr/ralph-playbook/LICENSE`). Include a `NOTICE` file in the repo
attributing Clayton Farr and Geoff Huntley.

---

## 6. Claude Code invocation contract (the heart of the CLI)

This is the contract that makes the loop autonomous. The CLI invokes Claude
Code with a **fixed argv** and a **piped stdin prompt**. Any change to these
flags is a breaking change to the contract.

### 6.1 The argv (v0.1, Claude Code v1.0+)

```go
argv := []string{
    "claude",
    "-p",                                                  // headless
    "--dangerously-skip-permissions",                      // YOLO mode (sandbox required)
    "--output-format=stream-json",                         // structured, parseable
    "--include-partial-messages",                          // live streaming (for --json)
    "--model", modelFlag,                                  // "opus" or "sonnet"
    "--verbose",                                           // detailed execution log
}
```

**Notes:**
- The order does not matter to Claude Code, but keep it stable for tests
- `--dangerously-skip-permissions` is **mandatory** for autonomy. Without it,
  the loop hangs on every tool-call approval prompt
- `--output-format=stream-json` produces newline-delimited JSON events on
  stdout. The CLI can pipe these through to its own `--json` output
- `--include-partial-messages` enables live tool-call streaming. The CLI can
  choose to ignore partial messages and only emit final events, OR relay them
  as `event: tool_call_partial` to its own output

### 6.2 The environment

The CLI sets these env vars on the `claude` process:

| Var | Value | Purpose |
|-----|-------|---------|
| `CLAUDE_CODE_ENTRYPOINT` | `ralph-cli` | Telemetry attribution |
| `ANTHROPIC_API_KEY` | (inherited) | Pass through; user must have it set |
| `RALPH_PROJECT_ROOT` | (absolute path) | Lets the harness know where `.ralph/` lives |

All other env vars are inherited from the parent shell. The CLI does NOT mask
secrets, does NOT inject API keys — that's the user's job (or a future
`secrets-helper` integration).

### 6.3 The stdin (the prompt)

The prompt passed to Claude Code is built as:

```
<contents of .ralph/PROMPT_<mode>.md>

---
## Ralph State

Project root: <absolute path>
Harness: claude
Loop iteration: <N>
Loop count: <loop_count from ralph.json>
Last commit: <sha or "none">
Last session: <id or "none">

---
## Additional Context

### specs/
<concatenation of all specs/*.md, each preceded by "## <filename>">

### IMPLEMENTATION_PLAN.md
<contents>

### AGENTS.md
<contents>
```

The concatenation is done by the CLI (Go) before piping to Claude Code's
stdin. Total prompt size is reported in `ralph status` and warned if > 50K
chars (rough heuristic for "you're past the smart zone").

### 6.4 Exit handling

- Claude Code exits 0: continue the loop (commit, push, next iteration)
- Claude Code exits non-zero: exit the loop with code 1 (generic failure);
  the stderr is captured and printed
- The CLI does NOT retry failed Claude Code invocations (a failed iteration
  is a signal that the plan is wrong; the user should investigate)
- If the user wants retry, that's a v0.2 feature (or a wrapper script)

### 6.5 Future harness flag

The `--harness <name>` flag is parsed in v0.1 but only `claude` is accepted.
Any other value exits 6 (config-invalid) with a helpful message: "only
`claude` is supported in v0.1; `--harness <name>` is reserved for future
use."

**Implementation hint:** the harness is a Go interface:

```go
type Harness interface {
    Name() string
    Invoke(ctx context.Context, req InvokeRequest) error
    Version() (string, error)  // for status output
}

type InvokeRequest struct {
    Prompt         string
    Model          string  // "opus" | "sonnet"
    ProjectRoot    string
    IterationCount int
    LoopCount      int
}
```

In v0.1, only `claude` is registered. The dispatcher is a `map[string]Harness`
so adding `pi` later is one line.

---

## 7. Loop semantics

The loop driver is the same for `plan`, `loop`, and `loop --reverse` (modulo
the prompt file and the `--reverse` pre-pass).

### 7.1 Pseudocode

```go
func (l *Loop) Run(ctx context.Context) error {
    // 0. Pre-flight: harness exists, .ralph/ exists, git repo
    if err := l.preflight(); err != nil { return err }

    // 0.5. --reverse pre-pass: generate specs before build
    if l.mode == ModeBuild && l.reverse {
        if err := l.runOneIteration(ctx, ModeReverse, 0); err != nil { return err }
    }

    // 1. Determine iteration count
    iter := 0
    for {
        // 2. Abort check
        if l.abortRequested() {
            return ErrAborted  // exit code 2
        }

        // 3. Hard cap
        if !l.noCap && l.maxIterations > 0 && iter >= l.maxIterations {
            return nil  // exit 0 — clean completion
        }

        // 4. Run one iteration
        if err := l.runOneIteration(ctx, l.mode, iter); err != nil {
            return err  // exit 1
        }

        // 5. Commit + push (build mode only, unless --no-commit / --no-push)
        if l.mode == ModeBuild {
            if !l.noCommit { l.gitCommit() }
            if !l.noPush && l.justCommitted() { l.gitPush() }
        }

        // 6. Update state
        l.state.LoopCount++
        l.state.LastCommit = l.lastCommitSHA()
        l.state.LastSessionID = l.lastSessionID()
        l.state.Save()

        // 7. Emit status event
        l.emitter.Emit(IterationCompleteEvent{...})

        iter++
    }
}
```

### 7.2 The single-iteration hot path

```go
func (l *Loop) runOneIteration(ctx context.Context, mode Mode, iter int) error {
    promptPath := l.promptPathFor(mode)  // .ralph/PROMPT_<mode>.md
    promptContent := buildPrompt(promptPath, l.state, l.projectRoot)

    cmd := exec.CommandContext(ctx, "claude", argvForMode(mode, l.model)...)
    cmd.Stdin  = strings.NewReader(promptContent)
    cmd.Stdout = os.Stdout  // pass-through (or pipe to JSON parser if --json)
    cmd.Stderr = os.Stderr
    cmd.Env    = append(os.Environ(),
        "CLAUDE_CODE_ENTRYPOINT=ralph-cli",
        fmt.Sprintf("RALPH_PROJECT_ROOT=%s", l.projectRoot))

    return cmd.Run()
}
```

### 7.3 Git operations (best-effort, never block)

- `gitCommit()`: `git add -A && git commit -m "<message from claude output>"`
  — if the message is empty, use "ralph: iteration <N>"
- `gitPush()`: `git push origin $(git branch --show-current) || git push -u origin HEAD`
  — matches canonical `loop.sh` behavior. If push fails, print warning but
  continue (the next iteration's commit will push the previous one too)
- The CLI never force-pushes. Never. Even with `--force` on `init`.

### 7.4 Concurrency

**Strictly single-threaded.** One Claude Code process at a time. The "parallel
subagents" are spawned INSIDE Claude Code via its own tools, not by the CLI.
The CLI is a loop driver, not an orchestrator.

If a user wants parallel Ralphs, that's `tmux` / `docker compose` /
multi-CLI-instance territory — not a v0.1 feature.

---

## 8. Exit codes (the contract for shell scripts and CI)

| Code | Constant | Meaning | When |
|------|----------|---------|------|
| 0 | `ExitOK` | Success | All iterations completed (or single-shot plan completed) |
| 1 | `ExitGeneric` | Generic failure | Claude Code non-zero exit, git failure, unexpected error |
| 2 | `ExitAborted` | Aborted by user | `ralph abort` or SIGINT during loop |
| 3 | `ExitInitFail` | Init failure | `init` could not create `.ralph/` (permission denied, disk full, path not directory) |
| 4 | `ExitHarnessMissing` | Harness not found | `claude` binary not on PATH |
| 5 | `ExitNotInGit` | Not a git repo | Loop/plan/init (when required) and no `.git/` found |
| 6 | `ExitConfigInvalid` | Config invalid | `ralph.json` malformed, `--harness <other>` passed, `.ralph/` missing when required |

**Reserved for v0.2:** 10+ for additional harness-specific errors.

---

## 9. JSON status event schema (the contract for downstream consumers)

When `--json` is passed, the CLI emits newline-delimited JSON (NDJSON) events
to stdout. Each event is a single line. Stderr is unchanged (used for human
warnings).

### 9.1 Event envelope

```json
{
  "ts": "2026-06-02T03:00:00.123Z",        // ISO 8601 UTC
  "level": "info",                          // "info" | "warn" | "error"
  "event": "iteration.start",               // event type
  "session_id": "ses_xyz",                  // unique per CLI invocation (ULID)
  "project": "/abs/path/to/project",        // absolute project root
  "data": { ... }                           // event-specific payload
}
```

### 9.2 Event types

| `event` field | When | `data` payload |
|---------------|------|----------------|
| `run.start` | CLI invocation starts | `{ "command": "loop", "mode": "build", "model": "opus", "max_iterations": 50 }` |
| `run.end` | CLI invocation ends (any reason) | `{ "exit_code": 0, "iterations": 17, "duration_s": 3600, "reason": "completed" \| "aborted" \| "max_iterations" \| "error" }` |
| `iteration.start` | Before each Claude Code invocation | `{ "iteration": 0, "mode": "build" }` |
| `iteration.end` | After each Claude Code invocation | `{ "iteration": 0, "duration_s": 120, "exit_code": 0, "commit_sha": "abc1234" \| null }` |
| `tool_call` | Tool call from Claude Code (parsed from stream-json) | `{ "tool": "Read", "input_summary": "Read .ralph/PROMPT_build.md" }` |
| `commit` | A commit was made | `{ "sha": "abc1234", "message": "Add user auth", "files_changed": 3 }` |
| `push` | A push was made | `{ "remote": "origin", "branch": "main", "sha": "abc1234" }` |
| `push.skipped` | A push was intentionally not executed (e.g. no origin remote); audit event for traceability | `{ "reason": "no_origin_remote" }` |
| `abort.requested` | Sentinel file detected | `{ "sentinel": ".ralph/ABORT_REQUESTED" }` |
| `error` | An error occurred | `{ "code": "harness_missing", "message": "claude not found on PATH" }` |

### 9.3 Example stream

```jsonl
{"ts":"2026-06-02T03:00:00.000Z","level":"info","event":"run.start","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"command":"loop","mode":"build","model":"opus","max_iterations":50}}
{"ts":"2026-06-02T03:00:00.100Z","level":"info","event":"iteration.start","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"iteration":0,"mode":"build"}}
{"ts":"2026-06-02T03:00:05.000Z","level":"info","event":"tool_call","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"tool":"Read","input_summary":"Read .ralph/PROMPT_build.md"}}
{"ts":"2026-06-02T03:02:30.000Z","level":"info","event":"iteration.end","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"iteration":0,"duration_s":150,"exit_code":0,"commit_sha":"abc1234"}}
{"ts":"2026-06-02T03:02:30.500Z","level":"info","event":"commit","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"sha":"abc1234","message":"Add login form","files_changed":2}}
{"ts":"2026-06-02T03:02:31.000Z","level":"info","event":"push","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"remote":"origin","branch":"main","sha":"abc1234"}}
```

### 9.4 Stable consumer contract

The following fields are **stable** (will not change in v0.x):
- `ts`, `level`, `event`, `session_id`, `project`
- All `data` field names listed in §9.2

Downstream consumers (Paperclip adapter, CI integrations) can pin to these.

---

## 10. State file: `.ralph/ralph.json`

The CLI's single source of mutable state. Schema is versioned for forward
compatibility.

```json
{
  "schema_version": 1,
  "harness": "claude",
  "ralph_version": "0.1.0",
  "created_at": "2026-06-02T03:00:00.000Z",
  "loop_count": 17,
  "last_commit": "abc1234567890abcdef1234567890abcdef12345",
  "last_session_id": "ses_01HXY...",
  "last_run_at": "2026-06-02T03:02:31.000Z",
  "last_run_duration_s": 150,
  "plan_hash": "sha256:abc123...",
  "config": {
    "max_iterations": 50,
    "model": "opus",
    "push": true
  }
}
```

**Schema migrations:** the CLI reads `schema_version` and applies migrations
sequentially. v0.1 only handles `schema_version: 1`. Future versions add
migrations in `internal/state/migrations.go`.

**Atomic writes:** the CLI writes to `ralph.json.tmp` then `os.Rename` to
`ralph.json` to avoid torn writes on crash.

**Concurrency:** the CLI assumes a single process touches the file. If two
`ralph` processes run in the same project, they will clobber each other's
state. This is a documented v0.1 limitation; v0.2 may add a `flock`.

---

## 11. Distribution

### 11.1 v0.1

- **Source build:** `git clone` + `go build -o ralph ./cmd/ralph` → single
  static binary
- **`go install`:** `go install github.com/klampa/ralph-cli/cmd/ralph@v0.1.0`
  (works once the repo is public on GitHub)
- **Release artifacts:** GitHub Releases with binaries for
  `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64` (built via
  `goreleaser` or a simple GoReleaser config; Elmo's call)

### 11.2 v0.2+ (deferred)

- **Homebrew tap:** `klampa/tap` with formula `ralph`
- **Install script:** `curl -sSf https://klampa.dev/ralph/install.sh | sh`
  (writes to `~/.local/bin/ralph`)
- **Docker image:** `klampa/ralph-cli` (just bundles the binary + a
  `claude` install path; sandboxing is the user's responsibility)

### 11.3 NOT shipping

- **npm package** — system CLIs do not belong on npm
- **PyPI package** — same reason
- **crates.io** — same reason
- **Snap/Flatpak/AppImage** — over-engineered for v0.1

---

## 12. v0.1 scope decisions (the 8 open questions answered)

These resolve the open questions Ernie queued in `RESEARCH.md §9`.

| # | Question | Decision |
|---|----------|----------|
| 1 | **License** | **MIT** (matches `ClaytonFarr/ralph-playbook`, broadest compatibility). Add `NOTICE` attributing Clayton Farr + Geoff Huntley. |
| 2 | **Repo location** | `github.com/klampa/ralph-cli`. Confirm with Kyle before Elmo creates the repo. |
| 3 | **v0.1 scope** | **Include `--reverse` in v0.1.** It's a single prompt swap + a one-iteration pre-pass. Low cost, high value (brownfield use case is real). Defer TUI/sandbox manager. |
| 4 | **`ralph doctor`** | **Defer to v0.2.** Reserve the `internal/doctor` package (see §2.2) so the name doesn't churn. |
| 5 | **`--harness` flag** | **Stub it in v0.1.** Accepts only `claude`; any other value exits 6. Documents the door is open. |
| 6 | **Homebrew tap** | **Defer to v0.2.** v0.1 ships `go install` + release binaries. |
| 7 | **Telemetry** | **Defer to v0.2.** v0.1 emits anonymous-ish data via the `--json` status events (§9), but does not phone home. If/when telemetry is added, it must be opt-in with a clear toggle. |
| 8 | **Test fixtures** | **Ship a minimal `testdata/smoke/`** fixture in v0.1: one `specs/01-hello.md`, one `src/hello.go`, one `src/hello_test.go`. Used by integration tests. |

---

## 13. v0.2+ deferrals (so we don't lose track)

These are explicitly OUT of v0.1. The SPEC reserves package names and config
keys for them where it makes sense.

- `ralph doctor` — sandbox detection, schema check, AGENTS.md size warning
- `ralph uninstall` — clean removal of binary + config
- `--harness pi|amp|codex|opencode` — multi-harness routing
- Homebrew tap + curl|sh installer
- Telemetry (opt-in, anonymous)
- `flock`-based concurrent run protection
- Cost / quota tracking in `ralph status`
- `PROMPT_specs.md` and `parse_stream.js` bundling
- TUI / web dashboard
- Schema validation for `prd.json`

---

## 14. Test strategy

### 14.1 Unit tests (always run)

- `internal/scaffold` — embed.FS extraction, file copy, chmod, dir creation.
  Use `t.TempDir()` for isolation. No mocks needed (pure file ops).
- `internal/state` — read/write/atomic-rename, schema migration. Use
  `t.TempDir()`. Mock `time.Now` for `created_at` determinism.
- `internal/harness` — mock the `os/exec` interface. Inject a fake binary
  that records its argv + stdin. Assert exact contract (§6.1).
- `internal/events` — event emission, NDJSON format, envelope validation.
- `internal/loop` — iteration driver. Mock the harness to return scripted
  exit codes. Test: max-iterations cap, abort sentinel, commit/push flow.

### 14.2 Integration tests (gated by `-tags=integration`)

```
go test -tags=integration ./...
```

- `testdata/smoke/` — minimal project. Run `ralph init`, verify all 6
  template files exist, verify `ralph.json` schema, verify `.gitignore`
  updated.
- Stub `claude` binary on PATH (via `t.Setenv("PATH", ...)`). Assert the
  exact argv and stdin the CLI passes to the harness. Verify state file
  updates.
- Use `t.Setenv("PATH", ...)` and a `testdata/bin/fake-claude` shell
  script. The fake prints "ok" and exits 0; the test asserts the CLI
  treated that as success.

### 14.3 End-to-end smoke (Grover's Task 4, not in Elmo's scope)

- Run `ralph init /tmp/smoke && cd /tmp/smoke && ralph plan 1` with the
  fake `claude`. Verify the loop calls the binary with the expected args
  and produces the expected `.ralph/` state changes.

### 14.4 Coverage target

- v0.1: 70% line coverage on `internal/` packages
- All exported functions have a test
- All exit codes have at least one test that triggers them

### 14.5 CI

GitHub Actions on push + PR:

```yaml
name: test
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: go test ./...
      - run: go test -tags=integration ./...
      - run: go vet ./...
      - run: go build -o ralph ./cmd/ralph
      - run: ./ralph --version
```

---

## 15. Elmo's implementation task breakdown (Task 3)

Order matters — each step unblocks the next.

### Step 1: Scaffold + version (30 min)

- `go mod init github.com/klampa/ralph-cli`
- `mkdir -p cmd/ralph internal/{cli,scaffold,harness,loop,state,events,version,doctor} testdata/{smoke/specs,smoke/src,bin}`
- `internal/version/version.go` with `Version`, `Commit`, `Date` vars
- `cmd/ralph/main.go` — minimal `fmt.Println(Version)` to verify build
- `go build -o ralph ./cmd/ralph && ./ralph --version` (via `-ldflags`)
- Commit: `chore: scaffold go module + version package`

### Step 2: Cobra root + subcommand stubs (45 min)

- `cobra init` or hand-rolled: `internal/cli/root.go` with the 5
  subcommands registered
- Each subcommand has a placeholder `Run` that prints its name
- `cmd/ralph/main.go` calls `cli.Execute()`
- `./ralph --help` shows all 5 subcommands
- `./ralph init --help` shows init's flags
- Commit: `feat: cobra subcommand skeleton`

### Step 3: Bundle templates via embed.FS (30 min)

- Clone `ClaytonFarr/ralph-playbook` (Ernie already cloned it to
  `/tmp/ralph-playbook` — copy `files/*` into `internal/scaffold/templates/`)
- `internal/scaffold/templates.go` exposes `TemplatesFS embed.FS`
- Unit test: extract all 6 files, verify content matches source
- Commit: `feat: bundle 6 canonical templates via embed.FS`

### Step 4: `ralph init` (1.5 hr)

- Implement `internal/scaffold/scaffold.go` — `Scaffold(path, opts) error`
- Implement `internal/cli/init.go` — flag parsing + call scaffold
- Implement pre-conditions: dir exists, has `.git/`
- Implement `.gitignore` append
- Implement `ralph.json` initial write
- Unit tests: scaffold a temp dir, verify all files
- Integration test: `testdata/smoke/` fixture
- Manual test: `ralph init ~/projects/ralph-cli-test/ && ls -la ~/projects/ralph-cli-test/.ralph/`
- Commit: `feat: ralph init scaffolds .ralph/ with all 6 templates + state`

### Step 5: Harness interface + ClaudeCode (1 hr)

- `internal/harness/harness.go` — interface definition
- `internal/harness/claude.go` — ClaudeCode implementation with the
  exact argv from §6.1
- Mock harness for tests (records argv + stdin)
- Unit tests: assert exact argv, env vars, stdin format
- Commit: `feat: Harness interface + ClaudeCode impl with exact argv contract`

### Step 6: State + events (1 hr)

- `internal/state/state.go` — `State` struct, `Load`, `Save`, atomic write
- `internal/events/events.go` — `Emitter` with `Emit(event)` → NDJSON
- Implement `schema_version: 1` and a stub `migrate(v int) State` that
  errors on unknown versions
- Unit tests: round-trip state, atomic write, NDJSON format
- Commit: `feat: state (ralph.json) + events (NDJSON emitter)`

### Step 7: Loop driver (1.5 hr)

- `internal/loop/loop.go` — iteration driver per §7
- Implement abort sentinel check
- Implement max-iterations cap
- Implement git commit + push (best-effort)
- Unit tests with mock harness: full loop, abort mid-loop, max-iter hit,
  Claude failure
- Commit: `feat: loop driver with abort sentinel, max-iter, git ops`

### Step 8: `ralph plan` + `ralph loop` + `ralph loop --reverse` (1 hr)

- Wire the loop driver into the cobra subcommands
- `--reverse` pre-pass
- Test the `--json` event stream
- Commit: `feat: ralph plan + ralph loop + --reverse`

### Step 9: `ralph status` + `ralph abort` (30 min)

- `internal/cli/status.go` — read state, print human-readable or JSON
- `internal/cli/abort.go` — touch sentinel file
- Unit tests: both human and JSON output formats
- Commit: `feat: ralph status + ralph abort`

### Step 10: Integration tests + CI (1 hr)

- `testdata/smoke/` fixture (the minimal project)
- `testdata/bin/fake-claude` shell script
- Integration tests gated by `-tags=integration`
- GitHub Actions workflow (`.github/workflows/test.yml`)
- Commit: `test: integration smoke + CI`

### Step 11: README + LICENSE + NOTICE (30 min)

- `README.md` — install, quickstart, command reference, sandbox warning
- `LICENSE` — MIT
- `NOTICE` — attribute Clayton Farr + Geoff Huntley for the bundled templates
- Commit: `docs: README + LICENSE + NOTICE`

### Step 12: Release v0.1.0 (30 min)

- Tag `v0.1.0`
- `go build` for all 4 targets → attach to GitHub Release
- Verify `go install github.com/klampa/ralph-cli/cmd/ralph@v0.1.0` works
- Commit `elmo.json` to `~/squad-output/ralph-cli/`
- Post completion to #operations
- Run `bash /tmp/squad-completion-elmo.sh`

**Total estimate: ~10 hours of focused work.** The longest single step is
`ralph init` (scaffold + state + gitignore) — the rest are mostly wiring
the contracts this SPEC pins down.

---

## 16. What this SPEC deliberately does NOT define

- **Claude Code's behavior** — that's Anthropic's contract, not ours
- **The content of `PROMPT_*.md`** — bundled verbatim from the playbook;
  users edit locally
- **The `prd.json` schema** — informal, let Ralph write what it wants
- **The Ralph philosophy** — Geoff + Clayton's; we package it, not define it
- **Sandbox management** — out of scope for v0.1; document the requirement

---

## 17. Contract summary (the one-page version for Elmo)

| Contract | Value |
|----------|-------|
| Language | Go 1.22+ |
| Module | `github.com/klampa/ralph-cli` |
| Entry | `cmd/ralph/main.go` |
| Subcommands | `init`, `plan`, `loop`, `status`, `abort` |
| `.ralph/` files | 6 templates + `ralph.json` + `ABORT_REQUESTED` + `sessions/` |
| Bundled templates | `PROMPT_build.md`, `PROMPT_plan.md`, `PROMPT_reverse_engineer_specs.md`, `AGENTS.md`, `IMPLEMENTATION_PLAN.md`, `loop.sh` |
| Harness | `claude` only (v0.1) |
| Claude argv | `claude -p --dangerously-skip-permissions --output-format=stream-json --include-partial-messages --model <opus\|sonnet> --verbose` |
| Claude env | `CLAUDE_CODE_ENTRYPOINT=ralph-cli`, `RALPH_PROJECT_ROOT=<abs path>` |
| Loop driver | One Claude Code process per iteration; commit + push after each |
| Abort | `.ralph/ABORT_REQUESTED` sentinel; checked at top of each iteration |
| Exit codes | 0 ok, 1 generic, 2 aborted, 3 init-fail, 4 harness-missing, 5 not-in-git, 6 config-invalid |
| JSON events | NDJSON to stdout when `--json`; see §9 for schema |
| State file | `.ralph/ralph.json` schema_version: 1; atomic write |
| Max iter default | 50 (override with `--no-cap`) |
| Distribution | `go install` + GitHub Releases (v0.1); Homebrew + curl|sh (v0.2) |
| License | MIT + NOTICE |
| Test gate | `go test ./...` always; `-tags=integration` for full suite |

---

**End of SPEC.** Elmo: §15 is your step-by-step; §17 is your cheat sheet. Grover: the README in step 11 is yours to polish (Task 4). The contracts in §6, §8, §9 are the load-bearing ones — don't break them.
