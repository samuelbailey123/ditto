package server

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/samuelbailey123/ditto/internal/config"
)

// ScenarioResponse holds the response data to send when a scenario step matches.
type ScenarioResponse struct {
	Status  int
	Body    interface{}
	Headers map[string]string
}

// bodyAsString marshals the response body to a JSON string, mirroring the
// logic in config.Route.BodyAsString so scenario steps behave consistently.
func (sr *ScenarioResponse) bodyAsString() (string, error) {
	if sr.Body == nil {
		return "", nil
	}
	switch v := sr.Body.(type) {
	case string:
		return v, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

// StateEngine evaluates stateful scenarios against incoming requests.
// Each scenario tracks its own current state independently.
//
// Matching rules:
//   - A step without WhenState matches when the scenario is in its initial
//     (empty) state.
//   - A step with WhenState only matches when the scenario's current state
//     equals that value.
//   - On a successful match the scenario's state is advanced to SetState and
//     the step's response is returned.
//   - Scenarios are evaluated in declaration order; the first matching step
//     across all scenarios wins.
type StateEngine struct {
	scenarios []config.Scenario
	states    map[string]string // scenario name -> current state
	mu        sync.RWMutex
}

// NewStateEngine constructs a StateEngine loaded with the given scenarios.
// All scenario states begin as the empty string (initial state).
func NewStateEngine(scenarios []config.Scenario) *StateEngine {
	return &StateEngine{
		scenarios: scenarios,
		states:    make(map[string]string, len(scenarios)),
	}
}

// Match checks whether any scenario step is eligible to handle method+path
// given the current per-scenario state. The first eligible step wins.
//
// A step is eligible when:
//  1. Its On field parses to the same method and path.
//  2. Its WhenState equals the scenario's current state (or WhenState is
//     empty, meaning "match only in the initial state").
//
// On a match the scenario state is advanced to step.SetState before returning.
func (se *StateEngine) Match(method, path string) (*ScenarioResponse, bool) {
	se.mu.Lock()
	defer se.mu.Unlock()

	for i := range se.scenarios {
		sc := &se.scenarios[i]
		current := se.states[sc.Name]

		for j := range sc.Steps {
			step := &sc.Steps[j]

			stepMethod, stepPath, ok := parseOn(step.On)
			if !ok {
				continue
			}

			if !strings.EqualFold(stepMethod, method) {
				continue
			}
			if stepPath != path {
				continue
			}

			// WhenState="" means this step is only valid in the initial state.
			if step.WhenState != current {
				continue
			}

			// Advance state before returning so concurrent calls see the new state.
			se.states[sc.Name] = step.SetState

			status := step.Status
			if status == 0 {
				status = 200
			}

			return &ScenarioResponse{
				Status:  status,
				Body:    step.Body,
				Headers: step.Headers,
			}, true
		}
	}

	return nil, false
}

// Reset clears all per-scenario state back to the empty initial state.
func (se *StateEngine) Reset() {
	se.mu.Lock()
	defer se.mu.Unlock()
	for k := range se.states {
		delete(se.states, k)
	}
}

// States returns a point-in-time copy of the current per-scenario state map.
// Callers can use this for debugging or TUI display without holding the lock.
func (se *StateEngine) States() map[string]string {
	se.mu.RLock()
	defer se.mu.RUnlock()
	out := make(map[string]string, len(se.states))
	for k, v := range se.states {
		out[k] = v
	}
	return out
}

// parseOn splits a step's On field (e.g. "POST /users") into method and path.
// It returns false when the field is malformed or missing either component.
func parseOn(on string) (method, path string, ok bool) {
	idx := strings.IndexByte(on, ' ')
	if idx <= 0 || idx == len(on)-1 {
		return "", "", false
	}
	return strings.ToUpper(strings.TrimSpace(on[:idx])), strings.TrimSpace(on[idx+1:]), true
}
