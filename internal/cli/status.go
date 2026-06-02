package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/state"
)

var statusCmd = &cobra.Command{
	Use:   "status [path]",
	Short: "Print current ralph state (human-readable or --json)",
	Long: `Print current ralph state. Default: human-readable. --json:
machine-readable NDJSON status events (see SPEC §9).

If no .ralph/ exists at the path, prints a helpful "not initialized" message
and exits 0.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().Bool("json", false, "Emit as JSON (see SPEC §9 schema)")
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	absRoot, err := filepath.Abs(target)
	if err != nil {
		return FailWith(ExitInitFail, "resolve path %q: %v", target, err)
	}

	emitJSON, _ := cmd.Flags().GetBool("json")

	// If .ralph/ doesn't exist, report cleanly and exit 0.
	if _, err := os.Stat(filepath.Join(absRoot, ".ralph")); err != nil {
		if os.IsNotExist(err) {
			if emitJSON {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"initialized": false,
					"project":     absRoot,
				})
			} else {
				fmt.Fprintf(cmd.OutOrStdout(),
					"ralph: %s is not initialized (no .ralph/). Run 'ralph init' to scaffold.\n", absRoot)
			}
			return nil
		}
		return FailWith(ExitInitFail, "stat .ralph/: %v", err)
	}

	st, err := state.Load(absRoot)
	if err != nil {
		return FailWith(ExitConfigInvalid, "load ralph.json: %v", err)
	}

	if emitJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(st)
	}

	// Human-readable output (SPEC §3.4).
	projectName := filepath.Base(absRoot)
	fmt.Fprintf(cmd.OutOrStdout(), "Project: %s\n", projectName)
	fmt.Fprintf(cmd.OutOrStdout(), "Harness: %s\n", st.Harness)
	fmt.Fprintf(cmd.OutOrStdout(), "Loop count: %d\n", st.LoopCount)
	if st.LastCommit != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Last commit: %s\n", st.LastCommit)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Last commit: none\n")
	}
	if st.LastSessionID != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Last session: %s\n", st.LastSessionID)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Last session: none\n")
	}
	if st.PlanHash != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Plan hash: %s\n", st.PlanHash)
	}
	// .ralph/ size is a small nicety; report or omit.
	if size, err := dirSize(filepath.Join(absRoot, ".ralph")); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), ".ralph/ size: %s\n", humanSize(size))
	}
	return nil
}

// dirSize returns the total file size under path (recursive).
func dirSize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// humanSize returns a human-readable byte count.
func humanSize(n int64) string {
	const (
		k = 1024
		m = k * 1024
		g = m * 1024
	)
	switch {
	case n >= g:
		return fmt.Sprintf("%.1fG", float64(n)/float64(g))
	case n >= m:
		return fmt.Sprintf("%.1fM", float64(n)/float64(m))
	case n >= k:
		return fmt.Sprintf("%.1fK", float64(n)/float64(k))
	default:
		return fmt.Sprintf("%dB", n)
	}
}
