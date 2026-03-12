package config

import (
	"fmt"
	"net/http"
	"time"
)

var validMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// Validate checks a MockConfig for structural and semantic errors.
// It returns all errors found rather than stopping at the first.
func Validate(cfg *MockConfig) []error {
	var errs []error

	for i, r := range cfg.Routes {
		prefix := fmt.Sprintf("route[%d]", i)

		if r.Method == "" {
			errs = append(errs, fmt.Errorf("%s: method is required", prefix))
		} else if !validMethods[r.Method] {
			errs = append(errs, fmt.Errorf("%s: invalid HTTP method %q", prefix, r.Method))
		}

		if r.Path == "" {
			errs = append(errs, fmt.Errorf("%s: path is required", prefix))
		} else if r.Path[0] != '/' {
			errs = append(errs, fmt.Errorf("%s: path must start with /", prefix))
		}

		if r.Status == 0 {
			errs = append(errs, fmt.Errorf("%s: status is required", prefix))
		} else if r.Status < 100 || r.Status > 599 {
			errs = append(errs, fmt.Errorf("%s: status %d is out of range (100-599)", prefix, r.Status))
		}

		if r.Body != nil && r.BodyFile != "" {
			errs = append(errs, fmt.Errorf("%s: body and body_file cannot both be set", prefix))
		}

		if r.Delay != nil {
			if delayErrs := validateDelay(r.Delay, prefix); len(delayErrs) > 0 {
				errs = append(errs, delayErrs...)
			}
		}

		if r.Chaos != nil {
			if r.Chaos.Probability < 0.0 || r.Chaos.Probability > 1.0 {
				errs = append(errs, fmt.Errorf("%s: chaos.probability must be between 0.0 and 1.0", prefix))
			}
		}
	}

	for i, s := range cfg.Scenarios {
		for j, step := range s.Steps {
			prefix := fmt.Sprintf("scenario[%d](%s).step[%d]", i, s.Name, j)

			if step.On == "" {
				errs = append(errs, fmt.Errorf("%s: on is required", prefix))
			}

		}
	}

	return errs
}

// validateDelay checks that a Delay config has parseable duration strings
// and that min/max are either both set or both unset.
func validateDelay(d *Delay, prefix string) []error {
	var errs []error

	if d.Fixed != "" {
		if _, err := time.ParseDuration(d.Fixed); err != nil {
			errs = append(errs, fmt.Errorf("%s: delay.fixed %q is not a valid duration: %w", prefix, d.Fixed, err))
		}
	}

	minSet := d.Min != ""
	maxSet := d.Max != ""

	if minSet != maxSet {
		errs = append(errs, fmt.Errorf("%s: delay.min and delay.max must both be set if either is set", prefix))
	}

	if minSet {
		if _, err := time.ParseDuration(d.Min); err != nil {
			errs = append(errs, fmt.Errorf("%s: delay.min %q is not a valid duration: %w", prefix, d.Min, err))
		}
	}

	if maxSet {
		if _, err := time.ParseDuration(d.Max); err != nil {
			errs = append(errs, fmt.Errorf("%s: delay.max %q is not a valid duration: %w", prefix, d.Max, err))
		}
	}

	return errs
}
