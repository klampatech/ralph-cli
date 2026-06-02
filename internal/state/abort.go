package state

import (
	"os"
	"path/filepath"
)

// SentinelName is the name of the abort sentinel file.
// Touched by `ralph abort`; checked by the loop driver.
const SentinelName = "ABORT_REQUESTED"

// SentinelPath is the absolute path to the abort sentinel inside .ralph/.
// Exported so the loop driver and the abort subcommand agree on the name.
func SentinelPath(root string) string {
	return filepath.Join(root, ".ralph", SentinelName)
}

// IsAbortRequested returns true if the abort sentinel exists at the
// standard location inside root/.ralph/.
func IsAbortRequested(root string) (bool, error) {
	_, err := os.Stat(SentinelPath(root))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// RequestAbort creates the empty abort sentinel at the standard location.
// Safe to call when the sentinel already exists (no-op).
func RequestAbort(root string) error {
	dir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.Create(SentinelPath(root))
	if err != nil {
		return err
	}
	return f.Close()
}
