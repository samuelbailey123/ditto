package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/samuelbailey123/ditto/internal/config"
)

// RequestEntry records metadata about a single handled request.
type RequestEntry struct {
	Method    string
	Path      string
	Status    int
	Latency   time.Duration
	Timestamp time.Time
	Params    map[string]string
}

// Server is an HTTP mock server driven by a MockConfig.
type Server struct {
	mu        sync.RWMutex
	cfg       *config.MockConfig
	router    *Router
	state     *StateEngine
	httpSrv   *http.Server
	reqLog    []RequestEntry
	host      string
	port      int
	OnRequest func(entry RequestEntry) // optional callback, called after each request
}

// New constructs a Server but does not start listening.
func New(cfg *config.MockConfig, host string, port int) *Server {
	s := &Server{
		cfg:    cfg,
		router: NewRouter(cfg.Routes),
		state:  NewStateEngine(cfg.Scenarios),
		host:   host,
		port:   port,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.ServeHTTP)

	s.httpSrv = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", host, port),
		Handler: corsMiddleware(cfg.Defaults.Cors, mux),
	}

	return s
}

// Start begins listening and serving. It blocks until the server is stopped.
// The server's actual address (including OS-assigned port when port is 0) is
// available via Addr() after Start returns its first byte from the listener.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.httpSrv.Addr)
	if err != nil {
		if strings.Contains(err.Error(), "address already in use") {
			return fmt.Errorf("port %d is already in use\n\nTry:\n  ditto serve --port %d api.yaml\n  lsof -ti:%d | xargs kill",
				s.port, s.port+1, s.port)
		}
		return fmt.Errorf("listening on %s: %w", s.httpSrv.Addr, err)
	}

	// Record the actual bound address so Addr() is accurate when port 0 is used.
	s.mu.Lock()
	addr := ln.Addr().(*net.TCPAddr)
	s.port = addr.Port
	s.httpSrv.Addr = addr.String()
	s.mu.Unlock()

	if err := s.httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the server within the given context deadline.
func (s *Server) Stop(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}

// Reload atomically replaces the active configuration, rebuilds the router,
// and recreates the state engine with the new scenario definitions.
func (s *Server) Reload(cfg *config.MockConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = cfg
	s.router = NewRouter(cfg.Routes)
	s.state = NewStateEngine(cfg.Scenarios)

	// Rebuild the handler chain so CORS config is also refreshed.
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.ServeHTTP)
	s.httpSrv.Handler = corsMiddleware(cfg.Defaults.Cors, mux)
}

// Addr returns the "host:port" the server is (or will be) listening on.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("%s:%d", s.host, s.port)
}

// RequestLog returns a snapshot copy of all logged request entries.
func (s *Server) RequestLog() []RequestEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RequestEntry, len(s.reqLog))
	copy(out, s.reqLog)
	return out
}

// ServeHTTP implements http.Handler. It is the main dispatch loop.
//
// Resolution order:
//  1. Control endpoints /__ditto/* — handled immediately.
//  2. Scenario steps — evaluated by the state engine before static routes so
//     stateful overrides take priority.
//  3. Static routes — matched by the router as usual.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Control endpoint: reset request log and scenario states.
	if r.Method == http.MethodPost && r.URL.Path == "/__ditto/reset" {
		s.mu.Lock()
		s.reqLog = s.reqLog[:0]
		s.mu.Unlock()
		s.state.Reset()
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Control endpoint: list configured routes and scenario state.
	if r.Method == http.MethodGet && r.URL.Path == "/__ditto/routes" {
		s.handleRoutesEndpoint(w)
		return
	}

	// Read body once so both routing and handling can use it.
	var body []byte
	if r.Body != nil {
		data, err := io.ReadAll(r.Body)
		if err == nil {
			body = data
		}
		_ = r.Body.Close()
	}

	s.mu.RLock()
	router := s.router
	cfg := s.cfg
	s.mu.RUnlock()

	start := time.Now()

	// Scenario steps take priority over static routes.
	if scenarioResp, ok := s.state.Match(r.Method, r.URL.Path); ok {
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		handleScenarioResponse(rw, scenarioResp)
		s.logRequest(r, rw.status, time.Since(start), nil)
		return
	}

	route, params := router.MatchWithBody(r, body)
	if route == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		available := availableRoutes(cfg.Routes, 10)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error":            "no matching route",
			"method":           r.Method,
			"path":             r.URL.Path,
			"available_routes": available,
		})
		s.logRequest(r, http.StatusNotFound, time.Since(start), nil)
		return
	}

	rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

	// Build the innermost handler that actually writes the response.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRoute(w, r, route, params, cfg.Defaults)
	})

	// Wrap with chaos, then delay (delay runs first, chaos runs after delay).
	handler := chaosMiddleware(route.Chaos, inner)

	// Merge delay: route-level overrides defaults.
	delay := cfg.Defaults.Delay
	if route.Delay != nil {
		delay = route.Delay
	}
	handler = delayMiddleware(delay, handler)

	handler(rw, r)

	latency := time.Since(start)
	s.logRequest(r, rw.status, latency, params)
}

// handleScenarioResponse writes the HTTP response defined by a matched
// scenario step. Header and Content-Type resolution follows the same rules as
// handleRoute but does not apply defaults (scenarios are self-contained).
func handleScenarioResponse(w http.ResponseWriter, sr *ScenarioResponse) {
	for k, v := range sr.Headers {
		w.Header().Set(k, v)
	}

	var body []byte
	if sr.Body != nil {
		s, err := sr.bodyAsString()
		if err != nil {
			http.Error(w, "encoding scenario body: "+err.Error(), http.StatusInternalServerError)
			return
		}
		body = []byte(s)
	}

	if len(body) > 0 {
		ct := resolveContentType(sr.Headers, body)
		if ct != "" {
			if _, exists := sr.Headers["Content-Type"]; !exists {
				w.Header().Set("Content-Type", ct)
			}
		}
	}

	w.WriteHeader(sr.Status)

	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// logRequest appends an entry to the request log and fires the OnRequest
// callback (if set) outside of the lock to avoid blocking the server loop.
func (s *Server) logRequest(r *http.Request, status int, latency time.Duration, params map[string]string) {
	entry := RequestEntry{
		Method:    r.Method,
		Path:      r.URL.Path,
		Status:    status,
		Latency:   latency,
		Timestamp: time.Now(),
		Params:    params,
	}

	s.mu.Lock()
	s.reqLog = append(s.reqLog, entry)
	cb := s.OnRequest
	s.mu.Unlock()

	if cb != nil {
		cb(entry)
	}
}

// routeInfo is the shape used by /__ditto/routes.
type routeInfo struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`
}

// scenarioInfo is the shape used by /__ditto/routes.
type scenarioInfo struct {
	Name  string `json:"name"`
	Steps int    `json:"steps"`
	State string `json:"state"`
}

// handleRoutesEndpoint writes the /__ditto/routes JSON response.
func (s *Server) handleRoutesEndpoint(w http.ResponseWriter) {
	s.mu.RLock()
	cfg := s.cfg
	s.mu.RUnlock()

	states := s.state.States()

	routes := make([]routeInfo, len(cfg.Routes))
	for i, r := range cfg.Routes {
		routes[i] = routeInfo{
			Method: strings.ToUpper(r.Method),
			Path:   r.Path,
			Status: r.Status,
		}
	}

	scenarios := make([]scenarioInfo, len(cfg.Scenarios))
	for i, sc := range cfg.Scenarios {
		scenarios[i] = scenarioInfo{
			Name:  sc.Name,
			Steps: len(sc.Steps),
			State: states[sc.Name],
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"routes":    routes,
		"scenarios": scenarios,
	})
}

// availableRoutes returns up to limit route strings formatted as "METHOD /path".
func availableRoutes(routes []config.Route, limit int) []string {
	out := make([]string, 0, min(len(routes), limit))
	for i, r := range routes {
		if i >= limit {
			break
		}
		out = append(out, strings.ToUpper(r.Method)+" "+r.Path)
	}
	return out
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// statusRecorder wraps ResponseWriter to capture the written status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}
