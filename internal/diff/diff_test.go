package diff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/samuelbailey123/ditto/internal/config"
)

// cfg is a convenience helper that builds a MockConfig from a slice of routes.
func cfg(routes ...config.Route) *config.MockConfig {
	return &config.MockConfig{Routes: routes}
}

// route constructs a minimal Route for use in tests.
func route(method, path string, status int) config.Route {
	return config.Route{Method: method, Path: path, Status: status}
}

func TestCompare_AddedRoute(t *testing.T) {
	a := cfg(route("GET", "/users", 200))
	b := cfg(route("GET", "/users", 200), route("POST", "/users", 201))

	changes := Compare(a, b)

	require.Len(t, changes, 1)
	assert.Equal(t, "added", changes[0].Type)
	assert.Equal(t, "POST /users", changes[0].Route)
}

func TestCompare_RemovedRoute(t *testing.T) {
	a := cfg(route("GET", "/users", 200), route("DELETE", "/users/{id}", 204))
	b := cfg(route("GET", "/users", 200))

	changes := Compare(a, b)

	require.Len(t, changes, 1)
	assert.Equal(t, "removed", changes[0].Type)
	assert.Equal(t, "DELETE /users/{id}", changes[0].Route)
}

func TestCompare_ModifiedRoute(t *testing.T) {
	ra := route("GET", "/ping", 200)
	rb := route("GET", "/ping", 503)

	changes := Compare(cfg(ra), cfg(rb))

	require.Len(t, changes, 1)
	assert.Equal(t, "modified", changes[0].Type)
	assert.Equal(t, "GET /ping", changes[0].Route)
	assert.Contains(t, changes[0].Details, "200")
	assert.Contains(t, changes[0].Details, "503")
}

func TestCompare_Identical(t *testing.T) {
	r := route("GET", "/health", 200)
	changes := Compare(cfg(r), cfg(r))
	assert.Empty(t, changes)
}

func TestCompare_MultipleChanges(t *testing.T) {
	a := cfg(
		route("GET", "/users", 200),    // kept — will be modified
		route("DELETE", "/items", 204), // removed
	)
	b := cfg(
		route("GET", "/users", 500),  // modified (status changed)
		route("POST", "/orders", 201), // added
	)

	changes := Compare(a, b)

	// Expect: 1 modified, 1 removed, 1 added = 3 changes total.
	require.Len(t, changes, 3)

	types := make(map[string]int)
	for _, c := range changes {
		types[c.Type]++
	}
	assert.Equal(t, 1, types["added"], "expected 1 added change")
	assert.Equal(t, 1, types["removed"], "expected 1 removed change")
	assert.Equal(t, 1, types["modified"], "expected 1 modified change")
}

func TestCompare_ModifiedHeaders(t *testing.T) {
	ra := config.Route{Method: "GET", Path: "/data", Status: 200,
		Headers: map[string]string{"X-Custom": "v1"}}
	rb := config.Route{Method: "GET", Path: "/data", Status: 200,
		Headers: map[string]string{"X-Custom": "v2"}}

	changes := Compare(cfg(ra), cfg(rb))

	require.Len(t, changes, 1)
	assert.Equal(t, "modified", changes[0].Type)
	assert.Contains(t, changes[0].Details, "headers changed")
}

func TestCompare_ModifiedBody(t *testing.T) {
	ra := config.Route{Method: "GET", Path: "/item", Status: 200, Body: map[string]interface{}{"v": 1}}
	rb := config.Route{Method: "GET", Path: "/item", Status: 200, Body: map[string]interface{}{"v": 2}}

	changes := Compare(cfg(ra), cfg(rb))

	require.Len(t, changes, 1)
	assert.Contains(t, changes[0].Details, "body changed")
}

func TestCompare_ModifiedDelay(t *testing.T) {
	fixed := func(d string) *config.Delay { return &config.Delay{Fixed: d} }
	minMax := func(mn, mx string) *config.Delay { return &config.Delay{Min: mn, Max: mx} }

	tests := []struct {
		name    string
		delayA  *config.Delay
		delayB  *config.Delay
		changed bool
	}{
		{"nil to fixed", nil, fixed("100ms"), true},
		{"fixed to nil", fixed("100ms"), nil, true},
		{"fixed changed", fixed("100ms"), fixed("200ms"), true},
		{"nil both", nil, nil, false},
		{"fixed same", fixed("100ms"), fixed("100ms"), false},
		{"min/max changed", minMax("10ms", "50ms"), minMax("20ms", "100ms"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ra := config.Route{Method: "GET", Path: "/d", Status: 200, Delay: tt.delayA}
			rb := config.Route{Method: "GET", Path: "/d", Status: 200, Delay: tt.delayB}
			changes := Compare(cfg(ra), cfg(rb))
			if tt.changed {
				require.Len(t, changes, 1, "expected a change")
				assert.Contains(t, changes[0].Details, "delay")
			} else {
				assert.Empty(t, changes)
			}
		})
	}
}

func TestCompare_ModifiedChaos(t *testing.T) {
	ca := &config.ChaosConfig{Probability: 0.1, Status: 500}
	cb := &config.ChaosConfig{Probability: 0.5, Status: 500}

	ra := config.Route{Method: "GET", Path: "/chaos", Status: 200, Chaos: ca}
	rb := config.Route{Method: "GET", Path: "/chaos", Status: 200, Chaos: cb}

	changes := Compare(cfg(ra), cfg(rb))
	require.Len(t, changes, 1)
	assert.Contains(t, changes[0].Details, "chaos config changed")
}

func TestCompare_ChaosAddedRemoved(t *testing.T) {
	chaos := &config.ChaosConfig{Probability: 0.1, Status: 503}

	t.Run("nil to value", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/c", Status: 200}
		rb := config.Route{Method: "GET", Path: "/c", Status: 200, Chaos: chaos}
		changes := Compare(cfg(ra), cfg(rb))
		require.Len(t, changes, 1)
		assert.Contains(t, changes[0].Details, "chaos config changed")
	})

	t.Run("value to nil", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/c", Status: 200, Chaos: chaos}
		rb := config.Route{Method: "GET", Path: "/c", Status: 200}
		changes := Compare(cfg(ra), cfg(rb))
		require.Len(t, changes, 1)
		assert.Contains(t, changes[0].Details, "chaos config changed")
	})

	t.Run("same value", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/c", Status: 200, Chaos: chaos}
		rb := config.Route{Method: "GET", Path: "/c", Status: 200, Chaos: chaos}
		changes := Compare(cfg(ra), cfg(rb))
		assert.Empty(t, changes)
	})
}

func TestCompare_ModifiedMatch(t *testing.T) {
	ma := &config.RequestMatch{Query: map[string]string{"page": "1"}}
	mb := &config.RequestMatch{Query: map[string]string{"page": "2"}}

	ra := config.Route{Method: "GET", Path: "/m", Status: 200, Match: ma}
	rb := config.Route{Method: "GET", Path: "/m", Status: 200, Match: mb}

	changes := Compare(cfg(ra), cfg(rb))
	require.Len(t, changes, 1)
	assert.Contains(t, changes[0].Details, "match criteria changed")
}

func TestCompare_MatchAddedRemoved(t *testing.T) {
	match := &config.RequestMatch{Query: map[string]string{"v": "1"}}

	t.Run("nil to value", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/m", Status: 200}
		rb := config.Route{Method: "GET", Path: "/m", Status: 200, Match: match}
		changes := Compare(cfg(ra), cfg(rb))
		require.Len(t, changes, 1)
		assert.Contains(t, changes[0].Details, "match criteria changed")
	})

	t.Run("value to nil", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/m", Status: 200, Match: match}
		rb := config.Route{Method: "GET", Path: "/m", Status: 200}
		changes := Compare(cfg(ra), cfg(rb))
		require.Len(t, changes, 1)
		assert.Contains(t, changes[0].Details, "match criteria changed")
	})

	t.Run("same match", func(t *testing.T) {
		ra := config.Route{Method: "GET", Path: "/m", Status: 200, Match: match}
		rb := config.Route{Method: "GET", Path: "/m", Status: 200, Match: match}
		changes := Compare(cfg(ra), cfg(rb))
		assert.Empty(t, changes)
	})
}

func TestCompare_HeadersEqualDifferentLength(t *testing.T) {
	ra := config.Route{Method: "GET", Path: "/h", Status: 200,
		Headers: map[string]string{"A": "1"}}
	rb := config.Route{Method: "GET", Path: "/h", Status: 200,
		Headers: map[string]string{"A": "1", "B": "2"}}

	changes := Compare(cfg(ra), cfg(rb))
	require.Len(t, changes, 1)
	assert.Contains(t, changes[0].Details, "headers changed")
}

// TestCompare_MultipleFieldChanges exercises the semicolon-joined details path
// in compareRoutes by changing status, headers, and body simultaneously.
func TestCompare_MultipleFieldChanges(t *testing.T) {
	ra := config.Route{
		Method:  "GET",
		Path:    "/multi",
		Status:  200,
		Headers: map[string]string{"X-V": "1"},
		Body:    "old body",
	}
	rb := config.Route{
		Method:  "GET",
		Path:    "/multi",
		Status:  404,
		Headers: map[string]string{"X-V": "2"},
		Body:    "new body",
	}

	changes := Compare(cfg(ra), cfg(rb))
	require.Len(t, changes, 1)
	assert.Equal(t, "modified", changes[0].Type)
	// Details must contain all three change descriptions joined by "; ".
	assert.Contains(t, changes[0].Details, "status")
	assert.Contains(t, changes[0].Details, "headers changed")
	assert.Contains(t, changes[0].Details, "body changed")
	assert.Contains(t, changes[0].Details, "; ")
}
