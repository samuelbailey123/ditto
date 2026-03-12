package ditto

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	// pollInterval is the time between readiness probes.
	pollInterval = 5 * time.Millisecond
	// shutdownTimeout is the grace period given to the server on cleanup.
	shutdownTimeout = 2 * time.Second
)

// pollDeadline is the maximum time to wait for the server to become ready.
// Declared as a variable (not a constant) so tests can reduce it to avoid slow
// timeout waits without changing production behaviour.
var pollDeadline = 3 * time.Second

// errStartFailed is a sentinel returned by pollUntilReady when the server
// goroutine itself returns an error before the server becomes reachable.
var errStartFailed = errors.New("server start error")

// newShutdownCtx returns a context that expires after shutdownTimeout.
// The returned context's Done channel closes when the deadline is reached,
// which is sufficient for a fire-and-forget shutdown call in t.Cleanup.
func newShutdownCtx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	// The goroutine ensures cancel is eventually called so the context's
	// internal timer resources are freed even when the caller does not hold
	// the cancel reference.
	go func() {
		<-ctx.Done()
		cancel()
	}()
	return ctx
}

// pollUntilReady probes baseURL in a tight loop until the server answers with
// any HTTP response, the errCh delivers a startup error, or the pollDeadline
// elapses.
//
// A nil return means the server is reachable. errStartFailed (wrapped) means
// the server goroutine itself failed. Any other non-nil error means the
// deadline elapsed before the server became ready.
func pollUntilReady(baseURL string, errCh <-chan error) error {
	deadline := time.Now().Add(pollDeadline)

	client := &http.Client{
		// Keep per-attempt timeout short so we cycle probes quickly.
		Timeout: 200 * time.Millisecond,
	}

	for time.Now().Before(deadline) {
		// Non-blocking check: did the server goroutine fail immediately?
		select {
		case err := <-errCh:
			return fmt.Errorf("%w: %v", errStartFailed, err)
		default:
		}

		resp, err := client.Get(baseURL + "/")
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}

		time.Sleep(pollInterval)
	}

	// Final check in case an error arrived during the last sleep.
	select {
	case err := <-errCh:
		return fmt.Errorf("%w: %v", errStartFailed, err)
	default:
	}

	return errors.New("timed out waiting for server to become ready")
}
