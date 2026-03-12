package cli

import (
	"fmt"
	"os"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/samuelbailey123/ditto/internal/diff"
	"github.com/spf13/cobra"
)

// ANSI colour codes — kept minimal to avoid a full terminal library dependency.
const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
)

var diffCmd = &cobra.Command{
	Use:   "diff <file-a> <file-b>",
	Short: "Compare two mock definition files",
	Long: `Diff loads two YAML mock definition files and reports the differences
between them.

Exit code:
  0 — files are identical
  1 — differences found or an error occurred`,
	Args: cobra.ExactArgs(2),
	RunE: runDiff,
}

// runDiff loads both files, computes the diff, and prints the results with
// colour-coded prefixes: + for added, - for removed, ~ for modified.
func runDiff(_ *cobra.Command, args []string) error {
	cfgA, err := loadMockConfig(args[0])
	if err != nil {
		return err
	}

	cfgB, err := loadMockConfig(args[1])
	if err != nil {
		return err
	}

	changes := diff.Compare(cfgA, cfgB)
	if len(changes) == 0 {
		fmt.Println("no differences found")
		return nil
	}

	for _, c := range changes {
		switch c.Type {
		case "added":
			fmt.Fprintf(os.Stdout, "%s+ %s%s\n", colorGreen, c.Route, colorReset)
		case "removed":
			fmt.Fprintf(os.Stdout, "%s- %s%s\n", colorRed, c.Route, colorReset)
		case "modified":
			fmt.Fprintf(os.Stdout, "%s~ %s%s", colorYellow, c.Route, colorReset)
			if c.Details != "" {
				fmt.Fprintf(os.Stdout, " (%s)", c.Details)
			}
			fmt.Fprintln(os.Stdout)
		}
	}

	// Non-zero exit when differences exist so scripts can detect changes.
	os.Exit(1)
	return nil
}

// loadMockConfig reads a YAML file and returns it as a MockConfig, which is
// the type accepted by diff.Compare.
func loadMockConfig(path string) (*config.MockConfig, error) {
	mf, err := config.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading %q: %w", path, err)
	}
	return config.MergeConfigs(mf), nil
}
