package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the absolute path to the repository-level testdata
// directory regardless of the current working directory during tests.
func testdataDir(t *testing.T) string {
	t.Helper()
	// __FILE__ is two directories deep: internal/server → internal → repo root
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// startTestServer spins up a Server on a random port, runs it in the background,
// and registers a cleanup function that shuts it down at test end.
func startTestServer(t *testing.T, cfg *config.MockConfig) *Server {
	t.Helper()
	srv := New(cfg, "127.0.0.1", 0)

	ready := make(chan struct{})
	go func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		srv.mu.Lock()
		addr := ln.Addr().(*net.TCPAddr)
		srv.port = addr.Port
		srv.httpSrv.Addr = addr.String()
		srv.mu.Unlock()

		close(ready)

		if serveErr := srv.httpSrv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			t.Logf("server error: %v", serveErr)
		}
	}()

	<-ready
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})

	return srv
}

// baseURL returns the http://host:port base URL for the test server.
func baseURL(srv *Server) string {
	return "http://" + srv.Addr()
}

func TestServer_BasicResponse(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/health", Status: 200, Body: map[string]interface{}{"status": "ok"}},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
}

func TestServer_PathParams(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/users/{id}", Status: 200, Body: map[string]interface{}{"id": 42}},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/users/42")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	log := srv.RequestLog()
	require.Len(t, log, 1)
	assert.Equal(t, "42", log[0].Params["id"])
}

func TestServer_NotFound(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/health", Status: 200},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/does-not-exist")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// The 404 body now includes available_routes ([]string) alongside string
	// fields, so we decode into a map with interface{} values.
	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "no matching route", body["error"])
	assert.Equal(t, "GET", body["method"])
	assert.Equal(t, "/does-not-exist", body["path"])

	routes, ok := body["available_routes"].([]interface{})
	require.True(t, ok, "expected available_routes to be a list")
	require.Len(t, routes, 1)
	assert.Equal(t, "GET /health", routes[0])
}

func TestServer_DefaultHeaders(t *testing.T) {
	cfg := &config.MockConfig{
		Defaults: config.Defaults{
			Headers: map[string]string{
				"X-Default": "yes",
				"Content-Type": "application/json",
			},
		},
		Routes: []config.Route{
			{Method: "GET", Path: "/ping", Status: 200, Body: `"pong"`},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/ping")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, "yes", resp.Header.Get("X-Default"))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestServer_BodyFile(t *testing.T) {
	bodyFile := filepath.Join(testdataDir(t), "body.json")

	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/file", Status: 200, BodyFile: bodyFile},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/file")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &body))
	fromFile, ok := body["from_file"].(bool)
	require.True(t, ok, "expected from_file to be a bool")
	assert.True(t, fromFile)
}

func TestServer_Reload(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/old", Status: 200, Body: "old"},
		},
	}
	srv := startTestServer(t, cfg)

	// Old route works before reload.
	resp, err := http.Get(baseURL(srv) + "/old")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Reload with a different config.
	newCfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/new", Status: 201, Body: "new"},
		},
	}
	srv.Reload(newCfg)

	// Old route should now 404.
	resp2, err := http.Get(baseURL(srv) + "/old")
	require.NoError(t, err)
	_ = resp2.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp2.StatusCode)

	// New route should work.
	resp3, err := http.Get(baseURL(srv) + "/new")
	require.NoError(t, err)
	_ = resp3.Body.Close()
	assert.Equal(t, http.StatusCreated, resp3.StatusCode)
}

func TestServer_ResetEndpoint(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/ping", Status: 200},
		},
	}
	srv := startTestServer(t, cfg)

	// Generate some log entries.
	for i := 0; i < 3; i++ {
		resp, err := http.Get(baseURL(srv) + "/ping")
		require.NoError(t, err)
		_ = resp.Body.Close()
	}

	require.Len(t, srv.RequestLog(), 3)

	// Call reset.
	resp, err := http.Post(baseURL(srv)+"/__ditto/reset", "application/json", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	assert.Empty(t, srv.RequestLog())
}

func TestServer_ChaosInjection(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/chaos",
				Status: 200,
				Body:   map[string]interface{}{"ok": true},
				Chaos: &config.ChaosConfig{
					Probability: 1.0,
					Status:      503,
					Body:        `{"error":"chaos"}`,
				},
			},
		},
	}
	srv := startTestServer(t, cfg)

	resp, err := http.Get(baseURL(srv) + "/chaos")
	require.NoError(t, err)
	defer resp.Body.Close()

	// With probability=1.0 the chaos response must always fire.
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "chaos", body["error"])
}

func TestServer_DelayFixed(t *testing.T) {
	delay := 50 * time.Millisecond

	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/slow",
				Status: 200,
				Delay:  &config.Delay{Fixed: fmt.Sprintf("%dms", delay.Milliseconds())},
			},
		},
	}
	srv := startTestServer(t, cfg)

	start := time.Now()
	resp, err := http.Get(baseURL(srv) + "/slow")
	elapsed := time.Since(start)

	require.NoError(t, err)
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.GreaterOrEqual(t, elapsed, delay,
		"expected response to take at least %v, took %v", delay, elapsed)
}

// TestServer_ServeHTTP_Direct exercises ServeHTTP via httptest.ResponseRecorder
// so the test does not require a live listener.
func TestServer_ServeHTTP_Direct(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/direct", Status: 202, Body: "ack"},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	req := httptest.NewRequest(http.MethodGet, "/direct", nil)
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusAccepted, rr.Code)
}

// TestServer_Start exercises the Start / Stop lifecycle via a live listener.
func TestServer_Start(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/up", Status: 200},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	// Poll until the server is reachable.
	var (
		resp *http.Response
		err  error
	)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + srv.Addr() + "/up")
		if err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.Stop(ctx))

	select {
	case stopErr := <-errCh:
		assert.NoError(t, stopErr)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}

// TestServer_BodyFile_Missing verifies that a missing body_file returns 500.
func TestServer_BodyFile_Missing(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/missing", Status: 200, BodyFile: "/nonexistent/path/file.json"},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// TestServer_BodyMatchRoute verifies that body-based RequestMatch filtering works.
func TestServer_BodyMatchRoute(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "POST",
				Path:   "/echo",
				Status: 201,
				Body:   map[string]interface{}{"matched": true},
				Match: &config.RequestMatch{
					Body: map[string]interface{}{"name": "*"},
				},
			},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	t.Run("body matches", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"name":"alice"}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusCreated, rr.Code)
	})

	t.Run("body does not match", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"age":30}`))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		srv.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})
}

// TestServer_Start_AddressInUse verifies that Start returns a descriptive error
// when the port is already occupied, including a suggestion to use the next port.
func TestServer_Start_AddressInUse(t *testing.T) {
	// Bind a listener to occupy a port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/health", Status: 200},
		},
	}
	srv := New(cfg, "127.0.0.1", port)
	startErr := srv.Start()
	require.Error(t, startErr)
	assert.Contains(t, startErr.Error(), "already in use")
	assert.Contains(t, startErr.Error(), fmt.Sprintf("%d", port+1), "error should suggest next port")
}

// TestServer_OnRequest verifies that the OnRequest callback is invoked after
// each handled request with the correct method, path, and status.
func TestServer_OnRequest(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/ping", Status: 200},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	var entries []RequestEntry
	srv.OnRequest = func(e RequestEntry) {
		entries = append(entries, e)
	}

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	require.Len(t, entries, 1)
	assert.Equal(t, "GET", entries[0].Method)
	assert.Equal(t, "/ping", entries[0].Path)
	assert.Equal(t, http.StatusOK, entries[0].Status)
	assert.Greater(t, entries[0].Latency, time.Duration(0))
}

// TestServer_OnRequest_NotFound verifies the callback fires on 404 responses.
func TestServer_OnRequest_NotFound(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/exists", Status: 200},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	var called bool
	srv.OnRequest = func(e RequestEntry) {
		called = true
		assert.Equal(t, http.StatusNotFound, e.Status)
	}

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.True(t, called, "OnRequest should be called for 404 responses")
}

// TestServer_RoutesEndpoint verifies that GET /__ditto/routes returns the
// configured routes and current scenario states as JSON.
func TestServer_RoutesEndpoint(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/health", Status: 200},
			{Method: "POST", Path: "/users", Status: 201},
		},
		Scenarios: []config.Scenario{
			{
				Name: "cart-flow",
				Steps: []config.ScenarioStep{
					{On: "POST /cart", SetState: "item_added", Status: 200},
				},
			},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	req := httptest.NewRequest(http.MethodGet, "/__ditto/routes", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))

	routes, ok := body["routes"].([]interface{})
	require.True(t, ok, "routes must be an array")
	require.Len(t, routes, 2)

	first := routes[0].(map[string]interface{})
	assert.Equal(t, "GET", first["method"])
	assert.Equal(t, "/health", first["path"])
	assert.Equal(t, float64(200), first["status"])

	scenarios, ok := body["scenarios"].([]interface{})
	require.True(t, ok, "scenarios must be an array")
	require.Len(t, scenarios, 1)

	sc := scenarios[0].(map[string]interface{})
	assert.Equal(t, "cart-flow", sc["name"])
	assert.Equal(t, float64(1), sc["steps"])
	assert.Equal(t, "", sc["state"])
}

// TestServer_NotFound_AvailableRoutes verifies that a 404 body includes the
// available_routes field listing up to 10 routes.
func TestServer_NotFound_AvailableRoutes(t *testing.T) {
	routes := make([]config.Route, 12)
	for i := range routes {
		routes[i] = config.Route{
			Method: "GET",
			Path:   fmt.Sprintf("/route%d", i),
			Status: 200,
		}
	}
	cfg := &config.MockConfig{Routes: routes}
	srv := New(cfg, "127.0.0.1", 0)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)

	var body map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))

	available, ok := body["available_routes"].([]interface{})
	require.True(t, ok, "available_routes must be an array")
	// Capped at 10 even though there are 12 routes.
	assert.Len(t, available, 10)
}

// TestServer_RouteHeaders verifies that route-specific headers are applied.
func TestServer_RouteHeaders(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method:  "GET",
				Path:    "/tagged",
				Status:  200,
				Headers: map[string]string{"X-Route": "custom"},
			},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	req := httptest.NewRequest(http.MethodGet, "/tagged", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "custom", rr.Header().Get("X-Route"))
}

// TestServer_DefaultDelay verifies that a default delay in Defaults is applied.
func TestServer_DefaultDelay(t *testing.T) {
	cfg := &config.MockConfig{
		Defaults: config.Defaults{
			Delay: &config.Delay{Fixed: "30ms"},
		},
		Routes: []config.Route{
			{Method: "GET", Path: "/slow", Status: 200},
		},
	}
	srv := New(cfg, "127.0.0.1", 0)

	start := time.Now()
	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.GreaterOrEqual(t, elapsed, 30*time.Millisecond)
}
