package cli

import (
	"github.com/spf13/cobra"
)

var loopCmd = &cobra.Command{
	Use:   "loop [path]",
	Short: "Run the canonical build loop: one task per iteration, commit + push per pass",
	Long: `Run the canonical Ralph build loop: one task per iteration,
commit + push per pass. Strictly single-threaded.

With --reverse, runs PROMPT_reverse_engineer_specs.md once first to
generate specs/* before starting the build loop.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("loop: stub (implemented in Step 8)")
		return nil
	},
}

func init() {
	loopCmd.Flags().Bool("json", false, "Emit JSON status events")
	loopCmd.Flags().String("model", "opus", "Model name: 'opus' or 'sonnet'")
	loopCmd.Flags().Int("max-iterations", 50, "Hard cap; use --no-cap for unlimited")
	loopCmd.Flags().Bool("no-cap", false, "Unlimited iterations (DANGEROUS — confirm interactively)")
	loopCmd.Flags().Bool("no-push", false, "Skip git push after each iteration")
	loopCmd.Flags().Bool("no-commit", false, "Skip git commit (iterate without saving — debug only)")
	loopCmd.Flags().Bool("reverse", false,
		"Brownfield mode: first run PROMPT_reverse_engineer_specs.md to generate specs, then proceed with build")
	rootCmd.AddCommand(loopCmd)
}
