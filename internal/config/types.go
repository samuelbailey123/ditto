package config

import "encoding/json"

// MockFile is the top-level YAML structure for a single mock definition file.
type MockFile struct {
	Name      string     `yaml:"name,omitempty"`
	Defaults  Defaults   `yaml:"defaults,omitempty"`
	Routes    []Route    `yaml:"routes"`
	Scenarios []Scenario `yaml:"scenarios,omitempty"`
}

// Defaults holds values that apply to all routes unless overridden.
type Defaults struct {
	Headers map[string]string `yaml:"headers,omitempty"`
	Delay   *Delay            `yaml:"delay,omitempty"`
	Cors    *CorsConfig       `yaml:"cors,omitempty"`
}

// Route defines a single HTTP mock endpoint.
type Route struct {
	Method   string            `yaml:"method"`
	Path     string            `yaml:"path"`
	Match    *RequestMatch     `yaml:"match,omitempty"`
	Status   int               `yaml:"status"`
	Headers  map[string]string `yaml:"headers,omitempty"`
	Body     interface{}       `yaml:"body,omitempty"`
	BodyFile string            `yaml:"body_file,omitempty"`
	Delay    *Delay            `yaml:"delay,omitempty"`
	Chaos    *ChaosConfig      `yaml:"chaos,omitempty"`
}

// BodyAsString returns the route body as a JSON string.
// If Body is already a string it is returned as-is.
// Maps and slices are marshalled to JSON.
func (r Route) BodyAsString() (string, error) {
	if r.Body == nil {
		return "", nil
	}
	switch v := r.Body.(type) {
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

// RequestMatch holds criteria for matching an incoming request.
type RequestMatch struct {
	Body    map[string]interface{} `yaml:"body,omitempty"`
	Query   map[string]string      `yaml:"query,omitempty"`
	Headers map[string]string      `yaml:"headers,omitempty"`
}

// Delay configures artificial latency for a route.
// Either Fixed or both Min and Max must be set.
type Delay struct {
	Fixed string `yaml:"fixed,omitempty"`
	Min   string `yaml:"min,omitempty"`
	Max   string `yaml:"max,omitempty"`
}

// ChaosConfig injects random failures into a route.
type ChaosConfig struct {
	Probability float64 `yaml:"probability"`
	Status      int     `yaml:"status"`
	Body        string  `yaml:"body,omitempty"`
}

// Scenario models a stateful sequence of responses.
type Scenario struct {
	Name  string         `yaml:"name"`
	Steps []ScenarioStep `yaml:"steps"`
}

// ScenarioStep is one transition within a scenario.
type ScenarioStep struct {
	On        string            `yaml:"on"`
	WhenState string            `yaml:"when_state,omitempty"`
	SetState  string            `yaml:"set_state"`
	Status    int               `yaml:"status,omitempty"`
	Body      interface{}       `yaml:"body,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty"`
}

// CorsConfig controls cross-origin resource sharing behaviour.
type CorsConfig struct {
	Origins []string `yaml:"origins"`
	Methods []string `yaml:"methods"`
	Headers []string `yaml:"headers"`
}

// MockConfig is the merged, validated configuration consumed by the server.
type MockConfig struct {
	Routes    []Route
	Scenarios []Scenario
	Defaults  Defaults
}
