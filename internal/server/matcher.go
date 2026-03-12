package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// matchQuery reports whether every key-value pair in expected is satisfied by
// actual. A value of "*" means the key must be present with any non-empty value.
func matchQuery(expected map[string]string, actual url.Values) bool {
	for key, want := range expected {
		got := actual.Get(key)
		if got == "" {
			return false
		}
		if want != "*" && got != want {
			return false
		}
	}
	return true
}

// matchHeaders reports whether every key-value pair in expected is present in
// actual. Key comparison is case-insensitive. A value of "*" means the header
// must exist with any non-empty value.
func matchHeaders(expected map[string]string, actual http.Header) bool {
	for key, want := range expected {
		got := actual.Get(key)
		if got == "" {
			// http.Header.Get is already canonical-case aware, but the expected
			// key may be in arbitrary case, so try a manual scan as a fallback.
			found := false
			for hk, hv := range actual {
				if strings.EqualFold(hk, key) {
					if len(hv) > 0 {
						got = hv[0]
						found = true
					}
					break
				}
			}
			if !found {
				return false
			}
		}
		if want != "*" && got != want {
			return false
		}
	}
	return true
}

// matchBody reports whether every key-value pair in expected is satisfied by
// the JSON-decoded body. A value of "*" means the key must exist. Comparison
// is recursive for nested maps.
func matchBody(expected map[string]interface{}, body []byte) bool {
	if len(body) == 0 {
		return len(expected) == 0
	}

	var actual map[string]interface{}
	if err := json.Unmarshal(body, &actual); err != nil {
		return false
	}

	return matchMapSubset(expected, actual)
}

// matchMapSubset checks that every key in expected is present in actual and
// that their values satisfy the wildcard or deep-equality rules.
func matchMapSubset(expected, actual map[string]interface{}) bool {
	for key, wantRaw := range expected {
		gotRaw, exists := actual[key]
		if !exists {
			return false
		}

		// Wildcard string matches any present value.
		if wantStr, ok := wantRaw.(string); ok && wantStr == "*" {
			continue
		}

		// Recurse into nested maps.
		wantMap, wantIsMap := wantRaw.(map[string]interface{})
		gotMap, gotIsMap := gotRaw.(map[string]interface{})
		if wantIsMap && gotIsMap {
			if !matchMapSubset(wantMap, gotMap) {
				return false
			}
			continue
		}

		// Fall back to formatted string comparison to avoid float64 vs int
		// mismatch from JSON unmarshalling.
		if wantStr, ok := wantRaw.(string); ok {
			gotStr, ok := gotRaw.(string)
			if !ok {
				return false
			}
			if wantStr != gotStr {
				return false
			}
			continue
		}

		// For numeric and boolean values, compare via JSON round-trip so that
		// both sides go through the same type coercion.
		wantBytes, err := json.Marshal(wantRaw)
		if err != nil {
			return false
		}
		gotBytes, err := json.Marshal(gotRaw)
		if err != nil {
			return false
		}
		if string(wantBytes) != string(gotBytes) {
			return false
		}
	}
	return true
}
