package cli

import (
	"github.com/samuelbailey123/ditto/internal/version"
	"github.com/spf13/cobra"
)

var verbose bool

var rootCmd = &cobra.Command{
	Use:   "ditto",
	Short: "Instant API mocking from YAML",
	Long: `Ditto is a local API mock server driven by YAML definition files.

Define HTTP endpoints, response bodies, status codes, delays, chaos injection,
and stateful scenarios in plain YAML. Ditto serves them instantly — no code required.

Typical workflow:

  1. Write a mock definition file (e.g. api.yaml)
  2. Validate it:   ditto validate api.yaml
  3. Serve it:      ditto serve api.yaml

Additional commands for recording real API traffic and diffing mock behaviour
against live services are available via ditto record and ditto diff.

Examples:
  ditto init                              Generate a starter YAML file
  ditto serve api.yaml                    Start mock server on port 8080
  ditto serve --ui api.yaml users.yaml    Start with live TUI dashboard
  ditto validate api.yaml                 Check config for errors
  ditto record -u https://api.example.com Record real API traffic
  ditto diff old.yaml new.yaml            Compare two mock definitions`,
}

// Execute runs the root command and returns any error.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Set a version string that cobra will print for --version without its own prefix.
	rootCmd.Version = version.String()
	rootCmd.SetVersionTemplate("{{.Version}}\n")

	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(recordCmd)
	rootCmd.AddCommand(diffCmd)
}
