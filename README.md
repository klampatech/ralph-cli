# ralph-cli

A globally-installable, single-binary CLI that wraps [ClaytonFarr's Ralph
file templates](https://github.com/ClaytonFarr/ralph-playbook) into a hidden
`.ralph/` scaffold and orchestrates Claude Code in a continuous,
one-task-per-iteration loop.

## Install

```bash
go install github.com/klampa/ralph-cli/cmd/ralph@v0.1.0
```

Or download a release binary from the [GitHub Releases](../../releases) page
(darwin/amd64, darwin/arm64, linux/amd64, linux/arm64).

## Quickstart

```bash
# 1. Scaffold .ralph/ in your project (requires a git repo)
cd ~/projects/my-app
ralph init

# 2. (Optional) Edit .ralph/PROMPT_*.md to fit your workflow

# 3. Run the planning loop to generate IMPLEMENTATION_PLAN.md
ralph plan

# 4. Run the build loop (one task per iteration, commit + push)
ralph loop

# 5. At any time, abort the loop from another terminal
ralph abort
```

## Commands

| Command | Purpose |
|---------|---------|
| `ralph init [path]` | Scaffold `.ralph/` with the 6 canonical templates + state file |
| `ralph plan [path]` | Run the planning loop (gap analysis → IMPLEMENTATION_PLAN.md) |
| `ralph loop [path]` | Run the canonical build loop (one task per iter, commit + push) |
| `ralph status [path]` | Print current ralph state (human-readable or `--json`) |
| `ralph abort [path]` | Signal the running loop to exit after the current iteration |
| `ralph --version` | Print version info |
| `ralph --help` | Show help |

### Common flags

- `--json` — emit NDJSON status events to stdout (downstream consumer contract)
- `--model <opus|sonnet>` — model to forward to Claude Code
- `--max-iterations N` — hard cap (default: 50 for `loop`, 1 for `plan`)
- `--no-cap` — unlimited iterations (DANGEROUS)
- `--no-push` — skip `git push` after each iteration
- `--no-commit` — skip `git commit` (debug only)
- `--reverse` — brownfield mode: run `PROMPT_reverse_engineer_specs.md` once first

## Sandbox required

`ralph loop` invokes Claude Code with `--dangerously-skip-permissions` to
enable autonomous iteration. Run it inside a sandbox (E2B, Sprites, Docker,
a throwaway VM) — not on your laptop's main filesystem.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Generic failure (harness non-zero exit, git failure) |
| 2 | Aborted by user (`ralph abort` or `SIGINT`) |
| 3 | Init failure (permission denied, disk full, path not directory) |
| 4 | Harness not found (`claude` binary not on `PATH`) |
| 5 | Not a git repo |
| 6 | Config invalid (bad `--harness`, missing `.ralph/`, malformed `ralph.json`) |

## Development

```bash
# Build
go build -o ralph ./cmd/ralph

# Unit tests
go test ./...

# Integration tests (builds the binary, runs it against a temp git repo + fake claude)
go test -tags=integration ./...

# Inject version at build time
go build -ldflags "-X github.com/klampa/ralph-cli/internal/version.Version=v0.1.0" -o ralph ./cmd/ralph
```

## License

MIT — see [LICENSE](LICENSE). Bundled Ralph templates are attributed in
[NOTICE](NOTICE).

## Acknowledgments

- [Clayton Farr](https://github.com/ClaytonFarr) — author of
  [ralph-playbook](https://github.com/ClaytonFarr/ralph-playbook) and the
  canonical Ralph prompt templates
- [Geoff Huntley](https://github.com/geoffhuntley) — co-creator of the Ralph
  methodology
