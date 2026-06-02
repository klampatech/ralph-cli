package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/state"
)

var abortCmd = &cobra.Command{
	Use:   "abort [path]",
	Short: "Signal the running loop to exit gracefully at the end of the current iteration",
	Long: `Touch .ralph/ABORT_REQUESTED (empty sentinel file). The loop
driver checks the sentinel at the top of each iteration and exits
with code 2 (aborted) after the current iteration completes cleanly.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAbort,
}

func init() {
	rootCmd.AddCommand(abortCmd)
}

func runAbort(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	absRoot, err := filepath.Abs(target)
	if err != nil {
		return FailWith(ExitInitFail, "resolve path %q: %v", target, err)
	}

	if err := state.RequestAbort(absRoot); err != nil {
		return FailWith(ExitInitFail, "create ABORT_REQUESTED: %v", err)
	}
	cmd.Printf("Abort requested. Loop will exit after the current iteration.\n")
	return nil
}
