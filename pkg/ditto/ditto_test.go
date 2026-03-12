package ditto_test

import (
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/samuelbailey123/ditto/pkg/ditto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataPath returns the absolute path to a file inside the repository-level
// testdata directory. Using runtime.Caller makes the path correct regardless
// of the working directory at test invocation time.
func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	// This file lives at pkg/ditto/ditto_test.go; the repo root is two levels up.
	root := filepath.Join(filepath.Dir(file), "..", "..")
	return filepath.Join(root, "testdata", name)
}

// TestStart_Basic verifies that Start loads a YAML file, binds a port, and
// serves a configured route without any additional options.
func TestStart_Basic(t *testing.T) {
	baseURL := ditto.Start(t, testdataPath("basic.yaml"))

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStart_PathParams verifies that a route with a path parameter ({id}) is
// matched correctly and returns the expected status code.
func TestStart_PathParams(t *testing.T) {
	baseURL := ditto.Start(t, testdataPath("basic.yaml"))

	resp, err := http.Get(baseURL + "/users/42")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStart_MultipleFiles verifies that routes from two YAML files are merged
// and that routes from each file are independently reachable.
func TestStart_MultipleFiles(t *testing.T) {
	baseURL := ditto.Start(t,
		testdataPath("basic.yaml"),
		testdataPath("chaos.yaml"),
	)

	// Route from basic.yaml
	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Route from chaos.yaml — chaos is configured but probability may not fire
	// every time, so we only assert the endpoint exists (2xx or 5xx, not 404).
	resp2, err := http.Get(baseURL + "/flaky")
	require.NoError(t, err)
	_ = resp2.Body.Close()
	assert.NotEqual(t, http.StatusNotFound, resp2.StatusCode,
		"/flaky route from chaos.yaml should be registered")
}

// TestStart_InvalidFile verifies that Start calls t.Fatal when a fixture file
// does not exist. A stub testing.TB is used so the test itself does not abort.
// The stub panics with a fatalSignal when Fatal/Fatalf is called; we recover
// that panic here to confirm it was triggered.
func TestStart_InvalidFile(t *testing.T) {
	stub := &fatalCapture{}

	fatalOccurred := func() (occurred bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); ok {
					occurred = true
				} else {
					// An unexpected panic — re-panic so the test fails loudly.
					panic(r)
				}
			}
		}()
		ditto.Start(stub, "/nonexistent/path/missing.yaml")
		return false
	}()

	assert.True(t, fatalOccurred,
		"Start should call t.Fatal when the fixture file does not exist")
	assert.True(t, stub.fatalCalled,
		"fatalCapture should record the Fatal call")
}

// TestStartWithOptions_CustomPort verifies that StartWithOptions with
// WithPort(0) returns a working base URL (port 0 means OS-assigned).
func TestStartWithOptions_CustomPort(t *testing.T) {
	baseURL := ditto.StartWithOptions(t,
		[]string{testdataPath("basic.yaml")},
		ditto.WithPort(0),
	)

	require.NotEmpty(t, baseURL)
	assert.True(t, len(baseURL) > len("http://"), "base URL should be a full http:// address")

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStart_Cleanup verifies that the mock server stops accepting connections
// after the test's cleanup functions have run. It does this by running a
// sub-test to capture the URL, then probing after that sub-test exits.
func TestStart_Cleanup(t *testing.T) {
	var capturedURL string

	// Run an inner sub-test so we can call its cleanup explicitly.
	innerT := &cleanupRunner{}
	capturedURL = ditto.Start(innerT, testdataPath("basic.yaml"))

	// Server should be reachable while the inner test is "running".
	resp, err := http.Get(capturedURL + "/health")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Trigger all registered cleanups (simulates the test ending).
	innerT.runCleanup()

	// After cleanup the server should no longer accept connections.
	_, err = http.Get(capturedURL + "/health")
	assert.Error(t, err, "server should be stopped after cleanup runs")
}

// TestStartWithOptions_WithHost verifies that the WithHost option is accepted
// and the server still serves routes correctly when bound to localhost.
func TestStartWithOptions_WithHost(t *testing.T) {
	baseURL := ditto.StartWithOptions(t,
		[]string{testdataPath("basic.yaml")},
		ditto.WithHost("127.0.0.1"),
	)

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStartWithOptions_WithVerbose verifies that WithVerbose is accepted and
// does not cause the server to behave incorrectly.
func TestStartWithOptions_WithVerbose(t *testing.T) {
	baseURL := ditto.StartWithOptions(t,
		[]string{testdataPath("basic.yaml")},
		ditto.WithVerbose(),
	)

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStartWithOptions_SpecificPort verifies that StartWithOptions with a
// non-zero port exercises the startOnSpecificPort code path and returns a
// working base URL.
func TestStartWithOptions_SpecificPort(t *testing.T) {
	// Pick a free port by briefly binding and then releasing it.
	port := findFreePort(t)

	baseURL := ditto.StartWithOptions(t,
		[]string{testdataPath("basic.yaml")},
		ditto.WithPort(port),
	)

	require.NotEmpty(t, baseURL)
	assert.Contains(t, baseURL, fmt.Sprintf(":%d", port))

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStartWithOptions_SpecificPort_VerboseAndHost exercises startOnSpecificPort
// combined with WithHost and WithVerbose so those option branches inside
// startOnSpecificPort are covered.
func TestStartWithOptions_SpecificPort_VerboseAndHost(t *testing.T) {
	port := findFreePort(t)

	baseURL := ditto.StartWithOptions(t,
		[]string{testdataPath("basic.yaml")},
		ditto.WithPort(port),
		ditto.WithHost("127.0.0.1"),
		ditto.WithVerbose(),
	)

	resp, err := http.Get(baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestStart_InvalidConfig verifies that Start calls t.Fatal when a YAML file
// loads successfully but fails config validation (e.g. invalid HTTP method,
// missing path slash, bad status code).
func TestStart_InvalidConfig(t *testing.T) {
	stub := &fatalCapture{}

	fatalOccurred := func() (occurred bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); ok {
					occurred = true
				} else {
					panic(r)
				}
			}
		}()
		ditto.Start(stub, testdataPath("invalid.yaml"))
		return false
	}()

	assert.True(t, fatalOccurred, "Start with an invalid config should call t.Fatal")
	assert.True(t, stub.fatalCalled)
}

// TestStart_NoFiles verifies that Start calls t.Fatal when no file arguments
// are supplied at all.
func TestStart_NoFiles(t *testing.T) {
	stub := &fatalCapture{}

	fatalOccurred := func() (occurred bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); ok {
					occurred = true
				} else {
					panic(r)
				}
			}
		}()
		ditto.Start(stub)
		return false
	}()

	assert.True(t, fatalOccurred, "Start with no files should call t.Fatal")
}

// findFreePort binds to port 0 on loopback, records the assigned port, then
// closes the listener so the port is free when the SDK attempts to bind it.
// There is an inherent TOCTOU race on heavily loaded machines but it is
// acceptable for test helpers.
func findFreePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	return port
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// fatalCapture is a minimal testing.TB stub that records whether Fatal/Fatalf
// was called. All other methods are no-ops or return safe zero values so that
// the SDK can call Helper(), Log(), etc. without panicking.
type fatalCapture struct {
	testing.TB // embed to satisfy the interface; panics on unimplemented methods

	mu          sync.Mutex
	fatalCalled bool
}

func (f *fatalCapture) Helper() {}

func (f *fatalCapture) Log(args ...any) {
	_ = fmt.Sprint(args...)
}

func (f *fatalCapture) Logf(format string, args ...any) {
	_ = fmt.Sprintf(format, args...)
}

func (f *fatalCapture) Fatal(args ...any) {
	f.mu.Lock()
	f.fatalCalled = true
	f.mu.Unlock()
	// Use panic to unwind the call stack so execution stops, mirroring how
	// t.Fatal works (it calls runtime.Goexit internally).
	panic(fatalSignal{msg: fmt.Sprint(args...)})
}

func (f *fatalCapture) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatalCalled = true
	f.mu.Unlock()
	panic(fatalSignal{msg: fmt.Sprintf(format, args...)})
}

func (f *fatalCapture) Cleanup(fn func()) {
	// No-op: cleanup not needed in this stub.
}

// fatalSignal is the panic value used by fatalCapture to simulate t.Fatal.
type fatalSignal struct{ msg string }

// cleanupRunner is a minimal testing.TB stub that collects Cleanup functions
// and exposes runCleanup() so tests can trigger them on demand, simulating the
// end of a test's lifecycle.
type cleanupRunner struct {
	testing.TB

	mu       sync.Mutex
	cleanups []func()
}

func (c *cleanupRunner) Helper() {}

func (c *cleanupRunner) Log(args ...any) {
	_ = fmt.Sprint(args...)
}

func (c *cleanupRunner) Logf(format string, args ...any) {
	_ = fmt.Sprintf(format, args...)
}

func (c *cleanupRunner) Fatal(args ...any) {
	panic(fatalSignal{msg: fmt.Sprint(args...)})
}

func (c *cleanupRunner) Fatalf(format string, args ...any) {
	panic(fatalSignal{msg: fmt.Sprintf(format, args...)})
}

func (c *cleanupRunner) Cleanup(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanups = append(c.cleanups, fn)
}

// runCleanup executes all registered cleanup functions in LIFO order,
// matching how the standard testing package runs them.
func (c *cleanupRunner) runCleanup() {
	c.mu.Lock()
	fns := make([]func(), len(c.cleanups))
	copy(fns, c.cleanups)
	c.mu.Unlock()

	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
