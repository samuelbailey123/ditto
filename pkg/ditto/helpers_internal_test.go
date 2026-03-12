// White-box tests for unexported helpers. This file belongs to package ditto
// (not ditto_test) so it can access unexported symbols directly.
package ditto

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// pollUntilReady
// ---------------------------------------------------------------------------

// TestPollUntilReady_Success verifies the happy path: nil is returned when the
// target server is already listening.
func TestPollUntilReady_Success(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hs.Close)

	err := pollUntilReady(hs.URL, make(chan error, 1))
	assert.NoError(t, err)
}

// TestPollUntilReady_ErrCh_Immediate verifies that pollUntilReady returns
// errStartFailed when the errCh already holds an error on first check.
func TestPollUntilReady_ErrCh_Immediate(t *testing.T) {
	errCh := make(chan error, 1)
	errCh <- errors.New("bind failed")

	// Port 1 on loopback is never open; every probe fails instantly.
	err := pollUntilReady("http://127.0.0.1:1", errCh)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errStartFailed),
		"expected errStartFailed, got: %v", err)
	assert.Contains(t, err.Error(), "bind failed")
}

// TestPollUntilReady_Timeout verifies that pollUntilReady returns a timeout
// error when the server never becomes reachable. pollDeadline is temporarily
// shrunk to 30ms so the test runs fast.
func TestPollUntilReady_Timeout(t *testing.T) {
	orig := pollDeadline
	pollDeadline = 30 * time.Millisecond
	t.Cleanup(func() { pollDeadline = orig })

	// Port 1 never accepts connections.
	err := pollUntilReady("http://127.0.0.1:1", make(chan error))

	require.Error(t, err)
	assert.False(t, errors.Is(err, errStartFailed),
		"timeout should not wrap errStartFailed")
	assert.Contains(t, err.Error(), "timed out")
}

// TestPollUntilReady_ErrCh_FinalCheck verifies the post-loop errCh drain: when
// the deadline is set to zero the loop body never executes, so an error that is
// already in the channel is caught by the final select — exercising line 77.
func TestPollUntilReady_ErrCh_FinalCheck(t *testing.T) {
	orig := pollDeadline
	// Setting pollDeadline to 0 means time.Now().Before(deadline) is
	// immediately false, so the loop body never runs and we fall straight
	// through to the post-loop select.
	pollDeadline = 0
	t.Cleanup(func() { pollDeadline = orig })

	errCh := make(chan error, 1)
	errCh <- errors.New("late error") // pre-fill; caught by the final select

	err := pollUntilReady("http://127.0.0.1:1", errCh)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errStartFailed),
		"post-loop select should wrap errStartFailed, got: %v", err)
	assert.Contains(t, err.Error(), "late error")
}

// ---------------------------------------------------------------------------
// waitReady
// ---------------------------------------------------------------------------

// tbFatal is a minimal testing.TB stub for white-box tests of waitReady.
// It records fatal calls and panics with a sentinel so the caller can recover.
type tbFatal struct {
	testing.TB
	mu          sync.Mutex
	fatalCalled bool
	lastMsg     string
}

func (f *tbFatal) Helper() {}

func (f *tbFatal) Fatalf(format string, args ...any) {
	f.mu.Lock()
	f.fatalCalled = true
	f.mu.Unlock()
	panic(fatalSignal{msg: ""}) // fatalSignal defined in ditto_test.go
}

// TestWaitReady_Success verifies that waitReady does not call t.Fatal when the
// server is already listening.
func TestWaitReady_Success(t *testing.T) {
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(hs.Close)

	// Should not panic or call Fatal.
	stub := &tbFatal{}
	waitReady(stub, hs.URL, make(chan error, 1))
	assert.False(t, stub.fatalCalled)
}

// TestWaitReady_ErrStartFailed verifies that waitReady calls t.Fatalf with the
// "server failed to start" message when pollUntilReady returns errStartFailed.
func TestWaitReady_ErrStartFailed(t *testing.T) {
	orig := pollDeadline
	pollDeadline = 30 * time.Millisecond
	t.Cleanup(func() { pollDeadline = orig })

	errCh := make(chan error, 1)
	errCh <- errors.New("bind failed")

	stub := &tbFatal{}
	fatalOccurred := func() (called bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); ok {
					called = true
				} else {
					panic(r)
				}
			}
		}()
		waitReady(stub, "http://127.0.0.1:1", errCh)
		return false
	}()

	assert.True(t, fatalOccurred)
	assert.True(t, stub.fatalCalled)
}

// TestWaitReady_Timeout verifies that waitReady calls t.Fatalf with the
// "server did not become ready" message on deadline expiry.
func TestWaitReady_Timeout(t *testing.T) {
	orig := pollDeadline
	pollDeadline = 30 * time.Millisecond
	t.Cleanup(func() { pollDeadline = orig })

	stub := &tbFatal{}
	fatalOccurred := func() (called bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(fatalSignal); ok {
					called = true
				} else {
					panic(r)
				}
			}
		}()
		waitReady(stub, "http://127.0.0.1:1", make(chan error))
		return false
	}()

	assert.True(t, fatalOccurred)
	assert.True(t, stub.fatalCalled)
}

// ---------------------------------------------------------------------------
// newShutdownCtx
// ---------------------------------------------------------------------------

// TestNewShutdownCtx_Deadline verifies that newShutdownCtx returns a context
// whose deadline lies within the expected shutdownTimeout window.
func TestNewShutdownCtx_Deadline(t *testing.T) {
	before := time.Now()
	ctx := newShutdownCtx()
	after := time.Now()

	require.NotNil(t, ctx)

	deadline, ok := ctx.Deadline()
	require.True(t, ok, "shutdown context must have a deadline")

	assert.True(t, deadline.After(before.Add(shutdownTimeout-time.Millisecond)),
		"deadline should be at least shutdownTimeout from the call time")
	assert.True(t, deadline.Before(after.Add(shutdownTimeout+time.Second)),
		"deadline should not exceed shutdownTimeout by more than one second")
}
