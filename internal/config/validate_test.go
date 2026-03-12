package config_test

import (
	"testing"

	"github.com/samuelbailey123/ditto/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidConfig(t *testing.T) {
	path := testdataPath(t, "basic.yaml")

	cfg, err := config.LoadFiles(path)
	require.NoError(t, err)

	errs := config.Validate(cfg)
	assert.Empty(t, errs)
}

func TestValidate_InvalidMethod(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "INVALID", Path: "/test", Status: 200},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, `invalid HTTP method "INVALID"`)
}

func TestValidate_MissingPath(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "", Status: 200},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "path is required")
}

func TestValidate_InvalidStatus(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/test", Status: 999},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "status 999 is out of range")
}

func TestValidate_InvalidChaos(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/flaky",
				Status: 200,
				Chaos:  &config.ChaosConfig{Probability: 1.5, Status: 500},
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "chaos.probability must be between 0.0 and 1.0")
}

func TestValidate_BodyAndBodyFile(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method:   "POST",
				Path:     "/both",
				Status:   200,
				Body:     "inline",
				BodyFile: "file.json",
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "body and body_file cannot both be set")
}

func TestValidate_InvalidDelay(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/slow",
				Status: 200,
				Delay:  &config.Delay{Min: "100ms"},
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "delay.min and delay.max must both be set if either is set")
}

func TestValidate_PathNoSlash(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "no-leading-slash", Status: 200},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "path must start with /")
}

func TestValidate_InvalidFile(t *testing.T) {
	path := testdataPath(t, "invalid.yaml")

	mf, err := config.LoadFile(path)
	require.NoError(t, err)

	cfg := config.MergeConfigs(mf)
	errs := config.Validate(cfg)

	assert.GreaterOrEqual(t, len(errs), 4, "expected at least 4 validation errors from invalid.yaml, got %d: %v", len(errs), errs)
}

func TestValidate_ZeroStatus(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "GET", Path: "/test", Status: 0},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "status is required")
}

func TestValidate_EmptyMethod(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{Method: "", Path: "/test", Status: 200},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "method is required")
}

func TestValidate_ScenarioMissingOn(t *testing.T) {
	cfg := &config.MockConfig{
		Scenarios: []config.Scenario{
			{
				Name: "test-flow",
				Steps: []config.ScenarioStep{
					{On: "", SetState: "done"},
				},
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "on is required")
}

func TestValidate_ScenarioEmptySetStateAllowed(t *testing.T) {
	cfg := &config.MockConfig{
		Scenarios: []config.Scenario{
			{
				Name: "test-flow",
				Steps: []config.ScenarioStep{
					{On: "GET /test", SetState: ""},
				},
			},
		},
	}

	errs := config.Validate(cfg)
	assert.Empty(t, errs)
}

func TestValidate_DelayFixedInvalidDuration(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/test",
				Status: 200,
				Delay:  &config.Delay{Fixed: "not-a-duration"},
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "delay.fixed")
	assert.Contains(t, messages, "not a valid duration")
}

func TestValidate_DelayRangeInvalidDurations(t *testing.T) {
	cfg := &config.MockConfig{
		Routes: []config.Route{
			{
				Method: "GET",
				Path:   "/test",
				Status: 200,
				Delay:  &config.Delay{Min: "bad", Max: "also-bad"},
			},
		},
	}

	errs := config.Validate(cfg)
	require.NotEmpty(t, errs)

	messages := errMessages(errs)
	assert.Contains(t, messages, "delay.min")
	assert.Contains(t, messages, "delay.max")
}

// errMessages collects the Error() string from each error into a single
// concatenated string so callers can use assert.Contains without looping.
func errMessages(errs []error) string {
	var out string
	for _, e := range errs {
		out += e.Error() + "\n"
	}
	return out
}
