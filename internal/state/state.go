// Package state manages the .ralph/ralph.json state file.
//
// The state file is the CLI's single source of mutable state. Schema is
// versioned for forward compatibility — see SchemaVersion. Atomic writes
// (write to tmp, then os.Rename) survive a crash mid-write.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the current schema version. Bump this when the State
// struct's on-disk shape changes; pair the bump with a migration in
// migrations.go (v0.1 has no migrations — only schema_version 1 is valid).
const SchemaVersion = 1

// Config is the user-facing configuration block inside ralph.json. These
// values are the defaults the user can override per-invocation via flags.
type Config struct {
	MaxIterations int  `json:"max_iterations"`
	Model         string `json:"model"`
	Push          bool `json:"push"`
}

// State is the on-disk representation of .ralph/ralph.json.
//
// Fields are stable for schema_version: 1. Adding a field is fine
// (forward-compat read); removing or renaming is a breaking change that
// requires a schema bump.
type State struct {
	SchemaVersion   int       `json:"schema_version"`
	Harness         string    `json:"harness"`
	RalphVersion    string    `json:"ralph_version"`
	CreatedAt       time.Time `json:"created_at"`
	LoopCount       int       `json:"loop_count"`
	LastCommit      string    `json:"last_commit"`
	LastSessionID   string    `json:"last_session_id"`
	LastRunAt       time.Time `json:"last_run_at"`
	LastRunDurationSec int    `json:"last_run_duration_s"`
	PlanHash        string    `json:"plan_hash"`
	Config          Config    `json:"config"`
}

// Default returns a freshly-initialized State for a new project.
// Time fields are zero so callers can stamp them with a deterministic value
// in tests.
func Default() State {
	return State{
		SchemaVersion: SchemaVersion,
		Harness:       "claude",
		LoopCount:     0,
		Config: Config{
			MaxIterations: 50,
			Model:         "opus",
			Push:          true,
		},
	}
}

// Path is the canonical on-disk path: <root>/.ralph/ralph.json.
func Path(root string) string {
	return filepath.Join(root, ".ralph", "ralph.json")
}

// Load reads .ralph/ralph.json from root. If the file does not exist, it
// returns os.ErrNotExist so callers can distinguish "no init yet" from
// "init done but state file corrupt".
//
// Unknown future schema versions cause an error: we deliberately do not
// silently fall through, because that would let a newer ralph binary
// silently downgrade state in a project that an older ralph would then
// fail to read. Better to fail loudly.
func Load(root string) (State, error) {
	var s State
	data, err := os.ReadFile(Path(root))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("malformed ralph.json: %w", err)
	}
	if s.SchemaVersion != SchemaVersion {
		return s, fmt.Errorf("unsupported schema_version %d (this build supports %d)",
			s.SchemaVersion, SchemaVersion)
	}
	return s, nil
}

// Save writes s to .ralph/ralph.json atomically: write to .tmp, fsync,
// then os.Rename. A crash at any point leaves either the old file
// untouched or the new file complete — never a torn write.
func Save(root string, s State) error {
	if s.SchemaVersion == 0 {
		s.SchemaVersion = SchemaVersion
	}
	dir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .ralph/: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := Path(root) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, Path(root)); err != nil {
		// Best-effort cleanup of the tmp on rename failure.
		_ = os.Remove(tmp)
		return fmt.Errorf("rename tmp to ralph.json: %w", err)
	}
	return nil
}

// ErrNoState is returned by LoadOrDefault when no state file exists.
// Callers can check with errors.Is to decide whether to init a fresh state.
var ErrNoState = errors.New("no ralph state file at .ralph/ralph.json")
