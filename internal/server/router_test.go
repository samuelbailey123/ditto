package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatch_ExactPath(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/health", Status: 200},
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	route, params := router.Match(req)

	require.NotNil(t, route)
	assert.Equal(t, "/health", route.Path)
	assert.Empty(t, params)
}

func TestMatch_PathParams(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/users/{id}", Status: 200},
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	route, params := router.Match(req)

	require.NotNil(t, route)
	assert.Equal(t, "/users/{id}", route.Path)
	require.NotNil(t, params)
	assert.Equal(t, "123", params["id"])
}

func TestMatch_Wildcard(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/static/*", Status: 200},
	})

	req := httptest.NewRequest(http.MethodGet, "/static/css/style.css", nil)
	route, params := router.Match(req)

	require.NotNil(t, route)
	assert.Equal(t, "/static/*", route.Path)
	assert.NotNil(t, params)
}

func TestMatch_NoMatch(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/health", Status: 200},
	})

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	route, params := router.Match(req)

	assert.Nil(t, route)
	assert.Nil(t, params)
}

func TestMatch_MethodMismatch(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/health", Status: 200},
	})

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	route, params := router.Match(req)

	assert.Nil(t, route)
	assert.Nil(t, params)
}

func TestMatch_Priority(t *testing.T) {
	// Both routes match /users/42 but the exact route should win.
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/users/{id}", Status: 200, Body: "parameterised"},
		{Method: "GET", Path: "/users/42", Status: 200, Body: "exact"},
	})

	req := httptest.NewRequest(http.MethodGet, "/users/42", nil)
	route, _ := router.Match(req)

	require.NotNil(t, route)
	assert.Equal(t, "/users/42", route.Path)
}

func TestMatch_QueryMatching(t *testing.T) {
	router := NewRouter([]config.Route{
		{
			Method: "GET",
			Path:   "/search",
			Status: 200,
			Match: &config.RequestMatch{
				Query: map[string]string{"q": "*"},
			},
		},
	})

	t.Run("present", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)
		route, _ := router.Match(req)
		require.NotNil(t, route)
	})

	t.Run("absent", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		route, _ := router.Match(req)
		assert.Nil(t, route)
	})
}

// TestMatchWithBody_MethodMismatch verifies the MatchWithBody method-check branch.
func TestMatchWithBody_MethodMismatch(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/health", Status: 200},
	})

	req := httptest.NewRequest(http.MethodDelete, "/health", nil)
	route, params := router.MatchWithBody(req, nil)

	assert.Nil(t, route)
	assert.Nil(t, params)
}

// TestMatchPath_LengthMismatch covers the segment-count guard in matchPath.
func TestMatchPath_LengthMismatch(t *testing.T) {
	router := NewRouter([]config.Route{
		{Method: "GET", Path: "/a/b/c", Status: 200},
	})

	req := httptest.NewRequest(http.MethodGet, "/a/b", nil)
	route, params := router.Match(req)

	assert.Nil(t, route)
	assert.Nil(t, params)
}

func TestMatch_HeaderMatching(t *testing.T) {
	router := NewRouter([]config.Route{
		{
			Method: "GET",
			Path:   "/secure",
			Status: 200,
			Match: &config.RequestMatch{
				Headers: map[string]string{"X-Api-Key": "secret"},
			},
		},
	})

	t.Run("correct header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("X-Api-Key", "secret")
		route, _ := router.Match(req)
		require.NotNil(t, route)
	})

	t.Run("wrong value", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		req.Header.Set("X-Api-Key", "wrong")
		route, _ := router.Match(req)
		assert.Nil(t, route)
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/secure", nil)
		route, _ := router.Match(req)
		assert.Nil(t, route)
	})
}
