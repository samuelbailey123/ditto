package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// TemplateContext holds the per-request data available inside response body templates.
type TemplateContext struct {
	// Params holds path parameters extracted from the route pattern, e.g. {name}.
	Params map[string]string
	// Query holds the raw query string values keyed by parameter name.
	Query map[string][]string
	// Headers holds the incoming request headers.
	Headers http.Header
	// Method is the HTTP method of the request (e.g. "GET").
	Method string
	// Path is the URL path of the request (e.g. "/greet/alice").
	Path string
}

// templateFuncs returns the custom function map made available to every template.
var templateFuncs = template.FuncMap{
	// now returns the current wall-clock time formatted as RFC3339.
	"now": func() string {
		return time.Now().UTC().Format(time.RFC3339)
	},

	// uuid returns a randomly generated UUID v4 string.
	"uuid": func() (string, error) {
		var b [16]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", fmt.Errorf("generating uuid: %w", err)
		}
		// Set version 4 and variant bits per RFC 4122.
		b[6] = (b[6] & 0x0f) | 0x40
		b[8] = (b[8] & 0x3f) | 0x80
		return fmt.Sprintf(
			"%08x-%04x-%04x-%04x-%012x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:],
		), nil
	},

	// seq returns a slice of ints [0, n) for use in range loops.
	"seq": func(n int) []int {
		s := make([]int, n)
		for i := range s {
			s[i] = i
		}
		return s
	},

	// json marshals v to a JSON string. Returns an error string on failure.
	"json": func(v any) (string, error) {
		b, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("marshalling json: %w", err)
		}
		return string(b), nil
	},

	// upper converts s to upper case.
	"upper": strings.ToUpper,

	// lower converts s to lower case.
	"lower": strings.ToLower,

	// default returns value (converted to string) if it is non-empty, otherwise
	// fallback. Accepting value as any handles the case where the template
	// engine passes a zero reflect.Value for a missing map key.
	"default": func(value any, fallback string) string {
		if value == nil {
			return fallback
		}
		s, ok := value.(string)
		if !ok || s == "" {
			return fallback
		}
		return s
	},
}

// renderTemplate parses body as a Go text/template, executes it with ctx as
// the data object, and returns the rendered string.
//
// If the template cannot be parsed or executed, the original body is returned
// unchanged (fail-open) so that non-template bodies that happen to contain
// curly braces are never silently dropped.
func renderTemplate(body string, ctx TemplateContext) (string, error) {
	tmpl, err := template.New("body").Funcs(templateFuncs).Parse(body)
	if err != nil {
		// Fail-open: return original body on parse error.
		return body, nil
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		// Fail-open: return original body on execution error.
		return body, nil
	}

	return buf.String(), nil
}
