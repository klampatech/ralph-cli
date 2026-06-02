# ralph

> Run a coding agent in a continuous, one-task-per-iteration loop. Single static
> Go binary. Claude Code under the hood. File-based shared state, no daemon,
> no orchestrator.

`ralph` wraps [ClaytonFarr/ralph-playbook](https://github.com/ClaytonFarr/ralph-playbook)'s
canonical Ralph file templates into a hidden `.ralph/` scaffold at any project
root, then drives Claude Code in a loop: one task, one commit, one push, fresh
context, repeat.

## What is Ralph?

Ralph is a methodology coined by [Geoff Huntley](https://ghuntley.com/) in 2025
for running a coding agent **continuously and autonomously** against a small
set of deterministic files (`PROMPT.md`, `AGENTS.md`, `specs/*`,
`IMPLEMENTATION_PLAN.md`). Each iteration does one unit of work, commits, and
exits. A bash loop restarts the agent with a fresh context window. The result
is a long, predictable, fully-observable autonomous run.

Clayton Farr's [playbook](https://github.com/ClaytonFarr/ralph-playbook)
captures the philosophy; `ralph-cli` packages it into a binary you can
`go install` and run anywhere.

> _"I'm in danger!"_ — Ralph Wiggum, cheerfully unaware of how fragile this
> somehow works.

## Install

```bash
go install github.com/klampa/ralph-cli/cmd/ralph@v0.1.0
```

Or build from source:

```bash
git clone https://github.com/klampa/ralph-cli
cd ralph-cli
go build -o ralph ./cmd/ralph
mv ralph ~/.local/bin/   # somewhere on $PATH
```

Release binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, and
`linux/arm64` are attached to the [GitHub Releases](https://github.com/klampa/ralph-cli/releases)
page — download, `chmod +x`, drop on `$PATH`.

> **Homebrew, `curl | sh`, and Docker image:** deferred to v0.2. `go install`
> is the canonical v0.1 path.

## 30-second quickstart

### Greenfield (new project)

```bash
mkdir ~/projects/cool-app && cd ~/projects/cool-app
git init
ralph init
# → creates .ralph/ with 6 templates + ralph.json
# → appends .ralph/ to .gitignore
# write specs/01-foo.md, specs/02-bar.md by hand (or via ralph plan)
ralph plan 1                    # one-shot gap analysis, no code
ralph loop                      # autonomous build loop, capped at 50 iterations
```

### Brownfield (existing code, no specs)

```bash
cd ~/projects/legacy-app       # existing code, already a git repo
ralph init
ralph loop --reverse           # generate specs from src/, then build
```

That's it. `ralph` owns `.ralph/`, the loop, and the JSON status stream. Your
project root stays clean.

## Commands

```
ralph <command> [path] [flags]
```

All commands accept an optional positional `[path]` (default: `.`).

| Command | What it does |
|---|---|
| `ralph init [path]` | Scaffold `.ralph/` at the project root. Bundles 6 template files, writes `ralph.json`, adds `.ralph/` to `.gitignore`. |
| `ralph plan [path]` | Run the planning loop: gap analysis, produce `IMPLEMENTATION_PLAN.md` from `specs/`. Plan mode does NOT commit code. |
| `ralph loop [path]` | Run the canonical build loop: one task per iteration, commit + push per pass. Default cap: 50 iterations. |
| `ralph loop --reverse` | Brownfield mode: first run `PROMPT_reverse_engineer_specs.md` to generate specs, then proceed with build. |
| `ralph status [path]` | Print current state. Default: human-readable. `--json` for machine-readable. |
| `ralph abort [path]` | Signal the running loop to exit gracefully at the end of the current iteration. Equivalent to SIGINT. |

### Common flags

| Flag | Applies to | Default | Notes |
|---|---|---|---|
| `--json` | `plan`, `loop`, `status` | `false` | Emit NDJSON status events to stdout |
| `--model opus\|sonnet` | `plan`, `loop` | `opus` | Forwarded to Claude Code |
| `--max-iterations N` | `plan`, `loop` | `1` / `50` | Hard cap. `--no-cap` for unlimited. |
| `--no-push` | `loop` | `false` | Skip `git push` after each iteration |
| `--no-commit` | `loop` | `false` | Skip `git commit` (debug only) |
| `--force` | `init` | `false` | Overwrite existing `.ralph/` files |
| `--reset-state` | `init` (with `--force`) | `false` | Also overwrite `ralph.json` |
| `--harness <name>` | `init` | `claude` | v0.1: only `claude` accepted; any other value exits 6 |
| `--no-templates` | `init` | `false` | Skip copying the 6 template files |
| `--no-gitignore` | `init` | `false` | Don't add `.ralph/` to `.gitignore` |

## Configuration

State lives in **`.ralph/ralph.json`** (created by `ralph init`):

```json
{
  "schema_version": 1,
  "harness": "claude",
  "ralph_version": "0.1.0",
  "created_at": "2026-06-02T03:00:00.000Z",
  "loop_count": 17,
  "last_commit": "abc1234",
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

### Bundled templates (6 files in `.ralph/`)

| Source (ClaytonFarr/ralph-playbook) | Destination | Chmod |
|---|---|---|
| `files/PROMPT_build.md` | `PROMPT_build.md` | `0644` |
| `files/PROMPT_plan.md` | `PROMPT_plan.md` | `0644` |
| `files/PROMPT_reverse_engineer_specs.md` | `PROMPT_reverse_engineer_specs.md` | `0644` |
| `files/AGENTS.md` | `AGENTS.md` | `0644` |
| `files/IMPLEMENTATION_PLAN.md` | `IMPLEMENTATION_PLAN.md` | `0644` |
| `files/loop.sh` | `loop.sh` | `0755` |

Edit any of these locally — the CLI uses your copy. `ralph init --force`
overwrites with the latest bundled defaults.

### `.gitignore`

`ralph init` appends `.ralph/` to `.gitignore` by default (creates the file
if missing). The user may remove the entry to commit template files for
team-wide consistency:

```gitignore
# default: ignore all of .ralph/
.ralph/

# optional: commit prompts, ignore state
!.ralph/PROMPT_*.md
!.ralph/AGENTS.md
!.ralph/IMPLEMENTATION_PLAN.md
!.ralph/loop.sh
```

`ralph.json` and `sessions/` should **never** be committed.

## How the loop works

Each iteration: the CLI builds a prompt from `.ralph/PROMPT_<mode>.md` plus
`specs/*`, `IMPLEMENTATION_PLAN.md`, `AGENTS.md`, and current state. It pipes
the prompt to Claude Code in headless mode with a fixed argv. Claude does one
unit of work, writes the commit message to its output, and exits. The CLI
commits with that message and pushes to the current branch. Next iteration
starts with a fresh context window.

```
   ┌────────────────────────────────────┐
   │  .ralph/PROMPT_build.md            │
   │  + specs/*  + AGENTS.md           │
   │  + IMPLEMENTATION_PLAN.md         │
   │  + ralph.json state               │
   └────────────────┬───────────────────┘
                    │ stdin
                    ▼
        ┌───────────────────────┐
        │  claude -p --dangerously-skip-permissions │
        │  --output-format=stream-json              │
        └───────────────┬───────────────┘
                        │ stdout (commit message)
                        ▼
        ┌───────────────────────┐
        │  git add -A && commit │
        │  git push origin HEAD│
        └───────────────┬───────────────┘
                        │ next iteration (fresh context)
                        ▼
                   (repeat)
```

**One task per iteration, one commit per pass, fresh context every time.** The
"parallel subagents" are spawned *inside* Claude Code via its own tools — the
CLI is single-threaded by design.

## Sandbox requirement

> **Warning:** `ralph loop` invokes Claude Code with
> `--dangerously-skip-permissions`, which **disables all tool-call approval
> prompts**. The loop is fully autonomous. You **must** run it inside a
> sandbox.

`ralph-cli` does not bundle a sandbox manager — pick the one that fits:

- **Docker** (simplest, local dev): `docker run -v $(pwd):/workspace -w /workspace <ralph-image> ralph loop`
- **E2B** (Firecracker microVMs, ~150ms cold start, best for production agents)
- **Sprites / Fly.io** (persistent microVMs for long-running loops)
- **Modal / Cloudflare Sandboxes / exe.dev** (all viable, see
  [the playbook's reference](https://github.com/ClaytonFarr/ralph-playbook/blob/main/references/sandbox-environments.md))

If you're running locally and accept the risk, just `ralph loop` directly —
but understand what that means.

## Exit codes

| Code | Meaning | When |
|---|---|---|
| 0 | Success | All iterations completed, or single-shot plan completed |
| 1 | Generic failure | Claude Code non-zero exit, git failure, unexpected error |
| 2 | Aborted by user | `ralph abort` or SIGINT during loop |
| 3 | Init failure | `init` could not create `.ralph/` (permissions, disk full, not a directory) |
| 4 | Harness not found | `claude` binary not on `PATH` |
| 5 | Not a git repo | `init`/`plan`/`loop` requires `.git/` and didn't find it |
| 6 | Config invalid | `ralph.json` malformed, `--harness <other>` passed, `.ralph/` missing when required |

Stable across v0.x. Safe to wire into CI.

## Limitations (v0.1)

These are explicitly **not** in v0.1. The package layout and config keys
reserve the room for them so v0.2 doesn't churn the tree.

- `ralph doctor` — sandbox detection, schema check, AGENTS.md size warning
- `ralph uninstall` — clean removal of binary + config
- `--harness pi|amp|codex|opencode` — multi-harness routing (the flag is parsed
  in v0.1 but rejects anything other than `claude`)
- Homebrew tap, `curl | sh` installer
- Telemetry (opt-in, anonymous) — v0.1 emits anonymous-ish data only via
  `--json` status events, does not phone home
- `flock`-based concurrent run protection
- Cost / quota tracking in `ralph status`
- `PROMPT_specs.md` and `parse_stream.js` bundling
- TUI / web dashboard
- Schema validation for `prd.json`

## JSON status events

When `--json` is passed, `ralph` emits newline-delimited JSON (NDJSON) to
stdout — one event per line, suitable for piping into `jq`, a log aggregator,
or a downstream governance adapter. Stderr is unchanged.

```jsonl
{"ts":"2026-06-02T03:00:00.000Z","level":"info","event":"run.start","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"command":"loop","mode":"build","model":"opus","max_iterations":50}}
{"ts":"2026-06-02T03:00:00.100Z","level":"info","event":"iteration.start","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"iteration":0,"mode":"build"}}
{"ts":"2026-06-02T03:02:30.000Z","level":"info","event":"iteration.end","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"iteration":0,"duration_s":150,"exit_code":0,"commit_sha":"abc1234"}}
{"ts":"2026-06-02T03:02:30.500Z","level":"info","event":"commit","session_id":"ses_01HXY...","project":"/home/kyle/projects/cool-app","data":{"sha":"abc1234","message":"Add login form","files_changed":2}}
```

Stable fields (`ts`, `level`, `event`, `session_id`, `project`, plus the
`data` keys per event type) will not change in v0.x. Pin to them.

## License

MIT — see [LICENSE](./LICENSE) and [NOTICE](./NOTICE).

Bundled templates (`PROMPT_*.md`, `AGENTS.md`, `IMPLEMENTATION_PLAN.md`,
`loop.sh`) are MIT-licensed by Clayton Farr, sourced from
[ClaytonFarr/ralph-playbook](https://github.com/ClaytonFarr/ralph-playbook).
The Ralph philosophy itself originated with **Geoff Huntley**
([ghuntley.com](https://ghuntley.com/)). See NOTICE for full attribution.
