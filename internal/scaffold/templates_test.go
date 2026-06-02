package scaffold

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBundledFilesPresent ensures all 6 expected templates are embedded
// and the embedded byte content matches the source files on disk byte-for-byte.
// This guards against accidental drift between the source files in
// /tmp/ralph-playbook/files/ and the embedded copies.
func TestBundledFilesPresent(t *testing.T) {
	sourceDir := "/tmp/ralph-playbook/files"
	for _, bf := range BundledFiles {
		t.Run(bf.Dest, func(t *testing.T) {
			embedded, err := Read(bf.FSPath)
			if err != nil {
				t.Fatalf("Read(%q) error: %v", bf.FSPath, err)
			}
			if len(embedded) == 0 {
				t.Fatalf("embedded file %q is empty", bf.FSPath)
			}

			// Compare against the source on disk if it exists.
			sourcePath := filepath.Join(sourceDir, bf.Dest)
			if _, err := os.Stat(sourcePath); err == nil {
				disk, err := os.ReadFile(sourcePath)
				if err != nil {
					t.Fatalf("read source %q: %v", sourcePath, err)
				}
				if string(disk) != string(embedded) {
					t.Fatalf("embedded copy of %q differs from source %q; re-copy", bf.FSPath, sourcePath)
				}
			}
		})
	}
}

// TestBundledFilesCount guards against accidentally adding or removing
// a template without updating SPEC §5.
func TestBundledFilesCount(t *testing.T) {
	if got, want := len(BundledFiles), 6; got != want {
		t.Fatalf("BundledFiles has %d entries, want %d (SPEC §5)", got, want)
	}
}

// TestBundledFilesChmod ensures loop.sh is executable and prompts are not.
func TestBundledFilesChmod(t *testing.T) {
	// Spot-check: loop.sh is the only executable.
	execCount := 0
	for _, bf := range BundledFiles {
		if bf.Mode&0o111 != 0 {
			execCount++
			if bf.Dest != "loop.sh" {
				t.Errorf("file %q is unexpectedly executable (mode %s)", bf.Dest, bf.Mode)
			}
		}
	}
	if execCount != 1 {
		t.Errorf("expected exactly 1 executable file, got %d", execCount)
	}
}
