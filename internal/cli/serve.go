package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/samuelbailey123/ditto/internal/server"
	"github.com/samuelbailey123/ditto/internal/tui"
	"github.com/samuelbailey123/ditto/internal/watcher"
	"github.com/spf13/cobra"
)

// Additional ANSI colour codes used for request logging.
const (
	colorBlue  = "\033[34m"
	colorCyan  = "\033[36m"
	colorWhite = "\033[37m"
)

var (
	servePort  int
	serveHost  string
	serveWatch bool
	serveUI    bool
)

var serveCmd = &cobra.Command{
	Use:   "serve [files...]",
	Short: "Start the mock server",
	Long: `Serve loads one or more YAML mock definition files and starts an HTTP server
that responds to the routes defined within them.

When --watch is enabled (the default), Ditto monitors the input files for
changes and reloads the configuration automatically without restarting the
server. Invalid changes are rejected and the previous configuration is kept.

When --ui is enabled, a terminal UI displays a live table of incoming requests
instead of plain log output. Press q or Ctrl+C in the TUI to quit.

Press Ctrl+C or send SIGTERM to shut down gracefully.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runServe,
}

func init() {
	serveCmd.Flags().IntVarP(&servePort, "port", "p", 8080, "port to listen on")
	serveCmd.Flags().StringVar(&serveHost, "host", "localhost", "host to bind to")
	serveCmd.Flags().BoolVar(&serveWatch, "watch", true, "reload config automatically when files change")
	serveCmd.Flags().BoolVar(&serveUI, "ui", false, "show live request log in a terminal UI")
}

// runServe loads config, validates it, starts the server, and handles graceful shutdown.
func runServe(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadFiles(args...)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if errs := config.Validate(cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "error: %v\n", e)
		}
		return fmt.Errorf("config validation failed with %d error(s)", len(errs))
	}

	srv := server.New(cfg, serveHost, servePort)

	// Start the file watcher before entering the signal loop so that any
	// config changes made immediately after startup are not missed.
	var w *watcher.Watcher
	if serveWatch {
		w, err = watcher.New(args, srv.Reload)
		if err != nil {
			return fmt.Errorf("starting watcher: %w", err)
		}
		if err := w.Start(); err != nil {
			return fmt.Errorf("starting watcher: %w", err)
		}
	}

	// Server always runs in a background goroutine so that either the signal
	// handler or the TUI can trigger a clean shutdown.
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	if serveUI {
		return runWithUI(srv, w, errCh)
	}
	return runWithSignals(srv, w, errCh, args, cfg)
}

// runWithUI starts the Bubble Tea TUI in the main goroutine (as bubbletea
// requires) and shuts the server down when the TUI exits.
func runWithUI(srv *server.Server, w *watcher.Watcher, errCh <-chan error) error {
	// Show startup info above the TUI before it takes over the screen.
	fmt.Printf("ditto serving on http://%s\n", srv.Addr())

	m := tui.New(srv.RequestLog)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Forward any early server error into the TUI so it quits cleanly.
	go func() {
		if err := <-errCh; err != nil {
			p.Quit()
		}
	}()

	if _, err := p.Run(); err != nil {
		stopWatcher(w)
		return fmt.Errorf("tui error: %w", err)
	}

	// TUI exited — shut the server down gracefully.
	stopWatcher(w)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}

// runWithSignals is the plain-stdout flow: print startup info, hook up live
// request logging, then wait for an OS signal or server error before shutting
// down gracefully.
func runWithSignals(srv *server.Server, w *watcher.Watcher, errCh <-chan error, args []string, cfg *config.MockConfig) error {
	fmt.Printf("\nditto serving on http://%s:%d\n", serveHost, servePort)

	printRouteList(cfg)
	printScenarioList(cfg)
	printWatchList(args)

	// Wire up live request logging (disabled in TUI mode).
	srv.OnRequest = func(entry server.RequestEntry) {
		printRequestLine(entry)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err != nil {
			stopWatcher(w)
			return fmt.Errorf("server error: %w", err)
		}
	case sig := <-quit:
		fmt.Printf("\nreceived signal %s, shutting down...\n", sig)
	}

	stopWatcher(w)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Stop(ctx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}

	fmt.Println("shutdown complete")
	return nil
}

// printRouteList prints all configured routes with coloured method labels.
func printRouteList(cfg *config.MockConfig) {
	if len(cfg.Routes) == 0 {
		return
	}
	fmt.Println("\n  Routes:")
	for _, r := range cfg.Routes {
		col := methodColor(r.Method)
		fmt.Printf("    %s%-7s%s %s\n", col, strings.ToUpper(r.Method), colorReset, r.Path)
	}
}

// printScenarioList prints all configured scenarios with their step counts.
func printScenarioList(cfg *config.MockConfig) {
	if len(cfg.Scenarios) == 0 {
		return
	}
	fmt.Println("\n  Scenarios:")
	for _, sc := range cfg.Scenarios {
		fmt.Printf("    %s (%d steps)\n", sc.Name, len(sc.Steps))
	}
}

// printWatchList prints the files being watched if watching is enabled.
func printWatchList(args []string) {
	if !serveWatch {
		return
	}
	fmt.Printf("\n  Watching: %v\n", args)
}

// printRequestLine writes a single coloured request summary line to stdout.
//
// Format:
//
//	  METHOD  STATUS  /path                          latency
func printRequestLine(entry server.RequestEntry) {
	methodCol := methodColor(entry.Method)
	statusCol := statusColor(entry.Status)

	latencyStr := formatLatency(entry.Latency)

	if verbose {
		fmt.Printf("  %s%-7s%s  %s%-3d%s  %-30s  %s\n",
			methodCol, entry.Method, colorReset,
			statusCol, entry.Status, colorReset,
			entry.Path,
			latencyStr,
		)
		if len(entry.Params) > 0 {
			fmt.Printf("         params: %v\n", entry.Params)
		}
	} else {
		fmt.Printf("  %s%-7s%s  %s%-3d%s  %-30s  %s\n",
			methodCol, entry.Method, colorReset,
			statusCol, entry.Status, colorReset,
			entry.Path,
			latencyStr,
		)
	}
}

// methodColor returns the ANSI colour code for a given HTTP method.
func methodColor(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return colorBlue
	case "POST":
		return colorGreen
	case "PUT":
		return colorYellow
	case "DELETE":
		return colorRed
	default:
		return colorWhite
	}
}

// statusColor returns the ANSI colour code for a given HTTP status code.
func statusColor(status int) string {
	switch {
	case status >= 500:
		return colorRed
	case status >= 400:
		return colorYellow
	case status >= 300:
		return colorCyan
	default:
		return colorGreen
	}
}

// formatLatency formats a duration as a human-readable latency string.
func formatLatency(d time.Duration) string {
	ms := float64(d) / float64(time.Millisecond)
	return fmt.Sprintf("%.1fms", ms)
}

// stopWatcher stops the watcher if one was created, logging any error.
func stopWatcher(w *watcher.Watcher) {
	if w == nil {
		return
	}
	if err := w.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "watcher stop error: %v\n", err)
	}
}
