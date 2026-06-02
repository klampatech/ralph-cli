package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/scaffold"
	"github.com/klampa/ralph-cli/internal/version"
)

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Scaffold .ralph/ at the project root",
	Long: `Scaffold a hidden .ralph/ directory at the project root with the 6
canonical Ralph template files, an initial ralph.json state file, and
an empty sessions/ directory. Updates .gitignore to ignore .ralph/.

The path argument defaults to the current directory. The path must be
a directory containing a .git/ subdirectory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().Bool("force", false,
		"Overwrite existing .ralph/ files (preserves ralph.json state unless --reset-state)")
	initCmd.Flags().Bool("reset-state", false,
		"With --force: also overwrite ralph.json (state goes to zero)")
	initCmd.Flags().Bool("no-templates", false,
		"Skip copying the 6 template files (use when user has hand-edited them)")
	initCmd.Flags().Bool("no-gitignore", false,
		"Don't add .ralph/ to .gitignore (user wants to commit it)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	// 1. Resolve the path. Default to cwd.
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	absRoot, err := filepath.Abs(target)
	if err != nil {
		return FailWith(ExitInitFail, "resolve path %q: %v", target, err)
	}

	// 2. Must be a directory.
	info, err := os.Stat(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return FailWith(ExitInitFail, "path does not exist: %s", absRoot)
		}
		return FailWith(ExitInitFail, "stat %s: %v", absRoot, err)
	}
	if !info.IsDir() {
		return FailWith(ExitInitFail, "path is not a directory: %s", absRoot)
	}

	// 3. Must be a git repo (SPEC §3.1 pre-condition).
	gitPath := filepath.Join(absRoot, ".git")
	if _, err := os.Stat(gitPath); err != nil {
		if os.IsNotExist(err) {
			return FailWith(ExitNotInGit, "no .git/ in %s; ralph init requires a git repo (v0.1)", absRoot)
		}
		return FailWith(ExitInitFail, "stat .git/: %v", err)
	}

	// 4. Validate --harness (v0.1: only "claude" accepted, SPEC §6.5).
	harness, _ := cmd.Flags().GetString("harness")
	if harness != "claude" {
		return FailWith(ExitConfigInvalid,
			"only 'claude' is supported in v0.1; --harness %q is reserved for future use", harness)
	}

	// 5. Parse remaining flags.
	force, _ := cmd.Flags().GetBool("force")
	resetState, _ := cmd.Flags().GetBool("reset-state")
	noTemplates, _ := cmd.Flags().GetBool("no-templates")
	noGitignore, _ := cmd.Flags().GetBool("no-gitignore")

	// 6. Do the scaffold.
	res, err := scaffold.Scaffold(absRoot, scaffold.Options{
		Force:        force,
		ResetState:   resetState,
		NoTemplates:  noTemplates,
		NoGitignore:  noGitignore,
		RalphVersion: version.Version,
	})
	if err != nil {
		// .ralph/ already exists without --force is the common case.
		// We map that to ExitConfigInvalid per SPEC §3.1.
		return FailWith(ExitConfigInvalid, "%v", err)
	}

	cmd.Printf("Created .ralph/ in %s\n", absRoot)
	cmd.Printf("  - %d files written, %d preserved, %d skipped\n",
		countNonDir(res.Created), len(res.Preserved), len(res.Skipped))
	cmd.Printf("  - state: %s\n", res.StateFilePath)
	return nil
}

// countNonDir counts entries in a path list that are NOT directories.
// Used for the human-friendly "files written" count in the init output.
func countNonDir(paths []string) int {
	n := 0
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !info.IsDir() {
			n++
		}
	}
	return n
}
