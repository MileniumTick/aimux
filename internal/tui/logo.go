package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// RenderLogo builds the ASCII art AIMUX logotype with tagline and version badge.
// windowWidth is used for horizontal centering; pass 0 for no centering.
// version is the semantic version without "v" prefix (e.g., "0.2.0").
func RenderLogo(windowWidth int, version string) string {
	// Colors from project palette — Synthwave Outrun
	pink := aimuxT.Accent // #FF007F
	cyan := aimuxT.Cyan   // #00F0FF

	logoStyle := lipgloss.NewStyle().Foreground(pink).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(cyan).MarginLeft(2)
	bracketStyle := lipgloss.NewStyle().Foreground(pink)
	versionStyle := lipgloss.NewStyle().Foreground(aimuxT.TextSecondary)

	// Isotipo: aimux in lowercase block glyphs
	row1 := logoStyle.Render("▄▀█ █ █▀▄▀█ █ █ ▀▄▀")
	row2 := logoStyle.Render("█▀█ █ █ ▀ █ █▄█ ▄▀▄")

	// Tagline — editorial charm-style
	tagline := taglineStyle.Render("░▒▓█ AI MULTIPLEXER")

	// Version badge on the right
	versionBadge := lipgloss.JoinHorizontal(lipgloss.Left,
		bracketStyle.Render("["),
		versionStyle.Render("v"+version),
		bracketStyle.Render("]"),
	)

	// Assemble: row1 + tagline on first line, row2 + version on second
	logoBlock := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Bottom, row1, tagline),
		lipgloss.JoinHorizontal(lipgloss.Bottom, row2, lipgloss.NewStyle().MarginLeft(2).Render(versionBadge)),
	)

	// Optional horizontal centering
	if windowWidth > 0 {
		return lipgloss.Place(windowWidth, 4, lipgloss.Center, lipgloss.Center, logoBlock)
	}

	// Padding top/bottom for breathing room
	return lipgloss.NewStyle().Padding(1, 0, 1, 0).Render(logoBlock)
}
