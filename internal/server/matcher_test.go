package server

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// matchQuery
// ---------------------------------------------------------------------------

func TestMatchQuery_AllPresent(t *testing.T) {
	expected := map[string]string{"q": "go", "page": "1"}
	actual := url.Values{"q": {"go"}, "page": {"1"}, "extra": {"ignored"}}
	assert.True(t, matchQuery(expected, actual))
}

func TestMatchQuery_Wildcard(t *testing.T) {
	expected := map[string]string{"q": "*"}
	assert.True(t, matchQuery(expected, url.Values{"q": {"anything"}}))
}

func TestMatchQuery_MissingKey(t *testing.T) {
	expected := map[string]string{"q": "*"}
	assert.False(t, matchQuery(expected, url.Values{}))
}

func TestMatchQuery_WrongValue(t *testing.T) {
	expected := map[string]string{"q": "exact"}
	assert.False(t, matchQuery(expected, url.Values{"q": {"other"}}))
}

// ---------------------------------------------------------------------------
// matchHeaders
// ---------------------------------------------------------------------------

func TestMatchHeaders_CaseInsensitive(t *testing.T) {
	expected := map[string]string{"x-api-key": "secret"}
	actual := http.Header{"X-Api-Key": {"secret"}}
	assert.True(t, matchHeaders(expected, actual))
}

func TestMatchHeaders_Wildcard(t *testing.T) {
	expected := map[string]string{"Authorization": "*"}
	actual := http.Header{"Authorization": {"Bearer token"}}
	assert.True(t, matchHeaders(expected, actual))
}

func TestMatchHeaders_MissingKey(t *testing.T) {
	expected := map[string]string{"X-Required": "yes"}
	assert.False(t, matchHeaders(expected, http.Header{}))
}

func TestMatchHeaders_WrongValue(t *testing.T) {
	expected := map[string]string{"X-Role": "admin"}
	actual := http.Header{"X-Role": {"user"}}
	assert.False(t, matchHeaders(expected, actual))
}

// ---------------------------------------------------------------------------
// matchBody
// ---------------------------------------------------------------------------

func TestMatchBody_ExactString(t *testing.T) {
	expected := map[string]interface{}{"name": "alice"}
	body := []byte(`{"name":"alice","age":30}`)
	assert.True(t, matchBody(expected, body))
}

func TestMatchBody_Wildcard(t *testing.T) {
	expected := map[string]interface{}{"name": "*"}
	body := []byte(`{"name":"bob"}`)
	assert.True(t, matchBody(expected, body))
}

func TestMatchBody_MissingKey(t *testing.T) {
	expected := map[string]interface{}{"role": "*"}
	body := []byte(`{"name":"bob"}`)
	assert.False(t, matchBody(expected, body))
}

func TestMatchBody_EmptyBodyEmptyExpected(t *testing.T) {
	assert.True(t, matchBody(map[string]interface{}{}, nil))
}

func TestMatchBody_EmptyBodyWithExpected(t *testing.T) {
	expected := map[string]interface{}{"key": "val"}
	assert.False(t, matchBody(expected, nil))
}

func TestMatchBody_InvalidJSON(t *testing.T) {
	expected := map[string]interface{}{"key": "*"}
	assert.False(t, matchBody(expected, []byte("not-json")))
}

func TestMatchBody_NestedMap(t *testing.T) {
	expected := map[string]interface{}{
		"user": map[string]interface{}{"name": "*"},
	}
	body := []byte(`{"user":{"name":"carol","age":25}}`)
	assert.True(t, matchBody(expected, body))
}

func TestMatchBody_NestedMapMismatch(t *testing.T) {
	expected := map[string]interface{}{
		"user": map[string]interface{}{"name": "alice"},
	}
	body := []byte(`{"user":{"name":"bob"}}`)
	assert.False(t, matchBody(expected, body))
}

func TestMatchBody_NumericValue(t *testing.T) {
	expected := map[string]interface{}{"count": float64(3)}
	body := []byte(`{"count":3}`)
	assert.True(t, matchBody(expected, body))
}

func TestMatchBody_NumericMismatch(t *testing.T) {
	expected := map[string]interface{}{"count": float64(5)}
	body := []byte(`{"count":3}`)
	assert.False(t, matchBody(expected, body))
}

// TestMatchBody_ExpectedMapActualScalar checks the branch where the expected
// value is a nested map but the actual value is a scalar.
func TestMatchBody_ExpectedMapActualScalar(t *testing.T) {
	expected := map[string]interface{}{
		"user": map[string]interface{}{"name": "alice"},
	}
	// "user" is a string, not an object — nested map comparison should fail.
	body := []byte(`{"user":"alice"}`)
	assert.False(t, matchBody(expected, body))
}

// TestMatchBody_StringVsNonString covers the branch where expected is a string
// but actual is a non-string (e.g. number).
func TestMatchBody_StringVsNonString(t *testing.T) {
	expected := map[string]interface{}{"id": "42"}
	// actual "id" is a number, not a string.
	body := []byte(`{"id":42}`)
	assert.False(t, matchBody(expected, body))
}

// TestMatchHeaders_ManualScanFallback constructs a Header whose canonical key
// differs from the expected key to exercise the manual lowercase scan path.
func TestMatchHeaders_ManualScanFallback(t *testing.T) {
	// Use a non-canonical key directly in the map (bypassing Header.Set).
	actual := http.Header{}
	actual["MY-CUSTOM-HEADER"] = []string{"value"}

	expected := map[string]string{"my-custom-header": "*"}
	assert.True(t, matchHeaders(expected, actual))
}
