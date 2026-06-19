package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// aimuxT is the global theme instance.
var aimuxT = newAimuxTheme()

// aimuxTheme holds all reusable styles for the TUI.
type aimuxTheme struct {
	Accent    lipgloss.Color
	AccentAlt lipgloss.Color

	TextPrimary   lipgloss.Color
	TextSecondary lipgloss.Color
	TextDim       lipgloss.Color

	BgBase      lipgloss.Color
	BgSelected  lipgloss.Color
	BgHighlight lipgloss.Color

	Green lipgloss.Color
	Red   lipgloss.Color

	Header   lipgloss.Style
	Title    lipgloss.Style
	Help     lipgloss.Style
	Divider  func(width int) string
	Padding  lipgloss.Style
	Inactive lipgloss.Style

	ItemTitle lipgloss.Style
	ItemDesc  lipgloss.Style
	SelTitle  lipgloss.Style
	SelDesc   lipgloss.Style

	TableHeader lipgloss.Style
	TableRow    lipgloss.Style
	TableRowAlt lipgloss.Style
	TableActive lipgloss.Style
}

func newAimuxTheme() aimuxTheme {
	t := aimuxTheme{
		Accent:        lipgloss.Color("#7C5CFC"),
		AccentAlt:     lipgloss.Color("#A78BFA"),
		TextPrimary:   lipgloss.Color("255"),
		TextSecondary: lipgloss.Color("252"),
		TextDim:       lipgloss.Color("241"),
		BgBase:        lipgloss.Color("236"),
		BgSelected:    lipgloss.Color("239"),
		BgHighlight:   lipgloss.Color("237"),
		Green:         lipgloss.Color("42"),
		Red:           lipgloss.Color("167"),
	}

	t.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Background(t.BgBase).
		Padding(0, 2)

	t.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Padding(0, 1)

	t.Help = lipgloss.NewStyle().
		Foreground(t.TextDim).
		Padding(0, 1).
		Italic(true)

	t.Divider = func(width int) string {
		style := lipgloss.NewStyle().Foreground(t.TextDim)
		return style.Width(width).Render(strings.Repeat("─", width))
	}

	t.Padding = lipgloss.NewStyle().Padding(1, 2)

	t.Inactive = lipgloss.NewStyle().
		Foreground(t.TextDim)

	t.ItemTitle = lipgloss.NewStyle().
		Foreground(t.TextSecondary).
		Padding(0, 1).
		Bold(true)

	t.ItemDesc = lipgloss.NewStyle().
		Foreground(t.TextDim).
		Padding(0, 1)

	t.SelTitle = lipgloss.NewStyle().
		Foreground(t.TextPrimary).
		Background(t.BgSelected).
		Padding(0, 1).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(t.Accent)

	t.SelDesc = lipgloss.NewStyle().
		Foreground(t.TextSecondary).
		Background(t.BgSelected).
		Padding(0, 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(t.Accent)

	t.TableHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Background(t.BgBase).
		Padding(0, 2)

	t.TableRow = lipgloss.NewStyle().
		Foreground(t.TextSecondary).
		Padding(0, 2)

	t.TableRowAlt = lipgloss.NewStyle().
		Foreground(t.TextSecondary).
		Background(t.BgHighlight).
		Padding(0, 2)

	t.TableActive = lipgloss.NewStyle().
		Foreground(t.Green)

	return t
}
