// Package cli implements the ralph command-line interface via cobra.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/klampa/ralph-cli/internal/version"
)

// Exit codes from SPEC §8. Stable contract for shell scripts and CI.
const (
	ExitOK             = 0
	ExitGeneric        = 1
	ExitAborted        = 2
	ExitInitFail       = 3
	ExitHarnessMissing = 4
	ExitNotInGit       = 5
	ExitConfigInvalid  = 6
)

// ExitError carries a stable exit code; the top-level Execute converts
// these into os.Exit() so subcommands can signal failure semantics that
// downstream scripts can rely on.
type ExitError struct {
	Code    int
	Message string
}

func (e *ExitError) Error() string { return e.Message }

// FailWith is a convenience for subcommands: signal an exit-code error.
func FailWith(code int, format string, args ...any) error {
	return &ExitError{Code: code, Message: fmt.Sprintf(format, args...)}
}

// rootCmd is the top-level cobra command. All subcommands attach here.
var rootCmd = &cobra.Command{
	Use:   "ralph",
	Short: "Ralph loop CLI — wrap ClaytonFarr's templates and orchestrate Claude Code",
	Long: `ralph is a globally-installable, single-binary CLI that wraps ClaytonFarr's
canonical Ralph file templates into a hidden .ralph/ scaffold and orchestrates
Claude Code in a continuous, one-task-per-iteration loop.

See ~/projects/ralph-cli/SPEC.md for the full contract.`,
	Version:      version.Version,
	SilenceUsage: true, // don't dump --help on a runtime error
	SilenceErrors: true, // we print our own formatted errors
}

// Execute is the top-level entry point called from cmd/ralph/main.go.
//
// It returns a *ExitError when a subcommand signals a non-zero exit;
// main.go converts that into os.Exit(<code>).
func Execute() error {
	rootCmd.Version = fmt.Sprintf("%s (commit %s, built %s)", version.Version, version.Commit, version.Date)
	if err := rootCmd.Execute(); err != nil {
		// Subcommand returned an error. If it's an ExitError, propagate the code.
		if ee, ok := err.(*ExitError); ok {
			fmt.Fprintf(os.Stderr, "ralph: %s\n", ee.Message)
			os.Exit(ee.Code)
		}
		return err
	}
	return nil
}

func init() {
	// Global flags. Path is a positional arg on each subcommand (SPEC §3).
	rootCmd.PersistentFlags().StringP("harness", "H", "claude",
		"Harness name (v0.1: only 'claude' is accepted)")
}
