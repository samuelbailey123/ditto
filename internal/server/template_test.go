package server

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

// baseCtx returns a minimal TemplateContext suitable for most tests.
func baseCtx() TemplateContext {
	return TemplateContext{
		Params:  map[string]string{"name": "alice"},
		Query:   map[string][]string{"page": {"2"}},
		Headers: http.Header{"X-Request-Id": {"abc123"}},
		Method:  "GET",
		Path:    "/test",
	}
}

func TestRenderTemplate_PathParams(t *testing.T) {
	body := "Hello {{.Params.name}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Hello alice" {
		t.Errorf("got %q, want %q", got, "Hello alice")
	}
}

func TestRenderTemplate_QueryParams(t *testing.T) {
	body := `Page {{index .Query.page 0}}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Page 2" {
		t.Errorf("got %q, want %q", got, "Page 2")
	}
}

func TestRenderTemplate_Now(t *testing.T) {
	body := "{{now}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the output parses as RFC3339.
	_, parseErr := time.Parse(time.RFC3339, got)
	if parseErr != nil {
		t.Errorf("now output %q is not a valid RFC3339 timestamp: %v", got, parseErr)
	}
}

func TestRenderTemplate_UUID(t *testing.T) {
	body := "{{uuid}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// UUID v4: xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx
	uuidRE := regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidRE.MatchString(got) {
		t.Errorf("uuid output %q does not match UUID v4 format", got)
	}
}

func TestRenderTemplate_Seq(t *testing.T) {
	body := "{{range seq 3}}item {{end}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "item item item " {
		t.Errorf("got %q, want %q", got, "item item item ")
	}
}

func TestRenderTemplate_NoTemplate(t *testing.T) {
	body := `{"message": "plain text, no template markers"}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != body {
		t.Errorf("plain body should be returned unchanged: got %q", got)
	}
}

func TestRenderTemplate_InvalidTemplate(t *testing.T) {
	// Malformed template — missing closing braces.
	body := "{{.Invalid"
	got, err := renderTemplate(body, baseCtx())
	// err should be nil (fail-open contract: error is swallowed).
	if err != nil {
		t.Fatalf("expected nil error for invalid template (fail-open), got: %v", err)
	}
	// Original body must be returned unchanged.
	if got != body {
		t.Errorf("invalid template should return original body unchanged: got %q", got)
	}
}

func TestRenderTemplate_Upper(t *testing.T) {
	body := "{{upper .Method}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "GET" {
		t.Errorf("got %q, want %q", got, "GET")
	}
}

func TestRenderTemplate_Lower(t *testing.T) {
	body := "{{lower .Method}}"
	ctx := baseCtx()
	ctx.Method = "DELETE"
	got, err := renderTemplate(body, ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "delete" {
		t.Errorf("got %q, want %q", got, "delete")
	}
}

func TestRenderTemplate_Default_WithValue(t *testing.T) {
	body := `{{default .Params.name "anonymous"}}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}

func TestRenderTemplate_Default_Fallback(t *testing.T) {
	body := `{{default .Params.missing "anonymous"}}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "anonymous" {
		t.Errorf("got %q, want %q", got, "anonymous")
	}
}

func TestRenderTemplate_JSON(t *testing.T) {
	body := `{{json .Params}}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Must produce valid JSON containing the param key.
	if !strings.Contains(got, `"name"`) || !strings.Contains(got, `"alice"`) {
		t.Errorf("json output %q should contain name:alice", got)
	}
}

func TestRenderTemplate_Headers(t *testing.T) {
	body := `{{index .Headers "X-Request-Id" 0}}`
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want %q", got, "abc123")
	}
}

func TestRenderTemplate_Path(t *testing.T) {
	body := "{{.Path}}"
	got, err := renderTemplate(body, baseCtx())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "/test" {
		t.Errorf("got %q, want %q", got, "/test")
	}
}
