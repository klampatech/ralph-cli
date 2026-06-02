package cli

import (
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Run the planning loop: gap analysis, produce IMPLEMENTATION_PLAN.md from specs/",
	Long: `Run the planning loop: gap analysis, produce IMPLEMENTATION_PLAN.md
from specs/. Typically a one-shot operation.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.Println("plan: stub (implemented in Step 8)")
		return nil
	},
}

func init() {
	planCmd.Flags().Bool("json", false, "Emit JSON status events to stdout (see SPEC §9)")
	planCmd.Flags().String("model", "opus", "Model name: 'opus' or 'sonnet' (forwarded to Claude Code)")
	planCmd.Flags().Int("max-iterations", 1, "Plan mode is typically one-shot")
	planCmd.Flags().Bool("no-push", false, "Skip git push (plan mode rarely pushes)")
	rootCmd.AddCommand(planCmd)
}
