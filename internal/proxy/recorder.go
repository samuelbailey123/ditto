package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/samuelbailey123/ditto/internal/config"
)

// CapturedExchange holds the full request/response pair for one HTTP interaction.
type CapturedExchange struct {
	Method      string
	Path        string
	Query       url.Values
	ReqHeaders  http.Header
	ReqBody     []byte
	Status      int
	RespHeaders http.Header
	RespBody    []byte
}

// Recorder captures HTTP exchanges in memory and can convert them to a MockFile.
type Recorder struct {
	mu        sync.Mutex
	exchanges []CapturedExchange
}

// NewRecorder returns an initialised Recorder ready to capture traffic.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// Record appends an exchange to the recorder. It is safe for concurrent use.
func (r *Recorder) Record(ex CapturedExchange) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.exchanges = append(r.exchanges, ex)
}

// Count returns the number of exchanges recorded so far.
func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.exchanges)
}

// ToMockFile converts all captured exchanges into a MockFile.
//
// Deduplication: only the first exchange for each (method, path, status)
// triple is kept. The response body is parsed as JSON where possible so that
// the YAML output is structured rather than a raw string. The name field is
// set to the provided name argument.
//
// If every recorded response carries a JSON Content-Type, the mock file's
// defaults will include Content-Type: application/json so individual routes
// do not need to repeat it.
func (r *Recorder) ToMockFile(name string) *config.MockFile {
	r.mu.Lock()
	defer r.mu.Unlock()

	type dedupKey struct {
		method string
		path   string
		status int
	}

	seen := make(map[dedupKey]struct{})
	allJSON := true
	var routes []config.Route

	for _, ex := range r.exchanges {
		key := dedupKey{method: ex.Method, path: ex.Path, status: ex.Status}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		ct := ex.RespHeaders.Get("Content-Type")
		isJSON := strings.Contains(ct, "application/json")
		if !isJSON {
			allJSON = false
		}

		var body interface{}
		if len(ex.RespBody) > 0 {
			var parsed interface{}
			if json.Unmarshal(ex.RespBody, &parsed) == nil {
				body = parsed
			} else {
				body = string(ex.RespBody)
			}
		}

		// Copy per-route headers, omitting Content-Type when it will be
		// promoted to defaults later (decision finalised after the loop).
		headers := make(map[string]string)
		for k, vals := range ex.RespHeaders {
			if len(vals) > 0 {
				headers[k] = vals[0]
			}
		}

		routes = append(routes, config.Route{
			Method:  ex.Method,
			Path:    ex.Path,
			Status:  ex.Status,
			Body:    body,
			Headers: headers,
		})
	}

	mf := &config.MockFile{
		Name:   name,
		Routes: routes,
	}

	if allJSON && len(routes) > 0 {
		// Promote Content-Type to defaults and strip it from individual routes.
		mf.Defaults = config.Defaults{
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}
		for i := range mf.Routes {
			delete(mf.Routes[i].Headers, "Content-Type")
			if len(mf.Routes[i].Headers) == 0 {
				mf.Routes[i].Headers = nil
			}
		}
	}

	return mf
}

// WriteYAML marshals the recorded exchanges as a MockFile to w.
// The mock file name is derived from the number of exchanges captured.
func (r *Recorder) WriteYAML(w io.Writer) error {
	name := fmt.Sprintf("recorded-%d-routes", r.Count())
	mf := r.ToMockFile(name)

	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(mf); err != nil {
		return fmt.Errorf("encoding YAML: %w", err)
	}
	return enc.Close()
}
