package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUpstream starts a test HTTP server that responds to every request with
// the given status, body, and Content-Type header.
func newUpstream(t *testing.T, status int, body, contentType string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		io.WriteString(w, body) //nolint:errcheck
	}))
}

func TestNew_InvalidURL(t *testing.T) {
	_, err := New("://bad-url", NewRecorder())
	assert.Error(t, err)
}

func TestNew_ValidURL(t *testing.T) {
	p, err := New("http://localhost:9999", NewRecorder())
	require.NoError(t, err)
	assert.NotNil(t, p)
}

func TestProxy_ServeHTTP_ForwardsRequest(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, `{"alive":true}`, "application/json")
	defer upstream.Close()

	rec := NewRecorder()
	p, err := New(upstream.URL, rec)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rw := httptest.NewRecorder()
	p.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusOK, rw.Code)
	assert.Contains(t, rw.Body.String(), "alive")

	require.Equal(t, 1, rec.Count())
	ex := rec.exchanges[0]
	assert.Equal(t, "GET", ex.Method)
	assert.Equal(t, "/health", ex.Path)
	assert.Equal(t, http.StatusOK, ex.Status)
	assert.Equal(t, `{"alive":true}`, string(ex.RespBody))
}

func TestProxy_ServeHTTP_RecordsRequestBody(t *testing.T) {
	upstream := newUpstream(t, http.StatusCreated, `{"id":1}`, "application/json")
	defer upstream.Close()

	rec := NewRecorder()
	p, err := New(upstream.URL, rec)
	require.NoError(t, err)

	body := `{"name":"Alice"}`
	req := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	p.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusCreated, rw.Code)
	require.Equal(t, 1, rec.Count())
	assert.Equal(t, body, string(rec.exchanges[0].ReqBody))
}

func TestProxy_ServeHTTP_RecordsNilBody(t *testing.T) {
	upstream := newUpstream(t, http.StatusOK, `{}`, "application/json")
	defer upstream.Close()

	rec := NewRecorder()
	p, err := New(upstream.URL, rec)
	require.NoError(t, err)

	// httptest.NewRequest with empty body produces a non-nil but empty body;
	// use a raw http.Request with nil body to cover that branch explicitly.
	req := httptest.NewRequest(http.MethodGet, "/empty", nil)
	req.Body = nil
	rw := httptest.NewRecorder()
	p.ServeHTTP(rw, req)

	require.Equal(t, 1, rec.Count())
	assert.Nil(t, rec.exchanges[0].ReqBody)
}

func TestProxy_ServeHTTP_WriteHeaderCapture(t *testing.T) {
	upstream := newUpstream(t, http.StatusNotFound, `{"error":"not found"}`, "application/json")
	defer upstream.Close()

	rec := NewRecorder()
	p, err := New(upstream.URL, rec)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rw := httptest.NewRecorder()
	p.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusNotFound, rw.Code)
	require.Equal(t, 1, rec.Count())
	assert.Equal(t, http.StatusNotFound, rec.exchanges[0].Status)
}

// TestResponseCapture_Write verifies that the capture buffer and the
// underlying ResponseWriter both receive the written bytes.
func TestResponseCapture_Write(t *testing.T) {
	inner := httptest.NewRecorder()
	cap := &responseCapture{ResponseWriter: inner, status: http.StatusOK}

	n, err := cap.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", cap.body.String())
	assert.Equal(t, "hello", inner.Body.String())
}

// TestResponseCapture_WriteHeader verifies that the captured status and the
// underlying ResponseWriter status are both set.
func TestResponseCapture_WriteHeader(t *testing.T) {
	inner := httptest.NewRecorder()
	cap := &responseCapture{ResponseWriter: inner, status: http.StatusOK}

	cap.WriteHeader(http.StatusAccepted)
	assert.Equal(t, http.StatusAccepted, cap.status)
	assert.Equal(t, http.StatusAccepted, inner.Code)
}
