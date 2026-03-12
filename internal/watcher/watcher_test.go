package watcher_test

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/samuelbailey123/ditto/internal/watcher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// reloadTimeout is the maximum time to wait for a reload callback to arrive.
	reloadTimeout = 2 * time.Second
	// settleDelay gives the OS and fsnotify time to register watches before
	// we write to the file.
	settleDelay = 50 * time.Millisecond
)

// validYAML returns a minimal, valid mock definition YAML document.
func validYAML() []byte {
	return []byte(`
routes:
  - method: GET
    path: /health
    status: 200
    body:
      status: ok
`)
}

// invalidYAML returns YAML that parses but fails config.Validate.
func invalidYAML() []byte {
	// Missing required fields: method, path, status are all absent.
	return []byte(`
routes:
  - body:
      status: error
`)
}

// writeTempYAML creates a temporary file containing the given content and
// returns its path. Callers are responsible for removing the file.
func writeTempYAML(t *testing.T, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ditto.yaml")
	require.NoError(t, os.WriteFile(path, content, 0o644))
	return path
}

// TestWatcher_DetectsChange verifies that modifying a watched file triggers
// the reload callback with a non-nil config.
func TestWatcher_DetectsChange(t *testing.T) {
	t.Parallel()

	path := writeTempYAML(t, validYAML())

	reloaded := make(chan *config.MockConfig, 1)
	w, err := watcher.New([]string{path}, func(cfg *config.MockConfig) {
		select {
		case reloaded <- cfg:
		default:
		}
	})
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { _ = w.Stop() })

	time.Sleep(settleDelay)

	// Rewrite the file with new (still valid) content to trigger a change event.
	updated := append(validYAML(), []byte("\n# change\n")...)
	require.NoError(t, os.WriteFile(path, updated, 0o644))

	select {
	case cfg := <-reloaded:
		assert.NotNil(t, cfg)
		assert.Len(t, cfg.Routes, 1)
	case <-time.After(reloadTimeout):
		t.Fatal("timeout: reload callback was not called after file change")
	}
}

// TestWatcher_Debounce verifies that multiple rapid writes to a watched file
// result in only a single reload callback, not one per write event.
func TestWatcher_Debounce(t *testing.T) {
	t.Parallel()

	path := writeTempYAML(t, validYAML())

	var callCount atomic.Int32
	reloaded := make(chan struct{}, 10)

	w, err := watcher.New([]string{path}, func(_ *config.MockConfig) {
		callCount.Add(1)
		select {
		case reloaded <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { _ = w.Stop() })

	time.Sleep(settleDelay)

	// Write three times in quick succession — all within the debounce window.
	for i := 0; i < 3; i++ {
		require.NoError(t, os.WriteFile(path, validYAML(), 0o644))
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for at least one reload to arrive.
	select {
	case <-reloaded:
	case <-time.After(reloadTimeout):
		t.Fatal("timeout: reload callback was not called")
	}

	// Allow a short window for any additional (spurious) calls to accumulate.
	time.Sleep(500 * time.Millisecond)

	count := callCount.Load()
	assert.Equal(t, int32(1), count, "expected exactly 1 reload, got %d", count)
}

// TestWatcher_InvalidConfig verifies that writing invalid YAML to a watched
// file does not invoke the reload callback, preserving the previous config.
func TestWatcher_InvalidConfig(t *testing.T) {
	t.Parallel()

	path := writeTempYAML(t, validYAML())

	reloaded := make(chan struct{}, 1)
	w, err := watcher.New([]string{path}, func(_ *config.MockConfig) {
		select {
		case reloaded <- struct{}{}:
		default:
		}
	})
	require.NoError(t, err)
	require.NoError(t, w.Start())
	t.Cleanup(func() { _ = w.Stop() })

	time.Sleep(settleDelay)

	// Write content that fails validation (missing required route fields).
	require.NoError(t, os.WriteFile(path, invalidYAML(), 0o644))

	// Give the watcher enough time to process the event and complete the
	// debounce window — if it were going to call reload, it would by now.
	select {
	case <-reloaded:
		t.Fatal("reload callback must not be called when config is invalid")
	case <-time.After(reloadTimeout / 2):
		// Correct behaviour: no callback received within the window.
	}
}
