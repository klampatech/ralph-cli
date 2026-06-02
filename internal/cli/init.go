package cli

import (
	"github.com/spf13/cobra"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("init: stub (implemented in Step 4)")
		return nil
	},
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
