package proxy

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/samuelbailey123/ditto/internal/config"
)

// jsonExchange returns a CapturedExchange with a JSON response body and the
// application/json Content-Type header, suitable for use in multiple tests.
func jsonExchange(method, path string, status int, body string) CapturedExchange {
	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	return CapturedExchange{
		Method:      method,
		Path:        path,
		Status:      status,
		RespHeaders: h,
		RespBody:    []byte(body),
	}
}

func TestRecorder_Record(t *testing.T) {
	r := NewRecorder()
	r.Record(jsonExchange("GET", "/health", 200, `{"status":"ok"}`))
	assert.Equal(t, 1, r.Count())
}

func TestRecorder_ToMockFile(t *testing.T) {
	r := NewRecorder()
	r.Record(jsonExchange("GET", "/users", 200, `[{"id":1}]`))
	r.Record(jsonExchange("POST", "/users", 201, `{"id":2}`))

	mf := r.ToMockFile("test-api")

	assert.Equal(t, "test-api", mf.Name)
	require.Len(t, mf.Routes, 2)

	// Verify first route fields.
	assert.Equal(t, "GET", mf.Routes[0].Method)
	assert.Equal(t, "/users", mf.Routes[0].Path)
	assert.Equal(t, 200, mf.Routes[0].Status)
	assert.NotNil(t, mf.Routes[0].Body)

	// All responses are JSON so Content-Type should be promoted to defaults.
	assert.Equal(t, "application/json", mf.Defaults.Headers["Content-Type"])

	// Individual routes must not repeat Content-Type when it lives in defaults.
	for _, route := range mf.Routes {
		_, hasContentType := route.Headers["Content-Type"]
		assert.False(t, hasContentType, "route %s %s should not repeat Content-Type already in defaults", route.Method, route.Path)
	}
}

func TestRecorder_DeduplicatesRoutes(t *testing.T) {
	r := NewRecorder()
	// Record the same method+path+status twice — second should be ignored.
	r.Record(jsonExchange("GET", "/ping", 200, `{"first":true}`))
	r.Record(jsonExchange("GET", "/ping", 200, `{"second":true}`))

	mf := r.ToMockFile("dedup-test")

	require.Len(t, mf.Routes, 1, "duplicate route should be deduplicated")

	// The first recorded body must be retained.
	body, ok := mf.Routes[0].Body.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, body["first"])
}

func TestRecorder_WriteYAML(t *testing.T) {
	r := NewRecorder()
	r.Record(jsonExchange("GET", "/status", 200, `{"alive":true}`))

	var buf bytes.Buffer
	err := r.WriteYAML(&buf)
	require.NoError(t, err)

	// The output must be parseable back into a MockFile.
	var mf config.MockFile
	err = yaml.Unmarshal(buf.Bytes(), &mf)
	require.NoError(t, err)

	require.Len(t, mf.Routes, 1)
	assert.Equal(t, "GET", mf.Routes[0].Method)
	assert.Equal(t, "/status", mf.Routes[0].Path)
	assert.Equal(t, 200, mf.Routes[0].Status)

	// Sanity-check that the YAML output is non-empty and looks like YAML.
	assert.True(t, strings.Contains(buf.String(), "routes:"))
}

// TestRecorder_NonJSONBody verifies that a plain-text response body is stored
// as a string rather than a parsed JSON value.
func TestRecorder_NonJSONBody(t *testing.T) {
	r := NewRecorder()

	h := make(http.Header)
	h.Set("Content-Type", "text/plain")
	r.Record(CapturedExchange{
		Method:      "GET",
		Path:        "/text",
		Status:      200,
		RespHeaders: h,
		RespBody:    []byte("plain text response"),
	})

	mf := r.ToMockFile("text-test")
	require.Len(t, mf.Routes, 1)
	assert.Equal(t, "plain text response", mf.Routes[0].Body)
}

// TestRecorder_MixedContentTypes verifies that Content-Type is NOT promoted
// to defaults when not all responses are JSON.
func TestRecorder_MixedContentTypes(t *testing.T) {
	r := NewRecorder()
	r.Record(jsonExchange("GET", "/json", 200, `{}`))

	textH := make(http.Header)
	textH.Set("Content-Type", "text/plain")
	r.Record(CapturedExchange{
		Method:      "GET",
		Path:        "/text",
		Status:      200,
		RespHeaders: textH,
		RespBody:    []byte("ok"),
	})

	mf := r.ToMockFile("mixed")
	assert.Nil(t, mf.Defaults.Headers, "defaults should not include Content-Type when responses are mixed")
}

// TestRecorder_EmptyBody verifies that an exchange with no response body
// results in a nil route body rather than an empty string.
func TestRecorder_EmptyBody(t *testing.T) {
	r := NewRecorder()

	h := make(http.Header)
	h.Set("Content-Type", "application/json")
	r.Record(CapturedExchange{
		Method:      "DELETE",
		Path:        "/item/1",
		Status:      204,
		RespHeaders: h,
		RespBody:    nil,
	})

	mf := r.ToMockFile("empty-body")
	require.Len(t, mf.Routes, 1)
	assert.Nil(t, mf.Routes[0].Body)
}

// TestRecorder_ToMockFile_Empty verifies that converting an empty recorder
// returns a MockFile with no routes and no defaults.
func TestRecorder_ToMockFile_Empty(t *testing.T) {
	r := NewRecorder()
	mf := r.ToMockFile("empty")
	assert.Empty(t, mf.Routes)
	assert.Nil(t, mf.Defaults.Headers)
}

// errWriter is an io.Writer that always returns an error after the first
// write so we can exercise the WriteYAML error branch.
type errWriter struct {
	written bool
}

func (e *errWriter) Write(p []byte) (int, error) {
	if e.written {
		return 0, io.ErrClosedPipe
	}
	e.written = true
	return len(p), nil
}

func TestRecorder_WriteYAML_EncoderError(t *testing.T) {
	r := NewRecorder()
	r.Record(jsonExchange("GET", "/err", 200, strings.Repeat(`{"x":1},`, 500)))

	err := r.WriteYAML(&errWriter{})
	assert.Error(t, err)
}
