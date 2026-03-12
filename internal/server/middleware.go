package server

import (
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/samuelbailey123/ditto/internal/config"
)

// corsMiddleware wraps next with CORS handling. If cfg is nil, permissive
// defaults are applied (allow all origins, common methods, any headers).
//
// For OPTIONS preflight requests the middleware responds immediately without
// calling next.
func corsMiddleware(cfg *config.CorsConfig, next http.Handler) http.Handler {
	origins := "*"
	methods := "GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS"
	headers := "*"

	if cfg != nil {
		if len(cfg.Origins) > 0 {
			origins = strings.Join(cfg.Origins, ", ")
		}
		if len(cfg.Methods) > 0 {
			methods = strings.Join(cfg.Methods, ", ")
		}
		if len(cfg.Headers) > 0 {
			headers = strings.Join(cfg.Headers, ", ")
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origins)
		w.Header().Set("Access-Control-Allow-Methods", methods)
		w.Header().Set("Access-Control-Allow-Headers", headers)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// delayMiddleware pauses for the duration defined in delay before calling next.
//
// If delay.Fixed is set, that exact duration is used.
// If delay.Min and delay.Max are set, a random duration in [min, max] is used.
// Malformed duration strings are silently ignored (no delay applied).
func delayMiddleware(delay *config.Delay, next http.HandlerFunc) http.HandlerFunc {
	if delay == nil {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if delay.Fixed != "" {
			if d, err := time.ParseDuration(delay.Fixed); err == nil {
				time.Sleep(d)
			}
		} else if delay.Min != "" && delay.Max != "" {
			minD, err1 := time.ParseDuration(delay.Min)
			maxD, err2 := time.ParseDuration(delay.Max)
			if err1 == nil && err2 == nil && maxD >= minD {
				spread := int64(maxD - minD)
				jitter := time.Duration(rand.Int63n(spread + 1))
				time.Sleep(minD + jitter)
			}
		}

		next(w, r)
	}
}

// chaosMiddleware randomly short-circuits requests by returning the configured
// chaos status and body. The roll happens once per request.
//
// If chaos is nil or probability is 0, next is always called.
// If probability is 1.0, the chaos response is always returned.
func chaosMiddleware(chaos *config.ChaosConfig, next http.HandlerFunc) http.HandlerFunc {
	if chaos == nil || chaos.Probability <= 0 {
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if rand.Float64() < chaos.Probability {
			status := chaos.Status
			if status == 0 {
				status = http.StatusInternalServerError
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			if chaos.Body != "" {
				_, _ = w.Write([]byte(chaos.Body))
			}
			return
		}
		next(w, r)
	}
}
