// Command ralph is the entry point for the ralph-cli binary.
//
// ralph wraps ClaytonFarr's Ralph file templates into a hidden .ralph/
// scaffold and orchestrates Claude Code in a continuous, one-task-per-iteration
// loop. See ~/projects/ralph-cli/SPEC.md for the full contract.
package main

import (
	"fmt"
	"os"

	"github.com/klampa/ralph-cli/internal/cli"
	"github.com/klampa/ralph-cli/internal/version"
)

func main() {
	// Allow the version package to be referenced from main even before the
	// full CLI is wired up — this also serves as a smoke test that the
	// version injection is wired correctly.
	_ = version.Version
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ralph:", err)
		os.Exit(1)
	}
}
