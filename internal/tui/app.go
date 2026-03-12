// Package tui provides a Bubble Tea terminal UI for watching live request logs.
package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelbailey123/ditto/internal/server"
)

const tickInterval = 500 * time.Millisecond

// tickMsg is sent by the periodic refresh timer.
type tickMsg struct{}

// Model is the root Bubble Tea model for the request log viewer.
type Model struct {
	table   table.Model
	entries []server.RequestEntry
	fetchFn func() []server.RequestEntry
	width   int
	height  int
}

// New constructs a Model that calls fetchFn every 500 ms to refresh the table.
func New(fetchFn func() []server.RequestEntry) Model {
	cols := buildColumns(80)

	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
		table.WithStyles(tableStyles()),
	)

	return Model{
		table:   t,
		fetchFn: fetchFn,
	}
}

// Init starts the periodic refresh timer.
func (m Model) Init() tea.Cmd {
	return tick()
}

// Update processes incoming messages and returns an updated model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.resize()
		return m, nil

	case tickMsg:
		m.entries = m.fetchFn()
		m.table.SetRows(entriesToRows(m.entries, m.table.Columns()))
		return m, tick()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			m.entries = nil
			m.table.SetRows(nil)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the full TUI screen.
func (m Model) View() string {
	header := HeaderStyle.Render(
		fmt.Sprintf("Ditto Request Log  •  %d request(s)", len(m.entries)),
	)

	footer := FooterStyle.Render("↑/k up  ↓/j down  r reset  q quit")

	// Build a border around the table using lipgloss.
	tableView := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("238")).
		Render(m.table.View())

	return lipgloss.JoinVertical(lipgloss.Left, header, tableView, footer)
}

// resize updates column widths and table height to match the current terminal.
func (m Model) resize() Model {
	if m.width == 0 || m.height == 0 {
		return m
	}

	cols := buildColumns(m.width)
	m.table.SetColumns(cols)

	// Reserve rows for the header, border (2), and footer.
	const chrome = 5
	tableHeight := m.height - chrome
	if tableHeight < 3 {
		tableHeight = 3
	}

	t := table.New(
		table.WithColumns(cols),
		table.WithRows(m.table.Rows()),
		table.WithFocused(true),
		table.WithHeight(tableHeight),
		table.WithStyles(tableStyles()),
	)
	m.table = t
	return m
}

// tick returns a command that fires a tickMsg after tickInterval.
func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// tableStyles returns Bubble Tea table styles consistent with the TUI palette.
func tableStyles() table.Styles {
	s := table.DefaultStyles()
	s.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		Padding(0, 1)
	s.Selected = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))
	s.Cell = lipgloss.NewStyle().Padding(0, 1)
	return s
}

// buildColumns returns column definitions sized proportionally for the given
// terminal width.
func buildColumns(totalWidth int) []table.Column {
	// Subtract 2 for the border padding each side, then the inter-column
	// separator characters added by the table widget (one per column).
	const numCols = 6
	const borderPad = 4
	usable := totalWidth - borderPad - numCols

	if usable < numCols*4 {
		usable = numCols * 4
	}

	// Fixed widths for the narrow columns.
	indexW := 4
	methodW := 8
	statusW := 6
	latencyW := 10
	timeW := 12
	pathW := usable - indexW - methodW - statusW - latencyW - timeW
	if pathW < 10 {
		pathW = 10
	}

	return []table.Column{
		{Title: "#", Width: indexW},
		{Title: "Method", Width: methodW},
		{Title: "Path", Width: pathW},
		{Title: "Status", Width: statusW},
		{Title: "Latency", Width: latencyW},
		{Title: "Time", Width: timeW},
	}
}

// entriesToRows converts server request entries into table rows. Entries are
// presented newest-first so the most recent request is always at the top.
// Colour styling is embedded via ANSI sequences.
func entriesToRows(entries []server.RequestEntry, cols []table.Column) []table.Row {
	rows := make([]table.Row, 0, len(entries))

	// Walk in reverse so most recent is row 0.
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		idx := len(entries) - i

		pathVal := truncate(e.Path, colWidth(cols, 2))

		rows = append(rows, table.Row{
			strconv.Itoa(idx),
			MethodStyle(e.Method).Render(padRight(e.Method, colWidth(cols, 1))),
			pathVal,
			StatusStyle(e.Status).Render(strconv.Itoa(e.Status)),
			formatLatency(e.Latency),
			e.Timestamp.Format("15:04:05.000"),
		})
	}

	return rows
}

// colWidth returns the Width of the column at position idx, or a fallback of 8.
func colWidth(cols []table.Column, idx int) int {
	if idx < len(cols) {
		return cols[idx].Width
	}
	return 8
}

// truncate shortens s to max runes, appending "…" if truncated.
func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// padRight right-pads s with spaces to width w.
func padRight(s string, w int) string {
	if len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

// formatLatency returns a human-readable representation of a duration suitable
// for display in the narrow Latency column.
func formatLatency(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%dns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(d.Nanoseconds())/1e6)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}
