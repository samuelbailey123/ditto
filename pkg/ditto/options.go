// Package ditto provides a test helper that launches a Ditto mock server and
// returns its base URL. Import this package in your test files to spin up
// per-test HTTP mocks driven by YAML fixture files.
package ditto

const (
	defaultHost = "127.0.0.1"
	defaultPort = 0 // 0 instructs the OS to pick a free port
)

// options holds the resolved configuration for a single SDK call.
type options struct {
	port    int
	host    string
	verbose bool
}

// defaultOptions returns the baseline configuration used when no Option
// values are provided by the caller.
func defaultOptions() options {
	return options{
		port:    defaultPort,
		host:    defaultHost,
		verbose: false,
	}
}

// Option is a functional configuration modifier for [Start] and
// [StartWithOptions].
type Option func(*options)

// WithPort overrides the TCP port the mock server binds to.
// Pass 0 (the default) to let the operating system assign a free port.
func WithPort(port int) Option {
	return func(o *options) {
		o.port = port
	}
}

// WithHost overrides the network interface the mock server listens on.
// Defaults to "127.0.0.1" so the server is only reachable from localhost.
func WithHost(host string) Option {
	return func(o *options) {
		o.host = host
	}
}

// WithVerbose enables verbose logging on the mock server. When set, the server
// prints each matched request to stderr so failures are easier to diagnose.
func WithVerbose() Option {
	return func(o *options) {
		o.verbose = true
	}
}
