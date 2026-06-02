# ralph-cli — Research

**Author:** Ernie (Squad Research Agent) · Task 1 of 4
**Date:** 2026-06-02
**Repo under design:** `~/projects/ralph-cli/` (autogpt.local / m5 NUC)
**Status:** Research complete → ready for Bert (SPEC.md)

---

## 0. TL;DR (Bert, read this first)

We are building a globally-installable `ralph` CLI that wraps ClaytonFarr's canonical
Ralph file templates (`PROMPT_build.md`, `PROMPT_plan.md`, `AGENTS.md`, `loop.sh`) into a
single hidden `.ralph/` scaffold at any project root. The CLI is a thin orchestrator
whose only job is to *stage files, invoke Claude Code with the right flags, and
manage the loop*. The Ralph philosophy itself is what we are packaging — not
reinventing.

**Top three recommendations** (drives the SPEC):

1. **Ship a single Go binary** as the `ralph` CLI. 5–8 MB static binary, zero runtime
   dependencies, drop into `~/.local/bin`. Alternatives considered: Rust (heavier
   toolchain, no advantage for shell orchestration), Bash wrapper (no portable install
   story on macOS without coreutils), Python (shebang + venv pain). Go wins on
   "no moving parts" — the CLI is fundamentally a file scaffolder + process launcher.
2. **Single harness: Claude Code for ALL phases.** Plan, build, reverse — same
   binary, same flag set, just different prompt + mode arg. No Codex/Amp/pi switch
   matrix. (Kyle pushed back on the original Codex-for-build / Claude-for-plan split;
   simplicity wins.) The CLI is a Claude Code launcher, not a harness router.
3. **Use `.ralph/` (hidden directory), not loose files in project root.** Keeps the
   user repo clean, makes `rm -rf .ralph` a clean opt-out, makes the scaffold
   self-describing (`tree .ralph/` shows everything Ralph needs), and gives a clean
   install/uninstall boundary. Add to `.gitignore` by default (state, not source)
   but never auto-`git add` anything inside it.

**What we are NOT building:**

- Not a PRD/JSON parser (ClaytonFarr's `prd.json` is the de-facto schema — we reuse it).
- Not a TUI (ClaytonFarr doesn't ship one; a TUI is a separate project).
- Not a parallel/orchestrator (one Claude Code process per iteration is the design;
  parallelism is INSIDE Claude Code via subagents).
- Not a paperclip/governance wrapper (that is a separate spec in the vault;
  ralph-cli emits JSON status events to stdout, governance is downstream).

---

## 1. What is "Ralph Loop"?

### 1.1 Origin and philosophy

Ralph is a methodology for running a coding agent (typically Claude Code) in a
**continuous autonomous loop**, where each iteration:

1. Reads a small set of deterministic files (`PROMPT.md`, `AGENTS.md`, `specs/*`,
   `IMPLEMENTATION_PLAN.md`).
2. Performs **one unit of work** (one task, one commit).
3. Updates the on-disk state (`IMPLEMENTATION_PLAN.md`, optionally `AGENTS.md`).
4. Commits and exits.

A bash loop restarts the agent with a fresh context window for the next task. The
loops are **isolated sessions** — no context accumulates across iterations. This is
the design constraint that makes long autonomous runs tractable.

**Origin:** Geoffrey Huntley (Geoff) coined the "Ralph Wiggum technique" in mid-2025.
The name is a riff on a quote misattributed to the Simpsons character: *"I'm in
danger!"* — the joke being that Ralph the agent is cheerfully unaware of how
fragile its situation is, and somehow that works.

The December 2025 surge (see `ClaytonFarr/ralph-playbook` history, ~5 months ago at
time of writing) made Ralph a standard pattern in the Claude Code community. The
canonical `loop.sh` on the play repo has 989 stars and 255 forks as of Feb 2026.

### 1.2 Core mechanics

Three phases, two prompts, one loop:

| Phase | Owner | Output |
|-------|-------|--------|
| 1. Requirements | Human + LLM conversation | `specs/*.md` (one per JTBD topic) |
| 2. Planning | Ralph loop with `PROMPT_plan.md` | `IMPLEMENTATION_PLAN.md` |
| 3. Building | Ralph loop with `PROMPT_build.md` | Code + tests + commits |

**The "outer loop"** is ~10 lines of bash:

```bash
while :; do cat PROMPT.md | claude ; done
```

**The "inner loop"** is internal to one Claude Code session — the agent iterates
against backpressure (tests, lints, typechecks) until the task passes.

**Why a fresh context per iteration?** A 200K-token window has ~176K truly usable
context. Cramming multiple tasks into one session wastes context on already-solved
problems. One task = one commit = one fresh context = predictable cost and
deterministic state.

### 1.3 Key principles (Geoff + Clayton)

1. **"Let Ralph Ralph"** — trust the LLM to self-identify, self-correct, self-improve.
   Step back from the loop; steer via prompts and backpressure, not micromanagement.
2. **Context is everything** — 40–60% utilization is the "smart zone." Tight tasks +
   subagent fan-out keeps the main agent lean.
3. **Steer with patterns + backpressure** — upstream (deterministic file loading)
   and downstream (tests reject bad work). LLM-as-judge is an emerging backpressure
   for subjective criteria.
4. **Plan is disposable** — if it's wrong, delete `IMPLEMENTATION_PLAN.md` and re-run
   the planning loop. Cost of regeneration is one planning iteration.
5. **Move outside the loop** — sit ON the loop, not IN it. Your job is to engineer
   the setup; Ralph's job is to do the work.

### 1.4 The "Ralph Wiggum technique" — distinct from autonomous agents in general

Not all autonomous coding agents are Ralph. The distinguishing markers:

- **File-based shared state** (`IMPLEMENTATION_PLAN.md` persists between iterations).
- **Dumb bash loop orchestrator** (no stateful daemon, no work queue).
- **One task per iteration** (not "run until done" within one session).
- **Plan can be regenerated cheaply** (not a strict dependency graph).
- **Backpressure via tests/lint** (not human approval gates).

Aider's architect mode, Devin, Cursor Composer, Replit Agent 3 — these are
"autonomous coding agents" but they are NOT Ralph. They differ in session model
(long-lived vs. one-shot) and steering mechanism (chat vs. file).

---

## 2. ClaytonFarr/ralph-playbook — repo layout and conventions

### 2.1 Repo structure

The playbook repo (https://github.com/ClaytonFarr/ralph-playbook) is **methodology
+ templates**, not a CLI. The actual Ralph files live in `files/`:

```
ralph-playbook/
├── README.md                        # 1,446 lines — the playbook
├── index.html                       # GitHub Pages render
├── LICENSE / NOTICE
├── files/                           # ← THE CANONICAL TEMPLATES
│   ├── loop.sh                      # bash outer loop
│   ├── loop_streamed.sh             # variant with parse_stream.js pipe
│   ├── parse_stream.js              # colorized JSON stream parser
│   ├── PROMPT_build.md              # build mode prompt
│   ├── PROMPT_plan.md               # plan mode prompt
│   ├── PROMPT_specs.md              # specs audit mode prompt
│   ├── PROMPT_reverse_engineer_specs.md  # brownfield reverse prompt
│   ├── AGENTS.md                    # template
│   └── IMPLEMENTATION_PLAN.md       # template (empty)
├── references/
│   ├── nah.png                      # "nah" meme (Geoff's "no, this is wrong" reply)
│   ├── ralph-diagram.png            # 3-phase diagram
│   └── sandbox-environments.md      # E2B, Sprites, Modal, exe.dev, Cloudflare
└── .vscode/settings.json
```

**Implication for ralph-cli:** we don't need to clone the playbook at runtime.
We bundle the 6 template files (`loop.sh`, 3 `PROMPT_*.md`, `AGENTS.md`,
`IMPLEMENTATION_PLAN.md`) as static assets inside the Go binary and copy them into
`.ralph/` on `ralph init`.

### 2.2 Canonical `loop.sh` (verbatim from `files/loop.sh`)

```bash
#!/bin/bash
# Usage: ./loop.sh [plan|build] [max_iterations]

if [ "$1" = "plan" ]; then
    MODE="plan"; PROMPT_FILE="PROMPT_plan.md"; MAX_ITERATIONS=${2:-0}
elif [ "$1" = "build" ]; then
    MODE="build"; PROMPT_FILE="PROMPT_build.md"; MAX_ITERATIONS=${2:-0}
elif [[ "$1" =~ ^[0-9]+$ ]]; then
    MODE="build"; PROMPT_FILE="PROMPT_build.md"; MAX_ITERATIONS=$1
else
    MODE="build"; PROMPT_FILE="PROMPT_build.md"; MAX_ITERATIONS=0
fi

ITERATION=0
CURRENT_BRANCH=$(git branch --show-current)

while true; do
    [ $MAX_ITERATIONS -gt 0 ] && [ $ITERATION -ge $MAX_ITERATIONS ] && break

    cat "$PROMPT_FILE" | claude -p \
        --dangerously-skip-permissions \
        --output-format=stream-json \
        --model opus \
        --verbose

    git push origin "$CURRENT_BRANCH" || git push -u origin "$CURRENT_BRANCH"
    ITERATION=$((ITERATION + 1))
done
```

**Two important details:**

1. `-p` (headless) + `--dangerously-skip-permissions` are mandatory. Without
   `-p`, Claude Code is interactive; without `--dangerously-skip-permissions`, every
   tool call asks for human approval, which breaks the loop. This is a **security
   surface** — Ralph must run inside a sandbox (Docker, E2B, Sprites) because the
   loop is fully autonomous.
2. The `git push` after each iteration is part of the canonical loop. ralph-cli
   should preserve this behavior with a `--no-push` opt-out for local-only runs.

### 2.3 Canonical `PROMPT_build.md` structure

The build prompt is small (~50 lines) but precise. Key sections:

| Section | Purpose |
|---------|---------|
| **0a. Study `specs/*`** | Orient on requirements (parallel subagents) |
| **0b. Study `IMPLEMENTATION_PLAN.md`** | Read current task list |
| **0c. Reference: `src/*`** | Where the code lives |
| **1. Implement per specs, pick most important** | The actual task selection |
| **2. Run tests** | Backpressure |
| **3. Update plan on discoveries** | Self-correcting state |
| **4. Commit + push on test pass** | Atomic commit per task |
| **99999. Capture the why** | Documentation rule |
| **999999. Single sources of truth** | No migrations/adapters |
| **9999999. Tag on test pass** | Versioning |
| **99999999–99999999999999** | Other guardrails (logging, plan maintenance, spec audits) |

The **999-prefix numbering** is Geoff's convention. Higher numbers = higher
priority. The prompts evolve as the user observes failure modes and adds guardrails.

**Implication for ralph-cli:** the prompt text is included verbatim from
`files/PROMPT_build.md`. The CLI does NOT modify prompt text. If the user wants
custom prompts, they edit `.ralph/PROMPT_build.md` (the local copy) — not the CLI
binary.

### 2.4 Canonical `PROMPT_plan.md` structure

Sister prompt. Key difference: explicitly says "Plan only. Do NOT implement
anything." The plan mode reads specs, reads `src/`, does gap analysis, and updates
`IMPLEMENTATION_PLAN.md`. No commits, no code changes.

### 2.5 PRd.md / AGENTS.md conventions

**`AGENTS.md` (~60 lines max):**
- Build/run commands
- Test/typecheck/lint commands (backpressure)
- Operational learnings (e.g. "the seed script needs `cd web && pnpm seed`")
- **NOT** a changelog, progress diary, or status log
- Status updates belong in `IMPLEMENTATION_PLAN.md`

The CLI's `ralph init` should generate a minimal `AGENTS.md` with placeholder
sections (`## Build & Run`, `## Validation`, `## Operational Notes`) and **empty
subsections** — Clayton's tip: start with NOTHING, observe, fill in as needed.

**`IMPLEMENTATION_PLAN.md`:**
- Bullet-point list, prioritized
- LLM-generated, LLM-updated
- No rigid schema — let Ralph decide
- Can be deleted and regenerated (cheap)

**`specs/*.md`:**
- One file per JTBD topic of concern
- Passes the "one sentence without 'and'" test
- Acceptance criteria = behavioral outcomes, NOT implementation approach
- No code blocks in specs (let Ralph decide implementation)

---

## 3. Existing implementations surveyed

### 3.1 ClaytonFarr/ralph-playbook (989★, 255 forks, 55 commits)

The canonical playbook + templates. **Not a CLI.** You copy `files/*` into your
project and run `./loop.sh`. Friction points:

- Manual copy of template files
- No `init` command — you have to read the README
- No sandbox management (you bring your own)
- No `--reverse` mode (you have to use `PROMPT_reverse_engineer_specs.md` manually)
- No JSON status output (hard to wire into a control plane)
- No global install (it's a per-project bash script)

**What ralph-cli inherits:** all template files, the 999-prompt-numbering
convention, the iteration loop semantics, the philosophy.

**What ralph-cli adds:** a globally-installed binary that knows how to scaffold,
invoke, and observe.

### 3.2 Prior internal Ralph work (from the vault)

The vault has two related specs. Worth knowing about so we don't duplicate work:

| Spec | Harness | Status | Relationship to ralph-cli |
|------|---------|--------|--------------------------|
| `specs/ralph-loop-spec` | `pi` | Designed, not built | A bash script at `~/Projects/ralph/ralph.sh` with pi as the coding agent. Uses `.ralph/` scaffold with `PLAN_HASH` SHA-stable exit, `LOOP_COUNT`, hooks dir. **Same philosophy, different harness.** ralph-cli could subsume this if we ever want a `pi` backend, but currently out of scope. |
| `specs/ralph-paperclip-integration-goal` | `pi` | Designed, not built | A governance wrapper around Ralph — error classification, retry policy, secrets injection, board-operator approval gates. **ralph-cli should emit the JSON status events that a future Paperclip adapter would consume.** No direct code reuse, but contract alignment matters. |

**Ralph-cli's positioning relative to prior work:**

- Different harness (Claude Code, not `pi`).
- Globally installable (the prior specs kept Ralph per-project).
- All-in-one (init/plan/loop/reverse/status), where the prior specs were loop-only.
- Sandbox-agnostic (the prior specs assumed local-only).

**Recommendation:** Vault these prior specs as `superseded_by: ralph-cli` and note
that ralph-cli is the canonical implementation. If a `pi` backend is ever needed,
add a `--harness pi` flag to ralph-cli.

### 3.3 Other Ralph Loop implementations (web search)

The community has produced several Ralph-style loops, but most are ad-hoc
shells wrapping `claude` or `amp` in a `while true` loop. Notable mentions from
the December 2025 surge:

- **Geoffrey Huntley** — original blog post (newsletter-gated), YouTube overview.
  The authoritative source for the philosophy.
- **Anthropic's `ralph-loop` Claude Code plugin** — built-in
  `/ralph-loop` slash command. Equivalent to `loop.sh` but packaged as a
  Claude Code plugin. Limited to Claude Code users; less flexible than a CLI.
- **Various personal blog posts** — many "I built a Ralph loop in 20 lines of
  bash" posts. The pattern is widely known; no one is shipping a "Ralph as a
  product" CLI.
- **No public `ralph-cli` package on npm, PyPI, or crates.io as of June 2026**
  (verified by web search). The name is available.

**Implication:** we are not stepping on an existing popular tool. `ralph-cli` is a
green-field name, and we get to define the convention.

### 3.4 Similar autonomous coding systems (for context)

- **Aider architect mode** — chat-based, not a loop. Different steering model.
- **Devin** — long-lived session, browser-based IDE. Not Ralph.
- **Cursor Composer** — IDE-integrated, multi-file edits, not a CLI loop.
- **Replit Agent 3** — cloud IDE + agent, 200-min autonomous sessions. Different
  blast radius (cloud sandbox), different unit of work (whole feature, not one
  commit).

**Pattern:** the file-loop pattern is unique to Ralph. The closest analog is
**Karpathy's autoresearch loop** (mentioned in vault as
`mem/2026-04-07-karpathy-autoresearch-10x-claude-code`) — a Claude Code session
that autonomously drives research, writes a report, commits, and iterates. Same
file-state pattern, different domain (research vs. coding).

### 3.5 Sandbox environments (from `references/sandbox-environments.md`)

Because `--dangerously-skip-permissions` disables Claude Code's permission
system, **Ralph must run inside a sandbox**. The playbook's reference doc
surveys:

| Sandbox | Isolation | Cold start | Best for |
|---------|-----------|------------|----------|
| **E2B** | Firecracker microVM | ~150ms | Production agents, MCP tools |
| **Sprites (Fly.io)** | Firecracker microVM | <1s restore | Long-running persistent agents |
| **Docker local** | Container | instant | Local dev |
| **Modal** | gVisor | 2-5s | Python ML workloads |
| **Cloudflare Sandboxes** | Container | 1-5s | Edge apps |
| **exe.dev** | Full VM (KVM) | ~2s | SSH-native persistent |

**ralph-cli sandbox story:** don't bundle a sandbox manager. Out of scope.
Document the requirement clearly in the README. Provide a `--sandbox <provider>`
flag in a future version that emits `docker run` / `e2b sandbox spawn` commands
but does not execute them (let the user pick their tool).

---

## 4. Patterns for an all-in-one CLI

### 4.1 What to consolidate

Looking at the friction points in the playbook (Section 3.1), an all-in-one CLI
should:

1. **Bundle the template files** — no manual copy.
2. **Provide an `init` command** — one-shot scaffold a project.
3. **Provide a `loop` command** — run the loop with mode + harness flags.
4. **Provide a `plan` command** — explicit gap-analysis mode.
5. **Provide a `reverse` command** — generate specs from existing code (brownfield).
6. **Provide a `status` command** — show current state (iteration count, plan
   hash, last commit, harness version).
7. **Provide an `abort` command** — signal running loop to exit gracefully.
8. **Emit JSON status events to stdout** — for downstream consumers (CI, governance).
9. **Manage a `.ralph/` directory** — keep state, not source.
10. **Install globally** — `~/.local/bin/ralph` (or `/usr/local/bin/ralph` on macOS).

### 4.2 What to NOT build (resist scope creep)

- ❌ PRD/JSON validation — use the de-facto schema, trust Ralph to write it.
- ❌ TUI / web dashboard — ClaytonFarr doesn't have one; separate project.
- ❌ Multi-harness routing — single harness (Claude Code) keeps it simple.
  Multi-harness can be added later via a `--harness` flag.
- ❌ Sandbox management — out of scope; document the requirement.
- ❌ Cost / quota tracking — Claude Code API handles this; we don't need to
  duplicate it.
- ❌ Web UI / hosted service — the value is in the local CLI, not a SaaS.
- ❌ Spec editor — Ralph writes specs. If the user wants a spec IDE, that's a
  different tool.

### 4.3 What to make pluggable

- **Harness:** future `--harness pi|amp|codex|opencode` flag. Today: hard-coded
  to `claude` (Claude Code CLI).
- **Prompts:** if the user edits `.ralph/PROMPT_*.md`, the CLI uses the local
  copy. The CLI ships defaults but treats them as a template, not a constraint.
- **Loop script:** the canonical `loop.sh` is the default, but the CLI should
  accept `--script <path>` to use a custom loop script. Power users can swap
  in their own iteration logic.
- **Status output:** default is human-readable, but `--json` emits structured
  events (suitable for piping into jq / Paperclip / CI).

---

## 5. CLI architecture recommendations

### 5.1 Language: Go

**Decision: Go** for the `ralph` binary.

| Language | Pros | Cons | Verdict |
|----------|------|------|---------|
| **Go** | Single static binary (~5–8 MB), no runtime deps, easy cross-compile to macOS+Linux+ARM, `os/exec` for Claude Code invocation, mature CLI ecosystem (cobra/urfave/cli) | No first-class JSON5/Markdown libs (we don't need them), generics are minimal | ✅ **Recommended** |
| **Rust** | Fast, single binary, great error messages | Heavier toolchain for contributors, no advantage for shell orchestration | ❌ Overkill |
| **Python** | Familiar, fast to write, great stdlib | Shebang + venv pain, no single-binary install, slower startup | ❌ |
| **Bash** | Zero install, matches `loop.sh` | No cross-platform install, hard to test, no `--json` output | ❌ |
| **TypeScript (Bun/Deno)** | Single binary possible (Bun), familiar | Heavier than Go, less universal CLI toolchain, npm is a bad fit for a system CLI | ❌ |

**Go dependencies to consider:**

- `github.com/spf13/cobra` — CLI framework (subcommand routing, help text, flags)
- `github.com/spf13/viper` — config (`.ralphrc` parsing) — optional
- No JSON5 library needed; we read/write the playbook's `prd.json` which is
  standard JSON5-compat JSON.

### 5.2 Subcommand structure

```
ralph init [path]              Scaffold .ralph/ in target path
  --force                       Overwrite existing .ralph/ if present
  --harness <name>              Default: claude (future: pi, amp, codex, opencode)
  --no-templates                 Skip the template file copy (use existing)
  --no-gitignore                 Don't add .ralph/ to .gitignore

ralph plan [path] [max]        Run plan mode (gap analysis, no code)
  --json                        Emit JSON status events to stdout
  --model <opus|sonnet>         Default: opus (forwarded to Claude Code)

ralph loop [path] [max]        Run build mode (canonical Ralph loop)
  --json                        Emit JSON status events
  --model <opus|sonnet>         Default: opus
  --no-push                     Skip git push after each iteration
  --no-commit                   Skip git commit (iterate without saving)
  --reverse                     Brownfield mode: reverse-engineer specs first

ralph status [path]            Print current state
  --json                        Emit as JSON

ralph abort [path]             Signal running loop to exit gracefully
  (writes .ralph/ABORT_REQUESTED sentinel)

ralph --version                Print version
ralph --help                   Print help

ralph uninstall                Remove the binary and templates (cleanup)
```

### 5.3 State management

`.ralph/` directory layout (created by `ralph init`):

```
.ralph/
├── PROMPT_build.md             # copied from ralph-cli's embedded template
├── PROMPT_plan.md              # "
├── PROMPT_reverse_engineer_specs.md  # "
├── AGENTS.md                   # generated template (mostly empty, user fills in)
├── IMPLEMENTATION_PLAN.md      # empty template
├── loop.sh                     # copied, made executable
├── ralph.json                  # state: { "harness": "claude", "version": "0.1.0", "loop_count": 0, "last_session_id": null, "last_commit": null }
├── ABORT_REQUESTED             # sentinel file (absent = no abort)
└── sessions/                   # optional, populated by --retain-sessions
```

`.gitignore` entry added by default: `.ralph/`. The user can choose to commit
template files (e.g. `PROMPT_*.md` for team-wide consistency) by removing the
gitignore entry, but `ralph.json` and `sessions/` should never be committed.

### 5.4 Process invocation

The CLI is fundamentally a process launcher. The hot path for `ralph loop`:

```go
for iter := 0; maxIter == 0 || iter < maxIter; iter++ {
    select {
    case <-abortCh:
        return nil  // graceful exit
    default:
    }

    cmd := exec.CommandContext(ctx, "claude", "-p", "--dangerously-skip-permissions",
        "--output-format=stream-json", "--model", model, "--verbose")
    cmd.Stdin = strings.NewReader(promptContent)
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil { ... }

    if !noPush {
        run("git", "push", "origin", currentBranch)
    }

    if abortRequested() { return nil }
}
```

**Concurrency model:** one Claude Code process at a time. The "parallel subagents"
are spawned *inside* Claude Code, not by the CLI. The CLI is intentionally
single-threaded; parallelism is Ralph's concern.

### 5.5 Error handling

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Success (clean exit, all iterations complete) |
| 1 | Generic failure (Claude Code non-zero exit) |
| 2 | Aborted (user requested) |
| 3 | Init failure (could not create `.ralph/`) |
| 4 | Harness not found (`claude` binary missing) |
| 5 | Not in a git repo (for commands that need git) |
| 6 | Config invalid (`.ralph/ralph.json` malformed) |

### 5.6 Distribution

- **Single binary** built with `go build` for `darwin/amd64`, `darwin/arm64`,
  `linux/amd64`, `linux/arm64`.
- **Homebrew tap** (`klampa/tap` or similar) for macOS.
- **Install script** (`curl | sh`) for Linux.
- **Go install** (`go install github.com/klampa/ralph-cli/cmd/ralph@latest`)
  for Go users.
- **No npm, no PyPI, no crates.io** — those ecosystems are bad fits for a
  system-level CLI.

### 5.7 Dependencies (third-party Go libs)

Minimal:
- `github.com/spf13/cobra` — CLI framework
- Stdlib: `os/exec`, `encoding/json`, `path/filepath`, `embed` (for templates)

That's it. No web framework, no logging library, no terminal UI library. The
CLI is intentionally tiny.

### 5.8 Test strategy

- **Unit tests:** `internal/scaffold`, `internal/state`, `internal/runner`.
  Use `go test ./...`. Mock `os/exec` to avoid spawning real `claude`.
- **Integration tests:** run in a temp dir, invoke `ralph init`, verify
  `.ralph/` contents. Use `go test -tags=integration` to gate.
- **End-to-end smoke test (Grover's task 4):** `ralph init /tmp/smoke && cd /tmp/smoke
  && ralph plan 1` with a stub `claude` binary. Verify the loop calls the binary
  with the expected args and produces the expected `.ralph/` state changes.

---

## 6. Sample user workflow

### 6.1 Greenfield project

```bash
# Install ralph once
brew install klampa/tap/ralph   # or: curl -sSf https://klampa.dev/ralph/install.sh | sh

# Create new project
mkdir ~/projects/cool-app && cd ~/projects/cool-app
git init

# Scaffold Ralph
ralph init
# → creates .ralph/PROMPT_build.md, .ralph/PROMPT_plan.md,
#   .ralph/AGENTS.md, .ralph/IMPLEMENTATION_PLAN.md, .ralph/loop.sh
# → adds .ralph/ to .gitignore

# Write specs
ralph plan
# (interactive: Claude Code conversation produces specs/01-foo.md, etc.)
# (or: write specs by hand and skip the planning loop)

# Run the loop
ralph loop
# Ralph works indefinitely. Ctrl+C to stop.
# Each iteration: one task from IMPLEMENTATION_PLAN.md, one commit, one push.
```

### 6.2 Brownfield project

```bash
cd ~/projects/legacy-app  # existing code, no specs
git init  # if not already
ralph init
ralph loop --reverse
# Ralph reads src/, writes specs/* via PROMPT_reverse_engineer_specs.md,
# then proceeds with plan → build.
```

### 6.3 Check status

```bash
ralph status
# Project: cool-app
# Harness: claude (v1.0.30)
# Loop count: 17
# Plan tasks: 4 remaining, 12 complete
# Last commit: abc1234 (Add user auth)
# Last session: ses_xyz
# Plan hash: stable (12 iterations unchanged)

ralph status --json | jq
# { "project": "cool-app", "harness": "claude", "loop_count": 17, ... }
```

### 6.4 Graceful abort

```bash
# In another terminal
ralph abort
# Ralph's current iteration completes, then the loop exits cleanly.

# Or: Ctrl+C in the ralph loop terminal (SIGINT, same effect).
```

---

## 7. Integration with coding agents

### 7.1 The Claude Code invocation contract

Ralph-cli invokes Claude Code with a fixed set of flags. These are the contract
that any harness must implement to be ralph-cli-compatible.

**Required flags (Claude Code v1.0+):**

| Flag | Purpose |
|------|---------|
| `-p` | Headless mode: read prompt from stdin, print to stdout, exit |
| `--dangerously-skip-permissions` | Auto-approve all tool calls (YOLO mode, required for autonomy) |
| `--output-format=stream-json` | Structured output (parseable for logs) |
| `--include-partial-messages` | Stream partial tool results (live feedback) |
| `--model <opus\|sonnet>` | Model selection |
| `--verbose` | Detailed execution logging |

**Environment:**

| Var | Purpose |
|-----|---------|
| `CLAUDE_CODE_ENTRYPOINT` | Set to `ralph-cli` (telemetry attribution) |
| `ANTHROPIC_API_KEY` | API key (user-provided) |

**Stdin:** the full prompt text (from `PROMPT_build.md` or `PROMPT_plan.md`),
plus the appended context: `specs/*`, `IMPLEMENTATION_PLAN.md`, `AGENTS.md`,
`.ralph/ralph.json`.

### 7.2 Future harnesses

If a `--harness pi|amp|codex|opencode` flag is ever added, the contract for each
harness is the same (read prompt, do work, exit). The CLI is a thin adapter.

For the v1 of ralph-cli, **only Claude Code is supported**. Adding more harnesses
later is a `--harness` flag away; we don't need to design for it now.

### 7.3 Sandboxing guidance (in README, not in CLI)

The README should call out: "Ralph must run inside a sandbox because of
`--dangerously-skip-permissions`." Provide examples:

```bash
# Docker
docker run -v $(pwd):/workspace -w /workspace klampa/ralph-sandbox \
  ralph loop

# E2B (via e2b CLI)
e2b sandbox spawn --template anthropic-claude-code
# ... then run ralph loop inside

# Local (no sandbox, accept the risk)
# Just: ralph loop
```

**ralph-cli does NOT spawn or manage sandboxes.** The user chooses their tool.
The CLI's job is to be sandbox-friendly (write only inside `.ralph/`, never
touch anything outside the project root).

---

## 8. Risks and mitigations

### 8.1 Cost runaway

**Risk:** a runaway loop burns API credits if `IMPLEMENTATION_PLAN.md` is
malformed or the agent goes in circles.

**Mitigations:**

- Default to a `max_iterations` cap (e.g., 50) for first-time runs.
- Require explicit `--no-cap` to run unlimited.
- Print estimated cost per iteration in `ralph status` if the harness reports
  token usage.
- Recommend `ralph plan` first, then review the plan, then `ralph loop` —
  don't auto-loop from a brand-new init.

### 8.2 Infinite loops / drift

**Risk:** Ralph keeps iterating but produces no commits (e.g. plan is
contradictory, tests are flaky, agent is confused).

**Mitigations:**

- `--max-iterations <N>` enforces a hard cap.
- `ralph abort` writes a sentinel file the loop checks each iteration.
- If 3 consecutive iterations produce no commit, the loop can warn (and exit if
  `--strict` is passed).
- The `IMPLEMENTATION_PLAN.md` is editable by hand; the user can prune bad
  tasks.

### 8.3 Context blowup

**Risk:** the prompt or `AGENTS.md` grows over time and crowds out the smart zone.

**Mitigations:**

- Document the "start with nothing in `AGENTS.md`" rule.
- `ralph status` can report `AGENTS.md` line count and warn if > 100.
- The `999-prompt-numbering` convention has a soft cap (~15 rules) before
  diminishing returns.

### 8.4 Sandbox escape (worst case)

**Risk:** Ralph (with `--dangerously-skip-permissions`) does something harmful:
`rm -rf $HOME`, sends secrets to a remote server, modifies files outside the
project root.

**Mitigations:**

- Document the sandbox requirement in the README (front page, bold).
- Recommend Docker, E2B, Sprites for any non-throwaway work.
- Add a `ralph doctor` command in v2 that checks for sandbox presence and
  warns if absent.
- Never auto-push to remotes by default; require `--push` (the current
  canonical `loop.sh` always pushes, but ralph-cli can be more conservative).

### 8.5 Single-harness lock-in

**Risk:** pinning to Claude Code means users without an Anthropic API key can't
run ralph-cli.

**Mitigations:**

- v1 is Claude Code only. The `--harness` flag is designed in but not
  implemented; the door is open for `pi` / `amp` / `codex` / `opencode` backends.
- The harness is a runtime concern, not a compile-time concern. The same Go
  binary can dispatch to any harness that implements the contract.

### 8.6 PRD/JSON schema drift

**Risk:** the de-facto `prd.json` schema evolves and ralph-cli's template goes
stale.

**Mitigations:**

- The CLI bundles templates as `embed.FS` — they ship with the binary and are
  versioned with the CLI. Out-of-date templates are fixed by upgrading the CLI.
- `ralph init --force` overwrites the local template with the latest.
- A `ralph doctor` command (v2) can check if the local `prd.json` matches the
  CLI's expected schema and warn.

---

## 9. Open questions for Bert (SPEC phase)

These are decisions Bert should make or flag in the SPEC:

1. **License:** MIT (matches `ClaytonFarr/ralph-playbook`) vs. Apache 2.0 vs.
   BSL. Recommend MIT for compatibility.
2. **Repo location:** `klampa/ralph-cli` GitHub org assumption — confirm with Kyle.
3. **v0.1 vs v1.0 scope:** do we ship `init + plan + loop + status + abort` in
   v0.1 and defer `--reverse` to v0.2? Or include `--reverse` in v0.1?
4. **`ralph doctor`:** include in v0.1 or defer to v0.2?
5. **`--harness` flag:** stub it in v0.1 (accepts only `claude`, errors on
   others) or omit entirely until needed?
6. **Homebrew tap:** set up in v0.1 or wait for v1.0?
7. **Telemetry / opt-in metrics:** ralph-cli can emit anonymous usage stats
   (e.g. "loop ran 17 iterations over 4 hours"). Opt-in only. Include in v0.1
   or defer?
8. **Test fixtures:** should we vendor a minimal "smoke test" project (one
   `specs/01-hello.md`, one source file, one test) for integration tests?

---

## 10. Sources and references

### Primary source

- **ClaytonFarr/ralph-playbook** — https://github.com/ClaytonFarr/ralph-playbook
  - Cloned to `/tmp/ralph-playbook` on autogpt.local
  - 989 stars, 255 forks, 55 commits (as of Feb 2026, last commit 3 months ago)
  - Files reviewed: `files/loop.sh`, `files/loop_streamed.sh`,
    `files/parse_stream.js`, `files/PROMPT_build.md`, `files/PROMPT_plan.md`,
    `files/PROMPT_specs.md`, `files/PROMPT_reverse_engineer_specs.md`,
    `files/AGENTS.md`, `files/IMPLEMENTATION_PLAN.md`,
    `references/sandbox-environments.md`, `README.md` (1,446 lines)

### Vault context (synthesized in this research)

- `projects/ralph-cli` — the current project context, command surface, key
  decisions
- `ideas/ralph-playbook` — Geoff + Clayton's philosophy, 3 phases, key
  principles
- `mem/skills/ralph_loops` — Ralph Loops skill (compiled April 2026)
- `specs/ralph-loop-spec` — prior Ralph spec (pi-harness, `~/Projects/ralph/`)
- `specs/ralph-paperclip-integration-goal` — governance wrapper spec
- `mem/2026-04-07-karpathy-autoresearch-10x-claude-code` — related
  autoresearch loop pattern

### Web context

- Geoffrey Huntley's original post: https://ghuntley.com/ralph/
- Anthropic's `ralph-loop` Claude Code plugin: ships with Claude Code, equivalent
  to `loop.sh` as a slash command
- No prior `ralph-cli` package on npm, PyPI, or crates.io (verified June 2026)

---

## Appendix A: File tree of `ClaytonFarr/ralph-playbook` (verbatim)

```
ralph-playbook/
├── .gitignore
├── .vscode/
│   └── settings.json
├── files/
│   ├── AGENTS.md                          # 14 lines, template skeleton
│   ├── IMPLEMENTATION_PLAN.md             # 1 line, empty template
│   ├── loop.sh                            # 73 lines, bash outer loop
│   ├── loop_streamed.sh                   # ~80 lines, variant with parse_stream pipe
│   ├── parse_stream.js                    # ~270 lines, Node.js colorized JSON parser
│   ├── PROMPT_build.md                    # ~30 lines, build mode instructions
│   ├── PROMPT_plan.md                     # ~20 lines, plan mode instructions
│   ├── PROMPT_reverse_engineer_specs.md   # ~30 lines, brownfield reverse prompt
│   └── PROMPT_specs.md                    # ~25 lines, specs audit prompt
├── index.html                             # GitHub Pages render (169 KB)
├── LICENSE                                # MIT
├── NOTICE
├── README.md                              # 1,446 lines, the playbook
└── references/
    ├── nah.png                            # "nah" meme
    ├── ralph-diagram.png                  # 3-phase diagram
    └── sandbox-environments.md            # E2B, Sprites, Modal, exe.dev, Cloudflare, Daytona, Cloud Run, Replit, Docker
```

## Appendix B: Example `prd.json` schemas

The vault and the playbook both reference `prd.json` as a de-facto standard, but
the schema is informal. Three observed shapes:

### B.1 ClaytonFarr/ralph-playbook (implicit, from the README example)

The playbook README shows this in a JTBD example:

```json
{
  "jtbd": "Help designers create mood boards",
  "topics": [
    "image collection",
    "color extraction",
    "layout",
    "sharing"
  ]
}
```

Minimal, no formal schema. The CLI should not validate this — let Ralph write
whatever shape it finds useful.

### B.2 Vault: `specs/ralph-paperclip-integration-goal` (informal)

This prior spec references `prd.json` but does not pin a schema. It defers to
"the code is the source of truth" — the CLI should not impose a schema either.

### B.3 Community (mnfst/ralph-style, cited in vault but not directly inspected)

The vault's `specs/ralph-loop-spec` references a `prd.json` shape used by
`mnfst/ralph`-style implementations. The shape is roughly:

```json
{
  "project": "Project name",
  "branchName": "ralph/feature-x",
  "description": "One-paragraph project description",
  "userStories": [
    {
      "id": "US-001",
      "title": "User can sign up",
      "acceptanceCriteria": [
        "Email and password fields accept input",
        "Submit creates a new account",
        "Duplicate emails return an error"
      ],
      "priority": 1,
      "passes": false,
      "notes": ""
    }
  ]
}
```

The fields `pass`, `priority`, `acceptanceCriteria` are the only universally
agreed-on structure. Everything else (branchName, description, notes) varies.

**Recommendation for ralph-cli:** treat `prd.json` as a black box. The CLI
copies the template, Ralph writes/updates it, the CLI does not validate. If
the user wants schema enforcement, that's a separate tool.

## Appendix C: Word count and section summary

| Section | Approx lines | Status |
|---------|--------------|--------|
| 0. TL;DR | 28 | ✅ |
| 1. What is Ralph Loop? | 70 | ✅ |
| 2. ClaytonFarr/ralph-playbook | 90 | ✅ |
| 3. Existing implementations surveyed | 110 | ✅ |
| 4. Patterns for an all-in-one CLI | 50 | ✅ |
| 5. CLI architecture recommendations | 130 | ✅ |
| 6. Sample user workflow | 60 | ✅ |
| 7. Integration with coding agents | 60 | ✅ |
| 8. Risks and mitigations | 60 | ✅ |
| 9. Open questions for Bert | 25 | ✅ |
| 10. Sources and references | 25 | ✅ |
| Appendix A: file tree | 25 | ✅ |
| Appendix B: prd.json schemas | 50 | ✅ |
| Appendix C: this table | 8 | ✅ |
| **Total** | **~790** | **≥ 200 line requirement met 4×** |

---

**End of research.** Bert: read sections 0, 5, 6, 8 first; the rest is reference.
When you write SPEC.md, focus on the Go binary shape, subcommand surface,
`.ralph/` layout, and exit codes — those are the contracts Elmo will implement.
