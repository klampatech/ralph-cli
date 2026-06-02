package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := Default()
	want.CreatedAt = time.Date(2026, 6, 2, 3, 0, 0, 0, time.UTC)
	want.RalphVersion = "0.1.0"
	want.LastSessionID = "ses_01TEST"
	want.LoopCount = 42
	want.Config.Model = "sonnet"

	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SchemaVersion != want.SchemaVersion {
		t.Errorf("SchemaVersion: got %d, want %d", got.SchemaVersion, want.SchemaVersion)
	}
	if got.Harness != want.Harness {
		t.Errorf("Harness: got %q, want %q", got.Harness, want.Harness)
	}
	if got.LoopCount != want.LoopCount {
		t.Errorf("LoopCount: got %d, want %d", got.LoopCount, want.LoopCount)
	}
	if got.Config.Model != want.Config.Model {
		t.Errorf("Config.Model: got %q, want %q", got.Config.Model, want.Config.Model)
	}
}

func TestLoadMissing(t *testing.T) {
	root := t.TempDir()
	_, err := Load(root)
	if err == nil {
		t.Fatal("Load on missing file should error")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected os.IsNotExist, got %v", err)
	}
}

func TestLoadMalformed(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ralph.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil {
		t.Fatal("Load on malformed JSON should error")
	}
}

func TestLoadUnknownSchemaVersion(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ralph.json"),
		[]byte(`{"schema_version": 999, "harness": "claude"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(root)
	if err == nil {
		t.Fatal("Load with future schema_version should error")
	}
}

func TestSaveAtomicNoTmpLeftBehind(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(Path(root) + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be removed after rename, got err=%v", err)
	}
}

func TestPath(t *testing.T) {
	if got, want := Path("/tmp/foo"), "/tmp/foo/.ralph/ralph.json"; got != want {
		t.Errorf("Path: got %q, want %q", got, want)
	}
}
