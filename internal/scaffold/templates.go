package scaffold

import (
	"embed"
	"io/fs"
	"os"
)

//go:embed templates/PROMPT_build.md templates/PROMPT_plan.md templates/PROMPT_reverse_engineer_specs.md templates/AGENTS.md templates/IMPLEMENTATION_PLAN.md templates/loop.sh
var templatesFS embed.FS

// BundledFile describes one template file embedded in templatesFS and
// how it should be written into a project's .ralph/ directory.
type BundledFile struct {
	FSPath string      // path inside templatesFS
	Dest   string      // filename inside .ralph/
	Mode   os.FileMode // Unix permission bits (chmod)
}

// BundledFiles is the canonical list of files that the ralph init command
// writes into a project's .ralph/ directory, in order. The order matches
// SPEC §5 — prompts first, AGENTS.md, IMPLEMENTATION_PLAN.md, loop.sh last.
//
// Each entry maps to a path inside templatesFS via the templates/ prefix.
// The destination name (second field) is what the file is written as inside
// .ralph/ — top-level, no templates/ subdirectory.
var BundledFiles = []BundledFile{
	{"templates/PROMPT_build.md", "PROMPT_build.md", 0o644},
	{"templates/PROMPT_plan.md", "PROMPT_plan.md", 0o644},
	{"templates/PROMPT_reverse_engineer_specs.md", "PROMPT_reverse_engineer_specs.md", 0o644},
	{"templates/AGENTS.md", "AGENTS.md", 0o644},
	{"templates/IMPLEMENTATION_PLAN.md", "IMPLEMENTATION_PLAN.md", 0o644},
	{"templates/loop.sh", "loop.sh", 0o755},
}

// Files returns the embedded filesystem rooted at the templates/ directory.
// Useful for callers that want to enumerate or read files dynamically.
func Files() fs.FS {
	return templatesFS
}

// Read reads a file from the embedded filesystem by its templatesFS path
// (e.g. "templates/PROMPT_build.md"). Returns the raw bytes.
func Read(fsPath string) ([]byte, error) {
	return templatesFS.ReadFile(fsPath)
}
