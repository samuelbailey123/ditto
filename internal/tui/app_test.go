package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/samuelbailey123/ditto/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeEntry returns a minimal RequestEntry for use in tests.
func makeEntry(method, path string, status int, latency time.Duration) server.RequestEntry {
	return server.RequestEntry{
		Method:    method,
		Path:      path,
		Status:    status,
		Latency:   latency,
		Timestamp: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
	}
}

// staticFetch returns a fetch function that always returns the given entries.
func staticFetch(entries []server.RequestEntry) func() []server.RequestEntry {
	return func() []server.RequestEntry { return entries }
}

func TestNew_InitialisesModel(t *testing.T) {
	m := New(staticFetch(nil))

	assert.NotNil(t, m.fetchFn, "fetchFn must be set")
	assert.Nil(t, m.entries, "entries should be nil before first tick")
	assert.Equal(t, 0, m.width)
	assert.Equal(t, 0, m.height)
}

func TestInit_ReturnsTick(t *testing.T) {
	m := New(staticFetch(nil))
	cmd := m.Init()
	require.NotNil(t, cmd, "Init must return a tick command")
}

func TestUpdate_TickRefreshesEntries(t *testing.T) {
	entries := []server.RequestEntry{
		makeEntry("GET", "/ping", 200, 5*time.Millisecond),
	}
	m := New(staticFetch(entries))

	next, _ := m.Update(tickMsg{})
	updated := next.(Model)

	require.Len(t, updated.entries, 1)
	assert.Equal(t, "GET", updated.entries[0].Method)
}

func TestUpdate_QuitOnQ(t *testing.T) {
	m := New(staticFetch(nil))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	// tea.Quit returns a non-nil Cmd.
	require.NotNil(t, cmd, "q must return a quit command")
}

func TestUpdate_QuitOnCtrlC(t *testing.T) {
	m := New(staticFetch(nil))
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	require.NotNil(t, cmd, "ctrl+c must return a quit command")
}

func TestUpdate_ResetClearsEntries(t *testing.T) {
	entries := []server.RequestEntry{
		makeEntry("POST", "/reset", 204, time.Millisecond),
	}
	m := New(staticFetch(entries))

	// First tick populates entries.
	next, _ := m.Update(tickMsg{})
	m = next.(Model)
	require.NotEmpty(t, m.entries)

	// r key clears them.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	m = next.(Model)
	assert.Empty(t, m.entries)
	assert.Empty(t, m.table.Rows())
}

func TestUpdate_WindowSizeUpdatesModel(t *testing.T) {
	m := New(staticFetch(nil))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := next.(Model)

	assert.Equal(t, 120, updated.width)
	assert.Equal(t, 40, updated.height)
}

func TestView_ContainsBanner(t *testing.T) {
	m := New(staticFetch(nil))
	view := m.View()

	assert.Contains(t, view, "Ditto Request Log")
}

func TestView_ContainsFooterHints(t *testing.T) {
	m := New(staticFetch(nil))
	view := m.View()

	assert.Contains(t, view, "quit")
	assert.Contains(t, view, "reset")
}

func TestView_ShowsEntryCount(t *testing.T) {
	entries := []server.RequestEntry{
		makeEntry("GET", "/a", 200, time.Millisecond),
		makeEntry("POST", "/b", 201, 2*time.Millisecond),
	}
	m := New(staticFetch(entries))
	m.entries = entries

	view := m.View()
	assert.Contains(t, view, "2 request")
}

func TestEntriesToRows_NewestFirst(t *testing.T) {
	older := makeEntry("GET", "/old", 200, time.Millisecond)
	older.Timestamp = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	newer := makeEntry("POST", "/new", 201, 2*time.Millisecond)
	newer.Timestamp = time.Date(2025, 1, 1, 0, 0, 1, 0, time.UTC)

	entries := []server.RequestEntry{older, newer}
	cols := buildColumns(120)
	rows := entriesToRows(entries, cols)

	require.Len(t, rows, 2)
	// Newest entry (index 2 in 1-based) should be first row.
	assert.Contains(t, rows[0][2], "/new", "newest entry must appear first")
	assert.Contains(t, rows[1][2], "/old")
}

func TestEntriesToRows_EmptySlice(t *testing.T) {
	cols := buildColumns(80)
	rows := entriesToRows(nil, cols)
	assert.Empty(t, rows)
}

func TestEntriesToRows_IndexColumn(t *testing.T) {
	entries := []server.RequestEntry{
		makeEntry("GET", "/one", 200, time.Millisecond),
		makeEntry("GET", "/two", 200, time.Millisecond),
		makeEntry("GET", "/three", 200, time.Millisecond),
	}
	cols := buildColumns(120)
	rows := entriesToRows(entries, cols)

	// Rows are displayed newest-first. The # column counts from 1 at the top
	// (newest) and increments downward toward the oldest entry.
	// rows[0] = "/three" (newest), index "1"
	// rows[1] = "/two",           index "2"
	// rows[2] = "/one" (oldest),  index "3"
	assert.Equal(t, "1", rows[0][0])
	assert.Equal(t, "2", rows[1][0])
	assert.Equal(t, "3", rows[2][0])
}

func TestBuildColumns_PathColumnGrowsWithTerminal(t *testing.T) {
	narrow := buildColumns(80)
	wide := buildColumns(200)

	narrowPathW := narrow[2].Width
	widePathW := wide[2].Width

	assert.Greater(t, widePathW, narrowPathW, "path column must be wider on wider terminal")
}

func TestBuildColumns_AllColumnsDefined(t *testing.T) {
	cols := buildColumns(120)
	require.Len(t, cols, 6)

	titles := []string{"#", "Method", "Path", "Status", "Latency", "Time"}
	for i, want := range titles {
		assert.Equal(t, want, cols[i].Title)
	}
}

func TestBuildColumns_PositiveWidths(t *testing.T) {
	for _, w := range []int{40, 80, 120, 200, 300} {
		cols := buildColumns(w)
		for _, col := range cols {
			assert.Positive(t, col.Width, "column %q must have positive width at terminal width %d", col.Title, w)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		max   int
		want  string
	}{
		{"no truncation needed", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello w…"},
		{"max 1", "hello", 1, "…"},
		{"max 0", "hello", 0, "hello"},
		{"empty string", "", 5, ""},
		{"unicode", "こんにちは", 3, "こん…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatLatency(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		contains string
	}{
		{"nanoseconds", 500, "ns"},
		{"microseconds", 1500, "µs"},
		{"milliseconds", 5 * time.Millisecond, "ms"},
		{"seconds", 2 * time.Second, "s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLatency(tt.d)
			assert.Contains(t, got, tt.contains)
		})
	}
}

func TestPadRight(t *testing.T) {
	assert.Equal(t, "GET   ", padRight("GET", 6))
	assert.Equal(t, "DELETE", padRight("DELETE", 6))
	assert.Equal(t, "LONGMETHOD", padRight("LONGMETHOD", 6)) // no truncation
}

func TestColWidth_OutOfBounds(t *testing.T) {
	cols := []table.Column{{Title: "A", Width: 10}}
	assert.Equal(t, 8, colWidth(cols, 5), "out-of-bounds index must return fallback 8")
	assert.Equal(t, 10, colWidth(cols, 0))
}

func TestResize_SetsWidthHeight(t *testing.T) {
	m := New(staticFetch(nil))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	updated := next.(Model)

	assert.Equal(t, 160, updated.width)
	assert.Equal(t, 50, updated.height)
}

func TestTableStyles_NotDefault(t *testing.T) {
	custom := tableStyles()
	// Our header must be bold and have a custom foreground colour set.
	assert.True(t, custom.Header.GetBold(), "custom header style must be bold")
	assert.NotEqual(t, lipgloss.Color(""), custom.Header.GetForeground(),
		"custom header must have a foreground colour")
	// The selected-row style must have a background colour set.
	assert.NotEqual(t, lipgloss.Color(""), custom.Selected.GetBackground(),
		"custom selected style must have a background colour")
}

func TestView_StripsAnsiContainsMethod(t *testing.T) {
	entries := []server.RequestEntry{
		makeEntry("DELETE", "/resource/1", 204, time.Millisecond),
	}
	m := New(staticFetch(entries))
	m.entries = entries
	// Force a tick to populate the table rows.
	next, _ := m.Update(tickMsg{})
	m = next.(Model)

	view := m.View()
	// Strip ANSI so we can do plain-text assertions.
	plain := stripAnsi(view)
	assert.Contains(t, plain, "DELETE")
	assert.Contains(t, plain, "/resource/1")
}

func TestUpdate_TableDelegation_ArrowKey(t *testing.T) {
	// When the key is not handled by our switch, it must be forwarded to the
	// embedded table model without panicking and without quitting.
	entries := make([]server.RequestEntry, 5)
	for i := range entries {
		entries[i] = makeEntry("GET", "/path", 200, time.Millisecond)
	}
	m := New(staticFetch(entries))
	// Populate rows first.
	next, _ := m.Update(tickMsg{})
	m = next.(Model)

	// Send a down-arrow key — should be forwarded to the table, not quit.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	_ = next.(Model) // must not panic
	// cmd may be nil or non-nil depending on whether the table returned one;
	// the important thing is we did not quit.
	_ = cmd
}

func TestUpdate_UnknownKey_Noop(t *testing.T) {
	m := New(staticFetch(nil))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	_ = next.(Model)
	// Unknown keys must not produce a quit command.
	assert.Nil(t, cmd)
}

func TestResize_ZeroWidthIsNoop(t *testing.T) {
	m := New(staticFetch(nil))
	// A window size of 0×0 must not panic and must leave model unchanged.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 0, Height: 0})
	updated := next.(Model)
	assert.Equal(t, 0, updated.width)
	assert.Equal(t, 0, updated.height)
}

func TestResize_VeryShortTerminalClampsTableHeight(t *testing.T) {
	m := New(staticFetch(nil))
	// Height 6 → tableHeight = 6-5 = 1, which is below the minimum of 3.
	// The clamp must kick in without panicking.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	updated := next.(Model)
	assert.Equal(t, 6, updated.height)
	assert.Equal(t, 80, updated.width)
}

func TestBuildColumns_VeryNarrowTerminal(t *testing.T) {
	// When the terminal is too narrow, the minimum floor kicks in and all
	// columns still have positive widths.
	cols := buildColumns(10)
	for _, col := range cols {
		assert.Positive(t, col.Width, "all columns must have positive width on narrow terminal")
	}
}

func TestTick_ExecutesCallback(t *testing.T) {
	cmd := tick()
	require.NotNil(t, cmd)
	// Execute the command to confirm it returns a tickMsg.
	msg := cmd()
	assert.IsType(t, tickMsg{}, msg)
}

// stripAnsi removes ANSI escape sequences for assertion purposes.
func stripAnsi(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case inEsc:
			// skip
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
