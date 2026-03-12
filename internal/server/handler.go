package server

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/samuelbailey123/ditto/internal/config"
)

// handleRoute writes the HTTP response defined by route to w.
//
// Header resolution order (later wins):
//  1. defaults.Headers from the global config
//  2. route.Headers
//
// Body resolution order:
//  1. BodyFile — contents of the file at the given path (relative to CWD)
//  2. Body — marshalled via route.BodyAsString()
//  3. No body — only the status code is written
//
// If the resolved body string contains "{{", it is treated as a Go
// text/template and rendered with a TemplateContext built from the request.
// Template rendering is fail-open: on any error the original body is used.
//
// Content-Type is set automatically when the body looks like JSON and no
// explicit Content-Type header has been declared.
func handleRoute(
	w http.ResponseWriter,
	r *http.Request,
	route *config.Route,
	params map[string]string,
	defaults config.Defaults,
) {
	// Build merged header map: defaults first, then route overrides.
	headers := make(map[string]string, len(defaults.Headers)+len(route.Headers))
	for k, v := range defaults.Headers {
		headers[k] = v
	}
	for k, v := range route.Headers {
		headers[k] = v
	}

	var (
		body        []byte
		contentType string
	)

	switch {
	case route.BodyFile != "":
		data, err := os.ReadFile(route.BodyFile)
		if err != nil {
			http.Error(w, fmt.Sprintf("reading body_file %q: %v", route.BodyFile, err), http.StatusInternalServerError)
			return
		}
		body = data

	case route.Body != nil:
		s, err := route.BodyAsString()
		if err != nil {
			http.Error(w, fmt.Sprintf("encoding body: %v", err), http.StatusInternalServerError)
			return
		}
		body = []byte(s)
	}

	// Render the body as a Go template when it contains template markers.
	if len(body) > 0 && strings.Contains(string(body), "{{") {
		ctx := TemplateContext{
			Params:  params,
			Query:   r.URL.Query(),
			Headers: r.Header,
			Method:  r.Method,
			Path:    r.URL.Path,
		}
		rendered, _ := renderTemplate(string(body), ctx)
		body = []byte(rendered)
	}

	// Auto-detect Content-Type when the body looks like JSON and none is set.
	if len(body) > 0 {
		contentType = resolveContentType(headers, body)
	}

	// Write headers before WriteHeader/Write.
	for k, v := range headers {
		w.Header().Set(k, v)
	}
	if contentType != "" {
		if _, exists := headers["Content-Type"]; !exists {
			w.Header().Set("Content-Type", contentType)
		}
	}

	status := route.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// resolveContentType returns the Content-Type to use for the response.
// If headers already include a Content-Type it is returned unchanged.
// Otherwise, if the body starts with a JSON marker, "application/json" is
// returned.
func resolveContentType(headers map[string]string, body []byte) string {
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Type") {
			return v
		}
	}

	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "application/json"
	}
	return ""
}
