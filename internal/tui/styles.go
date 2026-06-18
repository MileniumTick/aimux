package tui

import (
	"github.com/charmbracelet/lipgloss"
)

var dimItalicStyle = lipgloss.NewStyle().
	Italic(true).
	Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"})
