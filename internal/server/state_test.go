package server

import (
	"testing"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// shoppingScenario returns a three-step stateful checkout scenario used by
// multiple tests.
func shoppingScenario() config.Scenario {
	return config.Scenario{
		Name: "shopping",
		Steps: []config.ScenarioStep{
			{
				On:       "POST /cart/items",
				SetState: "item-added",
				Status:   201,
				Body:     map[string]interface{}{"cart": "created"},
			},
			{
				On:        "GET /cart",
				WhenState: "item-added",
				SetState:  "cart-viewed",
				Status:    200,
				Body:      map[string]interface{}{"items": 1},
			},
			{
				On:        "POST /cart/checkout",
				WhenState: "cart-viewed",
				SetState:  "checked-out",
				Status:    200,
				Body:      map[string]interface{}{"order": "placed"},
			},
		},
	}
}

// TestStateEngine_InitialMatch verifies that a step without WhenState fires
// immediately when the scenario is in its initial (empty) state.
func TestStateEngine_InitialMatch(t *testing.T) {
	se := NewStateEngine([]config.Scenario{shoppingScenario()})

	resp, ok := se.Match("POST", "/cart/items")
	require.True(t, ok, "expected match on initial state")
	assert.Equal(t, 201, resp.Status)
	assert.Equal(t, map[string]interface{}{"cart": "created"}, resp.Body)
}

// TestStateEngine_StateTransition verifies that a step with WhenState only
// matches after a previous step has advanced the scenario into that state.
func TestStateEngine_StateTransition(t *testing.T) {
	se := NewStateEngine([]config.Scenario{shoppingScenario()})

	// The second step requires state "item-added" — should not match yet.
	_, ok := se.Match("GET", "/cart")
	assert.False(t, ok, "GET /cart must not match before state is item-added")

	// Advance to "item-added".
	resp, ok := se.Match("POST", "/cart/items")
	require.True(t, ok)
	assert.Equal(t, "item-added", se.states["shopping"])
	assert.Equal(t, 201, resp.Status)

	// Now the second step should match.
	resp2, ok := se.Match("GET", "/cart")
	require.True(t, ok, "GET /cart must match when state is item-added")
	assert.Equal(t, 200, resp2.Status)
	assert.Equal(t, map[string]interface{}{"items": 1}, resp2.Body)
	assert.Equal(t, "cart-viewed", se.states["shopping"])
}

// TestStateEngine_FullFlow runs through the complete three-step shopping
// sequence and verifies each response in order.
func TestStateEngine_FullFlow(t *testing.T) {
	se := NewStateEngine([]config.Scenario{shoppingScenario()})

	steps := []struct {
		method     string
		path       string
		wantStatus int
		wantState  string
	}{
		{"POST", "/cart/items", 201, "item-added"},
		{"GET", "/cart", 200, "cart-viewed"},
		{"POST", "/cart/checkout", 200, "checked-out"},
	}

	for _, s := range steps {
		resp, ok := se.Match(s.method, s.path)
		require.Truef(t, ok, "expected match for %s %s", s.method, s.path)
		assert.Equalf(t, s.wantStatus, resp.Status, "status mismatch for %s %s", s.method, s.path)
		assert.Equalf(t, s.wantState, se.states["shopping"], "state mismatch after %s %s", s.method, s.path)
	}

	// After the final step, no further steps exist — nothing should match.
	_, ok := se.Match("POST", "/cart/items")
	assert.False(t, ok, "no step should match after scenario is exhausted")
}

// TestStateEngine_Reset verifies that Reset returns all scenarios to their
// initial state so steps that require WhenState no longer match, and initial
// steps match again.
func TestStateEngine_Reset(t *testing.T) {
	se := NewStateEngine([]config.Scenario{shoppingScenario()})

	// Advance the scenario.
	_, ok := se.Match("POST", "/cart/items")
	require.True(t, ok)
	assert.Equal(t, "item-added", se.states["shopping"])

	se.Reset()

	// State should be cleared.
	assert.Empty(t, se.States()["shopping"])

	// Step that required "item-added" must no longer match.
	_, ok = se.Match("GET", "/cart")
	assert.False(t, ok, "GET /cart must not match after Reset")

	// Initial step must match again.
	resp, ok := se.Match("POST", "/cart/items")
	require.True(t, ok, "POST /cart/items must match again after Reset")
	assert.Equal(t, 201, resp.Status)
}

// TestStateEngine_NoMatch verifies that an unrelated method/path returns false.
func TestStateEngine_NoMatch(t *testing.T) {
	se := NewStateEngine([]config.Scenario{shoppingScenario()})

	_, ok := se.Match("DELETE", "/unrelated")
	assert.False(t, ok)

	_, ok = se.Match("GET", "/cart/items")
	assert.False(t, ok, "wrong method must not match")
}

// TestStateEngine_MultipleScenarios verifies that two independent scenarios
// each maintain their own state and do not interfere with each other.
func TestStateEngine_MultipleScenarios(t *testing.T) {
	scenarioA := config.Scenario{
		Name: "auth",
		Steps: []config.ScenarioStep{
			{
				On:       "POST /login",
				SetState: "logged-in",
				Status:   200,
				Body:     map[string]interface{}{"token": "abc"},
			},
			{
				On:        "POST /logout",
				WhenState: "logged-in",
				SetState:  "logged-out",
				Status:    200,
			},
		},
	}

	scenarioB := config.Scenario{
		Name: "orders",
		Steps: []config.ScenarioStep{
			{
				On:       "POST /orders",
				SetState: "order-created",
				Status:   201,
				Body:     map[string]interface{}{"id": 1},
			},
			{
				On:        "GET /orders/1",
				WhenState: "order-created",
				SetState:  "order-fetched",
				Status:    200,
			},
		},
	}

	se := NewStateEngine([]config.Scenario{scenarioA, scenarioB})

	// Advance auth scenario.
	resp, ok := se.Match("POST", "/login")
	require.True(t, ok)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "logged-in", se.states["auth"])
	assert.Empty(t, se.states["orders"], "orders state must be unaffected")

	// Advance orders scenario.
	resp2, ok := se.Match("POST", "/orders")
	require.True(t, ok)
	assert.Equal(t, 201, resp2.Status)
	assert.Equal(t, "order-created", se.states["orders"])
	assert.Equal(t, "logged-in", se.states["auth"], "auth state must be unaffected")

	// Continue both independently.
	_, ok = se.Match("POST", "/logout")
	require.True(t, ok)
	assert.Equal(t, "logged-out", se.states["auth"])
	assert.Equal(t, "order-created", se.states["orders"])

	_, ok = se.Match("GET", "/orders/1")
	require.True(t, ok)
	assert.Equal(t, "order-fetched", se.states["orders"])
	assert.Equal(t, "logged-out", se.states["auth"])
}
