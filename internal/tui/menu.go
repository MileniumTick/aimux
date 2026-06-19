package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	menuItemSwitch          = 0
	menuItemManageProviders = 1
	menuItemManageCLIs      = 2
	menuItemRestore         = 3
	menuItemExit            = 4
	menuItemCount           = 5
)

var (
	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(aimuxT.TextPrimary).
				Background(aimuxT.BgSelected).
				Padding(0, 2).
				Bold(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(aimuxT.Accent)

	menuNormalStyle = lipgloss.NewStyle().
			Foreground(aimuxT.TextSecondary).
			Padding(0, 2)

	menuInactiveStyle = lipgloss.NewStyle().
				Foreground(aimuxT.TextDim).
				Padding(0, 2).
				Italic(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(lipgloss.Color(""))

	helpStyle = lipgloss.NewStyle().
			Foreground(aimuxT.TextDim).
			Padding(0, 1).
			Italic(true)
)

// MenuItemCount returns the number of menu items.
func MenuItemCount() int {
	return menuItemCount
}

// RenderMenu renders the action menu with the given selected index.
func RenderMenu(selectedIndex int, hasProviders bool) string {
	var b strings.Builder

	items := []struct {
		label   string
		enabled bool
	}{
		{"Switch", hasProviders},
		{"Manage Providers", true},
		{"Manage CLIs", true},
		{"Restore Backup", true},
		{"Exit", true},
	}

	var rendered []string
	for i, item := range items {
		if !item.enabled {
			rendered = append(rendered, menuInactiveStyle.Render(item.label))
		} else if i == selectedIndex {
			rendered = append(rendered, menuSelectedStyle.Render(" "+item.label))
		} else {
			rendered = append(rendered, "  "+menuNormalStyle.Render(item.label))
		}
	}

	b.WriteString("  ")
	b.WriteString(strings.Join(rendered, "\n  "))
	b.WriteString("\n\n  ")
	b.WriteString(helpStyle.Render("↑/↓ k/j · Enter · q quit"))

	return b.String()
}
