package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// corsMiddleware
// ---------------------------------------------------------------------------

func TestCorsMiddleware_Permissive(t *testing.T) {
	handler := corsMiddleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "*", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestCorsMiddleware_Preflight(t *testing.T) {
	handler := corsMiddleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Should not be called for OPTIONS.
		t.Error("next handler should not be called for preflight")
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
}

func TestCorsMiddleware_CustomConfig(t *testing.T) {
	cfg := &config.CorsConfig{
		Origins: []string{"https://example.com"},
		Methods: []string{"GET", "POST"},
		Headers: []string{"Content-Type"},
	}
	handler := corsMiddleware(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, "https://example.com", rr.Header().Get("Access-Control-Allow-Origin"))
	assert.Equal(t, "GET, POST", rr.Header().Get("Access-Control-Allow-Methods"))
	assert.Equal(t, "Content-Type", rr.Header().Get("Access-Control-Allow-Headers"))
}

// ---------------------------------------------------------------------------
// delayMiddleware
// ---------------------------------------------------------------------------

func TestDelayMiddleware_Nil(t *testing.T) {
	called := false
	handler := delayMiddleware(nil, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.True(t, called)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestDelayMiddleware_Fixed(t *testing.T) {
	delay := &config.Delay{Fixed: "30ms"}
	handler := delayMiddleware(delay, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 30*time.Millisecond)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestDelayMiddleware_MinMax(t *testing.T) {
	delay := &config.Delay{Min: "10ms", Max: "30ms"}
	handler := delayMiddleware(delay, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	elapsed := time.Since(start)

	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestDelayMiddleware_InvalidFixed(t *testing.T) {
	// A malformed duration string must not panic; next is still called.
	delay := &config.Delay{Fixed: "notaduration"}
	called := false
	handler := delayMiddleware(delay, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.True(t, called)
}

// ---------------------------------------------------------------------------
// chaosMiddleware
// ---------------------------------------------------------------------------

func TestChaosMiddleware_Nil(t *testing.T) {
	called := false
	handler := chaosMiddleware(nil, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.True(t, called)
}

func TestChaosMiddleware_ZeroProbability(t *testing.T) {
	chaos := &config.ChaosConfig{Probability: 0, Status: 500}
	called := false
	handler := chaosMiddleware(chaos, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.True(t, called, "next should always be called with probability=0")
}

func TestChaosMiddleware_AlwaysFires(t *testing.T) {
	chaos := &config.ChaosConfig{
		Probability: 1.0,
		Status:      503,
		Body:        `{"error":"chaos"}`,
	}
	handler := chaosMiddleware(chaos, func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not be called when chaos fires")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.Contains(t, rr.Body.String(), "chaos")
}

func TestChaosMiddleware_NoBody(t *testing.T) {
	chaos := &config.ChaosConfig{Probability: 1.0, Status: 500}
	handler := chaosMiddleware(chaos, func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Empty(t, rr.Body.String())
}

func TestChaosMiddleware_ZeroStatus_DefaultsTo500(t *testing.T) {
	chaos := &config.ChaosConfig{Probability: 1.0, Status: 0}
	handler := chaosMiddleware(chaos, func(w http.ResponseWriter, r *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestChaosMiddleware_LowProbability_CallsNext verifies that when probability
// is very low the next handler can still be invoked. We force the rand output
// by using probability=0.0000001 — the only reliable way without seeding the
// global rand from outside is to rely on the zero-probability guard. This test
// ensures the non-chaos path is reachable at all.
func TestChaosMiddleware_NonFireBranch(t *testing.T) {
	// Use a very low probability; over enough calls at least one should pass
	// through. Run 1000 iterations to guarantee the branch is hit.
	chaos := &config.ChaosConfig{Probability: 0.001, Status: 500}
	nextCalled := 0

	handler := chaosMiddleware(chaos, func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 1000; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler(rr, req)
	}
	assert.Greater(t, nextCalled, 0, "expected next to be called at least once")
}
