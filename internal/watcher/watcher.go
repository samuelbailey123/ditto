// Package watcher monitors YAML config files for changes and triggers
// a reload callback when a valid new configuration is detected.
package watcher

import (
	"fmt"
	"os"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/samuelbailey123/ditto/internal/config"
)

const debounceDuration = 200 * time.Millisecond

// ReloadFunc is called with the newly loaded configuration after a
// successful reload. The implementation must be safe to call concurrently.
type ReloadFunc func(cfg *config.MockConfig)

// Watcher monitors a set of files for Write and Create events and triggers
// a configuration reload after a debounce window.
type Watcher struct {
	files    []string
	reloadFn ReloadFunc
	fsw      *fsnotify.Watcher
	done     chan struct{}
}

// New creates a Watcher that will watch the given files and call reloadFn
// with the merged config whenever a change is detected and the new config
// is valid. An error is returned if the underlying fsnotify watcher cannot
// be created or if any file cannot be registered for watching.
func New(files []string, reloadFn ReloadFunc) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	for _, f := range files {
		if err := fsw.Add(f); err != nil {
			_ = fsw.Close()
			return nil, fmt.Errorf("watching file %q: %w", f, err)
		}
	}

	return &Watcher{
		files:    files,
		reloadFn: reloadFn,
		fsw:      fsw,
		done:     make(chan struct{}),
	}, nil
}

// Start launches the background goroutine that processes file-system events.
// It is non-blocking and returns immediately. Call Stop to shut down cleanly.
func (w *Watcher) Start() error {
	go w.run()
	return nil
}

// Stop signals the background goroutine to exit and closes the underlying
// fsnotify watcher. It blocks until the goroutine has exited.
func (w *Watcher) Stop() error {
	// Closing the fsnotify watcher causes its Events and Errors channels to
	// be closed, which causes run() to return and close done.
	err := w.fsw.Close()
	<-w.done
	return err
}

// run is the event loop executed in the background goroutine.
// It debounces rapid sequences of events and triggers a reload attempt
// once the debounce window expires without a new event arriving.
func (w *Watcher) run() {
	defer close(w.done)

	var debounce *time.Timer

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				// Channel closed; fsnotify watcher was shut down.
				if debounce != nil {
					debounce.Stop()
				}
				return
			}

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(debounceDuration, w.reload)
			}

		case err, ok := <-w.fsw.Errors:
			if !ok {
				if debounce != nil {
					debounce.Stop()
				}
				return
			}
			fmt.Fprintf(os.Stderr, "watcher error: %v\n", err)
		}
	}
}

// reload attempts to load and validate the watched files. On success it
// calls reloadFn with the new config. On failure it logs to stderr and
// leaves the server running with its current configuration.
func (w *Watcher) reload() {
	cfg, err := config.LoadFiles(w.files...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config reload failed: %v\n", err)
		return
	}

	if errs := config.Validate(cfg); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "config reload failed: %v\n", e)
		}
		return
	}

	w.reloadFn(cfg)
	fmt.Fprintln(os.Stderr, "config reloaded")
}
