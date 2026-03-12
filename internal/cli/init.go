package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initOutput string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a starter mock definition file",
	Long: `Init writes a well-commented example YAML file to disk so you can start
defining mock routes immediately without consulting the docs.

The generated file demonstrates every supported feature: static responses,
path parameters, Go template-powered dynamic responses (path params, timestamps,
UUIDs), per-route header overrides, CORS defaults, request matching,
file-backed bodies, artificial latency, chaos injection, and stateful scenarios.`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initOutput, "output", "o", "ditto.yaml", "path to write the generated file")
}

// runInit writes the example YAML to the configured output path.
// It refuses to overwrite an existing file to prevent accidental data loss.
func runInit(_ *cobra.Command, _ []string) error {
	if _, err := os.Stat(initOutput); err == nil {
		fmt.Fprintf(os.Stderr, "error: file %q already exists — remove it first or choose a different path with --output\n", initOutput)
		os.Exit(1)
	}

	if err := os.WriteFile(initOutput, []byte(exampleYAML), 0o644); err != nil {
		return fmt.Errorf("writing %q: %w", initOutput, err)
	}

	fmt.Printf("created %s\n\n", initOutput)
	fmt.Printf("  Next steps:\n")
	fmt.Printf("    1. Edit %s to define your API routes\n", initOutput)
	fmt.Printf("    2. Run: ditto serve %s\n", initOutput)
	fmt.Printf("    3. Send requests to http://localhost:8080\n\n")
	fmt.Printf("  Tips:\n")
	fmt.Printf("    - Use ditto validate %s to check for errors\n", initOutput)
	fmt.Printf("    - Use ditto serve --ui %s for a live request dashboard\n", initOutput)
	fmt.Printf("    - Use ditto record --upstream https://api.example.com to capture real API traffic\n")
	return nil
}

// exampleYAML is the starter template written by `ditto init`. It covers
// every feature supported by Ditto so that new users have a working reference
// without needing to read external documentation first.
const exampleYAML = `# Ditto mock definition file
# Docs: https://github.com/samuelbailey123/ditto

name: my-api

defaults:
  headers:
    Content-Type: application/json
  cors:
    origins: ["*"]
    methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
    headers: ["Content-Type", "Authorization"]

routes:
  # Simple static response
  - method: GET
    path: /health
    status: 200
    body:
      status: ok

  # Path parameters — {id} is extracted and available as a param
  - method: GET
    path: /users/{id}
    status: 200
    body:
      id: 1
      name: Alice
      email: alice@example.com

  # Dynamic response using Go templates — path params, timestamps, UUIDs
  - method: GET
    path: /greet/{name}
    status: 200
    body: '{"greeting": "Hello, {{.Params.name}}!", "time": "{{now}}", "request_id": "{{uuid}}"}'

  # Override default Content-Type for this route only
  - method: GET
    path: /text
    status: 200
    headers:
      Content-Type: text/plain
    body: "plain text response"

  # Request matching — only fires when the incoming body contains 'name'
  - method: POST
    path: /users
    status: 201
    body:
      id: 3
      name: Charlie
    match:
      headers:
        Content-Type: application/json
      body:
        name: "*"

  # Response loaded from a separate file — useful for large payloads
  - method: GET
    path: /products
    status: 200
    body_file: fixtures/products.json

  # Simulated latency — response is delayed by a random amount in [min, max]
  - method: GET
    path: /slow
    status: 200
    body:
      data: eventually
    delay:
      min: 100ms
      max: 500ms

  # Chaos injection — returns an error status 30 % of the time
  - method: GET
    path: /flaky
    status: 200
    body:
      data: success
    chaos:
      probability: 0.3
      status: 500
      body: '{"error": "internal server error"}'

# Tip: POST /__ditto/reset to clear all scenario state and request history
# Tip: GET /__ditto/routes to see all configured routes

# Stateful scenarios — model multi-step flows that depend on previous calls
scenarios:
  - name: cart-flow
    steps:
      - on: "POST /cart/items"
        set_state: item_added
        status: 201
        body:
          message: item added
      - on: "GET /cart"
        when_state: item_added
        set_state: item_added
        status: 200
        body:
          items:
            - id: 1
              name: Widget
      - on: "POST /cart/checkout"
        when_state: item_added
        set_state: checked_out
        status: 200
        body:
          order_id: ORD-001
`
