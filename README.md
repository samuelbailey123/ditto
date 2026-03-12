# Ditto

Instant API mocking from YAML. A single binary, zero dependencies, no code required.

Define HTTP endpoints, response bodies, status codes, delays, chaos injection, and stateful scenarios in plain YAML — Ditto serves them instantly.

## Install

```bash
go install github.com/samuelbailey123/ditto/cmd/ditto@latest
```

Or download a binary from the [releases page](https://github.com/samuelbailey123/ditto/releases).

## Quick Start

```bash
# Generate a starter YAML file
ditto init

# Start the mock server
ditto serve ditto.yaml
```

That's it. Your mock API is running on `http://localhost:8080`.

## YAML Format

```yaml
name: my-api

defaults:
  headers:
    Content-Type: application/json
  cors:
    origins: ["*"]
    methods: ["GET", "POST", "PUT", "DELETE"]
    headers: ["Content-Type", "Authorization"]

routes:
  # Static response
  - method: GET
    path: /health
    status: 200
    body:
      status: ok

  # Path parameters — {id} extracts the value
  - method: GET
    path: /users/{id}
    status: 200
    body:
      id: 1
      name: Alice

  # Go templates in responses
  - method: GET
    path: /greet/{name}
    status: 200
    body: '{"greeting": "Hello, {{.Params.name}}!", "time": "{{now}}"}'

  # Request matching — route only fires when conditions are met
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
        name: "*"    # "*" means any non-empty value

  # Response from file
  - method: GET
    path: /products
    status: 200
    body_file: fixtures/products.json

  # Simulated latency
  - method: GET
    path: /slow
    status: 200
    body:
      data: eventually
    delay:
      min: 100ms
      max: 500ms

  # Chaos injection — fails 30% of the time
  - method: GET
    path: /flaky
    status: 200
    body:
      data: success
    chaos:
      probability: 0.3
      status: 500
      body: '{"error": "internal server error"}'

# Stateful scenarios — multi-step flows
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
```

## Commands

| Command | Description |
|---------|-------------|
| `ditto serve [files...]` | Start the mock server |
| `ditto validate [files...]` | Check YAML files for errors |
| `ditto init` | Generate a starter YAML file |
| `ditto record -u <url>` | Proxy a real API and record traffic as YAML |
| `ditto diff <a> <b>` | Compare two mock definition files |

### serve

```bash
ditto serve api.yaml                    # Serve on port 8080
ditto serve --port 9090 api.yaml        # Custom port
ditto serve --ui api.yaml               # Live TUI request dashboard
ditto serve auth.yaml users.yaml        # Multiple files merged
ditto serve --watch=false api.yaml      # Disable hot reload
ditto serve -v api.yaml                 # Verbose request logging
```

On startup, Ditto lists all routes with coloured output and logs every incoming request:

```
ditto serving on http://localhost:8080

  Routes:
    GET     /health
    GET     /users/{id}
    POST    /users
    DELETE  /users/{id}

  Watching: [api.yaml]

  GET    200  /health                         1.2ms
  POST   201  /users                         45.3ms
  GET    404  /unknown                        0.1ms
```

### record

Capture real API traffic and generate mock definitions:

```bash
ditto record --upstream https://api.example.com --port 9090
# Send requests to http://localhost:9090
# Press Ctrl+C to save as recorded.yaml
```

### diff

Compare two mock files to see what changed:

```bash
ditto diff old.yaml new.yaml
```

Output shows added (+), removed (-), and modified (~) routes with details.

## Go SDK

Use Ditto as a test helper in Go projects:

```go
import "github.com/samuelbailey123/ditto/pkg/ditto"

func TestMyAPI(t *testing.T) {
    // Starts a mock server, returns base URL
    // Server is automatically stopped when the test ends
    baseURL := ditto.Start(t, "testdata/api.yaml")

    resp, err := http.Get(baseURL + "/health")
    if err != nil {
        t.Fatal(err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        t.Errorf("expected 200, got %d", resp.StatusCode)
    }
}
```

### SDK Options

```go
// Custom port and host
baseURL := ditto.StartWithOptions(t,
    []string{"testdata/api.yaml"},
    ditto.WithPort(9090),
    ditto.WithHost("127.0.0.1"),
    ditto.WithVerbose(),
)
```

## Features

### Path Parameters

Use `{name}` in paths to capture segments:

```yaml
- method: GET
  path: /users/{id}/posts/{post_id}
  status: 200
  body: '{"user": "{{.Params.id}}", "post": "{{.Params.post_id}}"}'
```

### Wildcards

Paths ending with `*` match any suffix:

```yaml
- method: GET
  path: /static/*
  status: 200
  body_file: public/index.html
```

### Request Matching

Match routes based on query parameters, headers, and request body:

```yaml
- method: GET
  path: /search
  status: 200
  body: '{"results": []}'
  match:
    query:
      q: "*"           # Query param must exist
    headers:
      Authorization: "Bearer *"  # Header must exist
```

Use `"*"` to match any non-empty value.

### Go Templates

Response bodies containing `{{` are rendered as Go templates:

| Function | Description | Example |
|----------|-------------|---------|
| `{{.Params.name}}` | Path parameter | `/users/{{.Params.id}}` |
| `{{.Query.key}}` | Query parameter (slice) | `{{index .Query.page 0}}` |
| `{{.Method}}` | HTTP method | `{{.Method}}` |
| `{{.Path}}` | Request path | `{{.Path}}` |
| `{{now}}` | Current time (RFC3339) | `"time": "{{now}}"` |
| `{{uuid}}` | Random UUID v4 | `"id": "{{uuid}}"` |
| `{{seq N}}` | Integer slice [0, N) | `{{range seq 3}}...{{end}}` |
| `{{upper .Method}}` | Uppercase string | `{{upper .Method}}` |
| `{{lower .Method}}` | Lowercase string | `{{lower .Method}}` |
| `{{default .Params.x "fallback"}}` | Default value | `{{default .Params.name "anon"}}` |

### Latency Simulation

```yaml
# Fixed delay
delay:
  fixed: 200ms

# Random delay in range
delay:
  min: 100ms
  max: 500ms
```

### Chaos Injection

Randomly fail a percentage of requests:

```yaml
chaos:
  probability: 0.3    # Fail 30% of the time
  status: 500
  body: '{"error": "internal server error"}'
```

### Stateful Scenarios

Model multi-step API flows with state machines:

```yaml
scenarios:
  - name: auth-flow
    steps:
      - on: "POST /login"
        set_state: authenticated
        status: 200
        body:
          token: "tok_abc123"

      - on: "GET /profile"
        when_state: authenticated
        set_state: authenticated
        status: 200
        body:
          name: Alice

      - on: "POST /logout"
        when_state: authenticated
        set_state: ""              # Reset to initial state
        status: 200
        body:
          message: logged out
```

Scenarios take priority over static routes. Use `POST /__ditto/reset` to clear all state.

### Hot Reload

Ditto watches your YAML files and reloads automatically when they change. Invalid configs are rejected — the server keeps running with the last valid config.

Disable with `--watch=false`.

### CORS

Configure CORS in defaults:

```yaml
defaults:
  cors:
    origins: ["*"]
    methods: ["GET", "POST", "PUT", "DELETE"]
    headers: ["Content-Type", "Authorization"]
```

If no CORS config is provided, Ditto allows all origins by default.

### Multiple Files

Merge routes from multiple files:

```bash
ditto serve auth.yaml users.yaml products.yaml
```

Routes are concatenated in file order. Defaults are taken from the first file.

## Control Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/__ditto/reset` | POST | Clear all scenario state and request history |
| `/__ditto/routes` | GET | List all configured routes and scenario states |

## Development

```bash
make build        # Build binary with version info
make test         # Run tests
make test-race    # Run tests with race detector
make coverage     # Generate coverage report
make lint         # Run linters (requires golangci-lint)
make install      # Install to $GOPATH/bin
```

## License

MIT — see [LICENSE](LICENSE).
