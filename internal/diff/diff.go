package diff

import (
	"encoding/json"
	"fmt"

	"github.com/samuelbailey123/ditto/internal/config"
)

// Change describes a single difference between two mock configurations.
type Change struct {
	// Type is one of "added", "removed", or "modified".
	Type string
	// Route is the human-readable key for the route, e.g. "GET /users".
	Route string
	// Details describes what specifically changed for a "modified" entry.
	Details string
}

// routeKey returns the canonical comparison key for a route.
func routeKey(r config.Route) string {
	return fmt.Sprintf("%s %s", r.Method, r.Path)
}

// Compare returns the list of changes needed to go from a to b.
//
//   - Routes present only in b are reported as "added".
//   - Routes present only in a are reported as "removed".
//   - Routes present in both are compared field-by-field; any difference is
//     reported as "modified" with a human-readable detail string.
func Compare(a, b *config.MockConfig) []Change {
	indexA := indexRoutes(a.Routes)
	indexB := indexRoutes(b.Routes)

	var changes []Change

	// Removed: in a but not b.
	for key := range indexA {
		if _, exists := indexB[key]; !exists {
			changes = append(changes, Change{Type: "removed", Route: key})
		}
	}

	// Added: in b but not a.
	for key := range indexB {
		if _, exists := indexA[key]; !exists {
			changes = append(changes, Change{Type: "added", Route: key})
		}
	}

	// Modified: in both — compare individual fields.
	for key, ra := range indexA {
		rb, exists := indexB[key]
		if !exists {
			continue
		}
		if details := compareRoutes(ra, rb); details != "" {
			changes = append(changes, Change{Type: "modified", Route: key, Details: details})
		}
	}

	return changes
}

// indexRoutes builds a map from route key to Route for fast lookup.
func indexRoutes(routes []config.Route) map[string]config.Route {
	m := make(map[string]config.Route, len(routes))
	for _, r := range routes {
		m[routeKey(r)] = r
	}
	return m
}

// compareRoutes returns a human-readable description of the differences
// between ra and rb, or an empty string if they are identical.
func compareRoutes(ra, rb config.Route) string {
	var parts []string

	if ra.Status != rb.Status {
		parts = append(parts, fmt.Sprintf("status %d → %d", ra.Status, rb.Status))
	}

	if !headersEqual(ra.Headers, rb.Headers) {
		parts = append(parts, "headers changed")
	}

	if !bodyEqual(ra.Body, rb.Body) {
		parts = append(parts, "body changed")
	}

	if delayDescription(ra.Delay) != delayDescription(rb.Delay) {
		parts = append(parts, fmt.Sprintf("delay %s → %s", delayDescription(ra.Delay), delayDescription(rb.Delay)))
	}

	if !chaosEqual(ra.Chaos, rb.Chaos) {
		parts = append(parts, "chaos config changed")
	}

	if !matchEqual(ra.Match, rb.Match) {
		parts = append(parts, "match criteria changed")
	}

	if len(parts) == 0 {
		return ""
	}

	result := parts[0]
	for _, p := range parts[1:] {
		result += "; " + p
	}
	return result
}

// headersEqual reports whether two header maps have the same key/value pairs.
func headersEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// bodyEqual compares two body values by marshalling them to JSON. This
// normalises differences in underlying Go types (e.g. map vs struct) while
// preserving semantic equality.
func bodyEqual(a, b interface{}) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

// delayDescription returns a stable string representation of a Delay config
// suitable for equality comparison.
func delayDescription(d *config.Delay) string {
	if d == nil {
		return "<none>"
	}
	if d.Fixed != "" {
		return fmt.Sprintf("fixed=%s", d.Fixed)
	}
	return fmt.Sprintf("min=%s,max=%s", d.Min, d.Max)
}

// chaosEqual reports whether two ChaosConfig values are semantically equal.
func chaosEqual(a, b *config.ChaosConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Probability == b.Probability && a.Status == b.Status && a.Body == b.Body
}

// matchEqual reports whether two RequestMatch values are semantically equal.
func matchEqual(a, b *config.RequestMatch) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}
