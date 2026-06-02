package cli

import (
	"github.com/spf13/cobra"
)

var abortCmd = &cobra.Command{
	Use:   "abort [path]",
	Short: "Signal the running loop to exit gracefully at the end of the current iteration",
	Long: `Touch .ralph/ABORT_REQUESTED (empty sentinel file). The loop
driver checks the sentinel at the top of each iteration and exits
with code 2 (aborted) after the current iteration completes cleanly.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("abort: stub (implemented in Step 9)")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(abortCmd)
}
