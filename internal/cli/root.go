// Package cli implements the ralph command-line interface via cobra.
package cli

import (
	"fmt"

	"github.com/klampa/ralph-cli/internal/version"
)

// Execute is the top-level entry point called from cmd/ralph/main.go.
//
// In Step 1, before cobra is wired up, this is a stub that just prints
// the version (matching the Step 1 acceptance criterion in SPEC §15).
// Step 2 replaces it with a real cobra root.
func Execute() error {
	fmt.Printf("ralph %s (%s, %s)\n", version.Version, version.Commit, version.Date)
	return nil
}
