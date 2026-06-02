package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/harness"
	"github.com/klampa/ralph-cli/internal/loop"
)

var planCmd = &cobra.Command{
	Use:   "plan [path]",
	Short: "Run the planning loop: gap analysis, produce IMPLEMENTATION_PLAN.md from specs/",
	Long: `Run the planning loop: gap analysis, produce IMPLEMENTATION_PLAN.md
from specs/. Typically a one-shot operation (default --max-iterations=1).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPlan,
}

func init() {
	planCmd.Flags().Bool("json", false, "Emit JSON status events to stdout (see SPEC §9)")
	planCmd.Flags().String("model", "opus", "Model name: 'opus' or 'sonnet' (forwarded to Claude Code)")
	planCmd.Flags().Int("max-iterations", 1, "Plan mode is typically one-shot")
	planCmd.Flags().Bool("no-push", false, "Skip git push (plan mode rarely pushes)")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	absRoot, err := filepath.Abs(target)
	if err != nil {
		return FailWith(ExitInitFail, "resolve path %q: %v", target, err)
	}

	model, _ := cmd.Flags().GetString("model")
	emitJSON, _ := cmd.Flags().GetBool("json")
	maxIter, _ := cmd.Flags().GetInt("max-iterations")
	noPush, _ := cmd.Flags().GetBool("no-push")

	exit, err := loop.Run(cmd.Context(), absRoot, harness.NewClaudeCode(), loop.Options{
		Mode:          loop.ModePlan,
		Model:         model,
		MaxIterations: maxIter,
		NoPush:        noPush,
		EmitJSON:      emitJSON,
		Stdout:        cmd.OutOrStdout(),
		Stderr:        cmd.ErrOrStderr(),
	})
	if err != nil {
		// loop.Run returns a LoopError for preflight failures (exit 4, 5, 6);
		// for runtime failures, err is the underlying cause and exit is the code.
		return FailWith(exit, "%v", err)
	}
	if exit != 0 {
		return FailWith(exit, "plan exited with code %d", exit)
	}
	return nil
}
