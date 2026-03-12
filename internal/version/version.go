package version

import "fmt"

// Build metadata injected at compile time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("ditto version %s (%s) built %s", Version, Commit, Date)
}
