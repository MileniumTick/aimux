package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// truncateText safely truncates s to at most maxWidth display cells.
// Uses ansi.Truncate which properly handles ANSI escape sequences and
// multi-byte characters (CJK wide chars count as 2 cells).
// Appends tail (default "…") when truncation occurs.
// Returns raw s when maxWidth <= 0 or s is already short enough.
func truncateText(s string, maxWidth int) string {
	if maxWidth <= 0 || s == "" {
		return s
	}
	w := lipgloss.Width(s)
	if w <= maxWidth {
		return s
	}
	return ansi.Truncate(s, maxWidth, "…")
}

// truncateTextStyle returns s rendered through style with MaxWidth(maxWidth).
// Unlike truncateText, this preserves the full ANSI styling from the Lipgloss
// style (colors, bold, etc.) while truncating the visible content.
// Use this when you want to apply a style AND truncate at the same time.
func truncateTextStyle(s string, style lipgloss.Style, maxWidth int) string {
	if maxWidth <= 0 {
		return style.Render(s)
	}
	w := lipgloss.Width(s)
	if w <= maxWidth {
		return style.Render(s)
	}
	return style.MaxWidth(maxWidth).Render(s)
}

