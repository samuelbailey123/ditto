package tui

import "github.com/charmbracelet/lipgloss"

// Terminal-friendly styles for the request log viewer. All colours are chosen
// for readability on dark backgrounds.

var (
	// HeaderStyle is used for the banner text above the table.
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1).
			Foreground(lipgloss.Color("255"))

	// FooterStyle is used for the keybinding hint line below the table.
	FooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Padding(0, 1)

	// StatusOK colours 2xx responses.
	StatusOK = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	// StatusRedirect colours 3xx responses.
	StatusRedirect = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

	// StatusClient colours 4xx responses.
	StatusClient = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	// StatusServer colours 5xx responses.
	StatusServer = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))

	// MethodGET colours GET requests.
	MethodGET = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))

	// MethodPOST colours POST requests.
	MethodPOST = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	// MethodPUT colours PUT requests.
	MethodPUT = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	// MethodDELETE colours DELETE requests.
	MethodDELETE = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

// StatusStyle returns the appropriate lipgloss style for a given HTTP status code.
func StatusStyle(code int) lipgloss.Style {
	switch {
	case code >= 500:
		return StatusServer
	case code >= 400:
		return StatusClient
	case code >= 300:
		return StatusRedirect
	case code >= 200:
		return StatusOK
	default:
		return lipgloss.NewStyle()
	}
}

// MethodStyle returns the appropriate lipgloss style for a given HTTP method.
func MethodStyle(method string) lipgloss.Style {
	switch method {
	case "GET":
		return MethodGET
	case "POST":
		return MethodPOST
	case "PUT", "PATCH":
		return MethodPUT
	case "DELETE":
		return MethodDELETE
	default:
		return lipgloss.NewStyle()
	}
}
