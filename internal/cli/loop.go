package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/harness"
	"github.com/klampa/ralph-cli/internal/loop"
)

var loopCmd = &cobra.Command{
	Use:   "loop [path]",
	Short: "Run the canonical build loop: one task per iteration, commit + push per pass",
	Long: `Run the canonical Ralph build loop: one task per iteration,
commit + push per pass. Strictly single-threaded.

With --reverse, runs PROMPT_reverse_engineer_specs.md once first to
generate specs/* before starting the build loop.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLoop,
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

func runLoop(cmd *cobra.Command, args []string) error {
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
	noCap, _ := cmd.Flags().GetBool("no-cap")
	noPush, _ := cmd.Flags().GetBool("no-push")
	noCommit, _ := cmd.Flags().GetBool("no-commit")
	reverse, _ := cmd.Flags().GetBool("reverse")

	exit, err := loop.Run(cmd.Context(), absRoot, harness.NewClaudeCode(), loop.Options{
		Mode:          loop.ModeBuild,
		Model:         model,
		MaxIterations: maxIter,
		NoCap:         noCap,
		NoPush:        noPush,
		NoCommit:      noCommit,
		Reverse:       reverse,
		EmitJSON:      emitJSON,
		Stdout:        cmd.OutOrStdout(),
		Stderr:        cmd.ErrOrStderr(),
	})
	if err != nil {
		return FailWith(exit, "%v", err)
	}
	if exit != 0 {
		return FailWith(exit, "loop exited with code %d", exit)
	}
	return nil
}
