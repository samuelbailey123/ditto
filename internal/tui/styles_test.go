package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"
)

func TestStatusStyle_2xx(t *testing.T) {
	s := StatusStyle(200)
	assert.Equal(t, StatusOK.GetForeground(), s.GetForeground())
}

func TestStatusStyle_3xx(t *testing.T) {
	s := StatusStyle(301)
	assert.Equal(t, StatusRedirect.GetForeground(), s.GetForeground())
}

func TestStatusStyle_4xx(t *testing.T) {
	s := StatusStyle(404)
	assert.Equal(t, StatusClient.GetForeground(), s.GetForeground())
}

func TestStatusStyle_5xx(t *testing.T) {
	s := StatusStyle(503)
	assert.Equal(t, StatusServer.GetForeground(), s.GetForeground())
}

func TestStatusStyle_Unknown(t *testing.T) {
	s := StatusStyle(100)
	// Informational/unknown — must return a non-nil style without panicking.
	assert.NotNil(t, s)
	// The foreground should not match any of the coloured styles.
	assert.NotEqual(t, StatusOK.GetForeground(), s.GetForeground())
}

func TestStatusStyle_BoundaryValues(t *testing.T) {
	tests := []struct {
		code int
		want lipgloss.Style
	}{
		{200, StatusOK},
		{299, StatusOK},
		{300, StatusRedirect},
		{399, StatusRedirect},
		{400, StatusClient},
		{499, StatusClient},
		{500, StatusServer},
		{599, StatusServer},
	}

	for _, tt := range tests {
		got := StatusStyle(tt.code)
		assert.Equal(t, tt.want.GetForeground(), got.GetForeground(),
			"StatusStyle(%d) foreground mismatch", tt.code)
	}
}

func TestMethodStyle_GET(t *testing.T) {
	s := MethodStyle("GET")
	assert.Equal(t, MethodGET.GetForeground(), s.GetForeground())
}

func TestMethodStyle_POST(t *testing.T) {
	s := MethodStyle("POST")
	assert.Equal(t, MethodPOST.GetForeground(), s.GetForeground())
}

func TestMethodStyle_PUT(t *testing.T) {
	s := MethodStyle("PUT")
	assert.Equal(t, MethodPUT.GetForeground(), s.GetForeground())
}

func TestMethodStyle_PATCH(t *testing.T) {
	s := MethodStyle("PATCH")
	assert.Equal(t, MethodPUT.GetForeground(), s.GetForeground())
}

func TestMethodStyle_DELETE(t *testing.T) {
	s := MethodStyle("DELETE")
	assert.Equal(t, MethodDELETE.GetForeground(), s.GetForeground())
}

func TestMethodStyle_Unknown(t *testing.T) {
	s := MethodStyle("OPTIONS")
	// Unknown methods must return a style without panicking.
	assert.NotNil(t, s)
}

func TestGlobalStyles_DefinedCorrectly(t *testing.T) {
	// Ensure none of the package-level vars are zero-value lipgloss.Style
	// (lipgloss.NewStyle() always returns a non-zero value with no foreground
	// set, but the coloured ones should have a foreground colour assigned).
	coloured := []struct {
		name  string
		style lipgloss.Style
	}{
		{"StatusOK", StatusOK},
		{"StatusRedirect", StatusRedirect},
		{"StatusClient", StatusClient},
		{"StatusServer", StatusServer},
		{"MethodGET", MethodGET},
		{"MethodPOST", MethodPOST},
		{"MethodPUT", MethodPUT},
		{"MethodDELETE", MethodDELETE},
	}

	// Each coloured style must have a non-empty foreground.
	for _, tc := range coloured {
		fg := tc.style.GetForeground()
		assert.NotEqual(t, lipgloss.Color(""), fg,
			"%s must have a non-empty foreground colour", tc.name)
	}
}

func TestHeaderStyle_IsBold(t *testing.T) {
	assert.True(t, HeaderStyle.GetBold(), "HeaderStyle must be bold")
}

func TestFooterStyle_IsNotBold(t *testing.T) {
	assert.False(t, FooterStyle.GetBold(), "FooterStyle should not be bold")
}
