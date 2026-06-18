package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ── Palette ──────────────────────────────────────────────────────────────
// Modern, clean palette. No muddy grays. Single source for the whole TUI.
const (
	colorAccent  = "75"  // bright blue — brand, focus, selection accent
	colorOK      = "42"  // green
	colorErr     = "196" // red
	colorWarn    = "214" // orange
	colorFg      = "254" // body text (near-white on dark terminals)
	colorFgDim   = "243" // secondary / muted text
	colorPanel   = "235" // very dark gray — panel/row backgrounds
	colorHeader  = "255" // bright white — headers, emphasis
	colorInactive = "238" // disabled items
)

// ── Styles ───────────────────────────────────────────────────────────────
// No other file in this package declares lipgloss.NewStyle(). Import theme.
var (
	brandStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorAccent))

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorHeader))

	rowEvenStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))

	rowOddStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg)).
			Background(lipgloss.Color(colorPanel))

	inactiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorInactive))

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorOK))

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorErr))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorHeader)).
			Background(lipgloss.Color("60")) // subdued indigo — selection

	oddSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorHeader)).
				Background(lipgloss.Color("59")) // slightly darker indigo for odd rows

	menuSelectedStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorHeader)).
				Background(lipgloss.Color("60"))

	menuNormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))

	menuInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorInactive)).
				Italic(true)

	statusStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(colorPanel)).
			Foreground(lipgloss.Color(colorFgDim)).
			Padding(0, 1)

	notifOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorOK))

	notifErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorErr))

	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAccent))
)

// ── Helpers ──────────────────────────────────────────────────────────────

// Brand returns the colored wordmark.
func Brand() string { return brandStyle.Render("aimux") }

// AppHeader renders the top bar: brand + title. No heavy bg, minimal.
func AppHeader(width int, title string) string {
	left := Brand()
	if title != "" {
		left += "  " + headerStyle.Render(title)
	}
	return headerStyle.
		Width(width).
		Background(lipgloss.Color(colorPanel)).
		Render(" " + left)
}

// StatusBar renders the bottom key-hints line, full width.
func StatusBar(width int, hints string) string {
	return statusStyle.Width(width).Render(hints)
}

// Notification renders a one-line message with severity coloring.
func Notification(msg string, isErr bool) string {
	if msg == "" {
		return ""
	}
	s := notifOKStyle
	icon := "✓"
	if isErr {
		s = notifErrStyle
		icon = "✗"
	}
	return s.Render(icon + " " + msg)
}


// Col sets up a single column: style + width + text.
type Col struct {
	Style lipgloss.Style
	Text  string
	Width int
}

// Row renders columns separated by a vertical bar.
func Row(cols ...Col) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, c.Style.Width(c.Width).Render(c.Text))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// Divider renders a horizontal line under the header row.
func Divider(widths ...int) string {
	if len(widths) == 0 {
		return ""
	}
	parts := make([]string, 0, len(widths))
	for _, w := range widths {
		parts = append(parts, lipgloss.NewStyle().
			Width(w).
			Foreground(lipgloss.Color("239")).
			Render(strings.Repeat("─", w)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// RowPadded returns a styled row with vertical padding.
func RowPadded(style lipgloss.Style, cols ...Col) string {
	var parts []string
	for _, c := range cols {
		parts = append(parts, c.Style.Width(c.Width).Render(c.Text))
	}
	return style.Padding(0, 1).Render(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
}

// EmptyState renders a dimmed info line, e.g. "No items available".
func EmptyState(icon, msg string, _ int) string {
	if icon == "" {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFgDim)).
			Italic(true).
			Render(msg)
	}
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFgDim)).
		Italic(true).
		Render(icon + " " + msg)
}

// RowAlt returns a padded row with alternating background.
func RowAlt(i int, cols ...Col) string {
	style := rowEvenStyle
	if i%2 == 1 {
		style = rowOddStyle
	}
	var parts []string
	for _, c := range cols {
		parts = append(parts, c.Style.Width(c.Width).Render(c.Text))
	}
	return style.Padding(0, 1).Render(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
}
