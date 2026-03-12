package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// responseCapture wraps http.ResponseWriter so the proxy can intercept the
// status code and response body that the upstream sends back to the client.
type responseCapture struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (rc *responseCapture) WriteHeader(status int) {
	rc.status = status
	rc.ResponseWriter.WriteHeader(status)
}

func (rc *responseCapture) Write(b []byte) (int, error) {
	rc.body.Write(b) // capture a copy; ignore error — main writer is authoritative
	return rc.ResponseWriter.Write(b)
}

// Proxy forwards HTTP requests to an upstream server and passes each
// request/response pair to a Recorder.
type Proxy struct {
	upstream *url.URL
	proxy    *httputil.ReverseProxy
	recorder *Recorder
}

// New constructs a Proxy that forwards traffic to upstream and records
// every exchange via recorder.
func New(upstream string, recorder *Recorder) (*Proxy, error) {
	u, err := url.Parse(upstream)
	if err != nil {
		return nil, fmt.Errorf("parsing upstream URL %q: %w", upstream, err)
	}

	rp := httputil.NewSingleHostReverseProxy(u)

	p := &Proxy{
		upstream: u,
		proxy:    rp,
		recorder: recorder,
	}

	return p, nil
}

// ServeHTTP satisfies http.Handler. It reads the request body, forwards the
// request to the upstream via the embedded ReverseProxy, captures the
// response, and records the exchange.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read request body so it can be recorded. Restore it so the proxy can
	// forward it unchanged.
	var reqBody []byte
	if r.Body != nil {
		var err error
		reqBody, err = io.ReadAll(r.Body)
		if err == nil {
			r.Body = io.NopCloser(bytes.NewReader(reqBody))
		}
	}

	// Clone request headers before the reverse proxy modifies them.
	reqHeaders := r.Header.Clone()

	capture := &responseCapture{
		ResponseWriter: w,
		status:         http.StatusOK,
	}

	p.proxy.ServeHTTP(capture, r)

	p.recorder.Record(CapturedExchange{
		Method:      r.Method,
		Path:        r.URL.Path,
		Query:       r.URL.Query(),
		ReqHeaders:  reqHeaders,
		ReqBody:     reqBody,
		Status:      capture.status,
		RespHeaders: capture.Header(),
		RespBody:    capture.body.Bytes(),
	})
}
