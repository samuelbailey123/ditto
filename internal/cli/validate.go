package cli

import (
	"fmt"
	"os"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate [files...]",
	Short: "Validate mock definition files",
	Long: `Validate parses one or more YAML mock definition files and reports any
structural or semantic errors without starting a server.

All errors across all files are printed before exiting. Exit code is 1 if
any validation error is found, 0 if all files are valid.

Example:
  ditto validate api.yaml && ditto serve api.yaml`,
	Args: cobra.MinimumNArgs(1),
	RunE: runValidate,
}

// runValidate loads the provided files, runs validation, and reports results.
func runValidate(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadFiles(args...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	errs := config.Validate(cfg)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
		}
		os.Exit(1)
	}

	fmt.Printf("ok: %d file(s) valid\n", len(args))
	return nil
}
