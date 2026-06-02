package cli

import (
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "Print current ralph state (human-readable or --json)",
	Long: `Print current ralph state. Default: human-readable. --json:
machine-readable NDJSON status events (see SPEC §9).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("status: stub (implemented in Step 9)")
		return nil
	},
}

func init() {
	statusCmd.Flags().Bool("json", false, "Emit as JSON (see SPEC §9 schema)")
	rootCmd.AddCommand(statusCmd)
}
