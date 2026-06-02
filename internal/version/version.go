// Package version provides build-time version metadata for ralph-cli.
//
// Version, Commit, and Date are injected at build time via -ldflags:
//
//	-X github.com/klampa/ralph-cli/internal/version.Version=v0.1.1
//	-X github.com/klampa/ralph-cli/internal/version.Commit=$(git rev-parse HEAD)
//	-X github.com/klampa/ralph-cli/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
package version

// These vars are overridden at link time. Defaults are dev placeholders.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
