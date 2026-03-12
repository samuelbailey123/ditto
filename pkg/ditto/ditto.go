package ditto

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/samuelbailey123/ditto/internal/server"
)

// Start launches a mock HTTP server from one or more YAML fixture files and
// returns the base URL (e.g. "http://127.0.0.1:54321"). The server is
// automatically stopped when the test ends via t.Cleanup.
//
// Files are merged in order: defaults come from the first file, routes and
// scenarios are appended from all files.
//
// The test is immediately failed with t.Fatal if any file cannot be loaded or
// if the merged configuration fails validation.
//
// Usage:
//
//	baseURL := ditto.Start(t, "testdata/api.yaml")
//	resp, err := http.Get(baseURL + "/health")
func Start(t testing.TB, files ...string) string {
	t.Helper()
	return StartWithOptions(t, files)
}

// StartWithOptions launches a mock HTTP server with caller-supplied Option
// values and returns the base URL. File paths must be provided via the files
// slice; options adjust host, port, and verbosity.
//
// Usage:
//
//	baseURL := ditto.StartWithOptions(t, []string{"api.yaml"}, ditto.WithPort(9090))
func StartWithOptions(t testing.TB, files []string, opts ...Option) string {
	t.Helper()

	o := defaultOptions()
	for _, apply := range opts {
		apply(&o)
	}

	cfg := loadAndValidate(t, files)

	srv := server.New(cfg, o.host, o.port)

	if o.verbose {
		t.Logf("ditto: starting mock server (host=%s port=%d files=%s)",
			o.host, o.port, strings.Join(files, ", "))
	}

	// httptest.NewUnstartedServer lets us bind the handler we want while
	// httptest manages listener lifecycle and cleanup registration.
	hs := httptest.NewUnstartedServer(srv)

	// When a specific port is requested we cannot use httptest's default
	// listener, so we start the server normally and register cleanup manually.
	// Port 0 (the common case) always uses httptest's automatic listener.
	if o.port != 0 {
		return startOnSpecificPort(t, srv, o, files)
	}

	hs.Start()
	t.Cleanup(hs.Close)

	if o.verbose {
		t.Logf("ditto: server listening at %s", hs.URL)
	}

	return hs.URL
}

// startOnSpecificPort starts the server on a caller-specified port and returns
// the base URL. It polls until the server is reachable before returning.
func startOnSpecificPort(t testing.TB, srv *server.Server, o options, files []string) string {
	t.Helper()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Poll until the server answers or Start returns an error.
	addr := fmt.Sprintf("%s:%d", o.host, o.port)
	baseURL := "http://" + addr

	waitReady(t, baseURL, errCh)

	t.Cleanup(func() {
		// context.Background is acceptable here; httptest.Server.Close is also
		// fire-and-forget. We use a short-lived context so the cleanup never
		// hangs the test suite.
		_ = srv.Stop(newShutdownCtx())
	})

	if o.verbose {
		t.Logf("ditto: server listening at %s (files=%s)", baseURL, strings.Join(files, ", "))
	}

	return baseURL
}

// loadAndValidate loads and merges YAML files then validates the result.
// On any error it calls t.Fatal, terminating the test immediately.
func loadAndValidate(t testing.TB, files []string) *config.MockConfig {
	t.Helper()

	if len(files) == 0 {
		t.Fatal("ditto: at least one fixture file must be provided")
	}

	cfg, err := config.LoadFiles(files...)
	if err != nil {
		t.Fatalf("ditto: loading fixture files: %v", err)
	}

	if errs := config.Validate(cfg); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		t.Fatalf("ditto: invalid configuration:\n  %s", strings.Join(msgs, "\n  "))
	}

	return cfg
}

// waitReady polls the server's root path until an HTTP response is received or
// the errCh delivers a startup error. It fails the test if neither succeeds
// within the poll deadline.
func waitReady(t testing.TB, baseURL string, errCh <-chan error) {
	t.Helper()

	err := pollUntilReady(baseURL, errCh)
	if err != nil {
		if errors.Is(err, errStartFailed) {
			t.Fatalf("ditto: server failed to start: %v", err)
		}
		t.Fatalf("ditto: server did not become ready: %v", err)
	}
}
