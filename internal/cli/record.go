package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/samuelbailey123/ditto/internal/proxy"
	"github.com/spf13/cobra"
)

var (
	recordUpstream string
	recordOutput   string
	recordPort     int
)

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record API traffic and generate mock definitions",
	Long: `Record starts a local HTTP proxy that forwards requests to a real upstream
API and captures every request/response pair.

When you press Ctrl+C, Ditto writes the captured exchanges to a YAML mock
definition file that can be served directly with ditto serve.

Duplicate routes (same method + path + status) are deduplicated automatically —
only the first captured response is kept.

Example:
  ditto record --upstream https://api.example.com --port 9090
  # Then send requests to http://localhost:9090 — they'll be proxied
  # and recorded. Press Ctrl+C to save as recorded.yaml.`,
	RunE: runRecord,
}

func init() {
	recordCmd.Flags().StringVarP(&recordUpstream, "upstream", "u", "", "upstream API URL to proxy to (required)")
	recordCmd.Flags().StringVarP(&recordOutput, "output", "o", "recorded.yaml", "output file for the recorded mock definitions")
	recordCmd.Flags().IntVarP(&recordPort, "port", "p", 8080, "local port to listen on")

	if err := recordCmd.MarkFlagRequired("upstream"); err != nil {
		// MarkFlagRequired only errors on programmer mistakes (unknown flag name).
		panic(err)
	}
}

// runRecord creates a recorder and proxy, serves traffic until SIGINT/SIGTERM,
// then writes the captured exchanges to the configured output file.
func runRecord(_ *cobra.Command, _ []string) error {
	rec := proxy.NewRecorder()

	p, err := proxy.New(recordUpstream, rec)
	if err != nil {
		return fmt.Errorf("creating proxy: %w", err)
	}

	addr := fmt.Sprintf(":%d", recordPort)
	srv := &http.Server{
		Addr:              addr,
		Handler:           p,
		ReadHeaderTimeout: 30 * time.Second,
	}

	fmt.Printf("Recording traffic from http://localhost:%d → %s\n", recordPort, recordUpstream)
	fmt.Printf("Send requests to http://localhost:%d, press Ctrl+C to stop and save\n", recordPort)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("proxy server error: %w", err)
		}
	case sig := <-quit:
		fmt.Printf("\nreceived signal %s, stopping...\n", sig)
	}

	// Gracefully shut down the HTTP server before writing output.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown warning: %v\n", err)
	}

	if err := writeRecording(rec, recordOutput); err != nil {
		return err
	}

	fmt.Printf("Recorded %d exchanges to %s\n", rec.Count(), recordOutput)
	return nil
}

// writeRecording opens (or creates) outputPath and writes the recorder's YAML
// mock definitions to it, replacing any previous content.
func writeRecording(rec *proxy.Recorder, outputPath string) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("creating output file %q: %w", outputPath, err)
	}
	defer f.Close()

	if err := rec.WriteYAML(f); err != nil {
		return fmt.Errorf("writing YAML to %q: %w", outputPath, err)
	}
	return nil
}
