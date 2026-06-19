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

// MenuItemCount returns the number of menu items.
func MenuItemCount() int {
	return menuItemCount
}

// RenderMenu renders the action menu with the given selected index.
// Wraps items in a lipgloss card with NativeBorder for a clean, modern look.
func RenderMenu(selectedIndex int, hasProviders bool) string {
	items := []struct {
		label   string
		desc    string
		enabled bool
	}{
		{"Switch", "Activate a provider for a CLI", hasProviders},
		{"Manage Providers", "Add, edit, or remove providers", true},
		{"Manage CLIs", "Configure target CLI paths", true},
		{"Restore Backup", "Revert to a previous config", true},
		{"Exit", "Quit aimux", true},
	}

	var lines []string
	for i, item := range items {
		selected := i == selectedIndex

		if !item.enabled {
			// Disabled item: dimmed, italic
			line := lipgloss.NewStyle().
				Foreground(aimuxT.TextMuted).
				Italic(true).
				Padding(0, 2).
				Render("  " + item.label)
			lines = append(lines, line)
		} else if selected {
			// Selected: pink accent bar + bold text on pink bg + description
			indicator := lipgloss.NewStyle().
				Foreground(aimuxT.Accent).
				Bold(true).
				Blink(true).
				Render("▸ ")
			label := lipgloss.NewStyle().
				Foreground(aimuxT.TextPrimary).
				Background(aimuxT.Accent).
				Bold(true).
				Render(item.label)
			desc := lipgloss.NewStyle().
				Foreground(aimuxT.TextSecondary).
				Render("  " + item.desc)
			line := lipgloss.NewStyle().
				Padding(0, 2).
				Render(indicator + label + desc)
			lines = append(lines, line)
		} else {
			// Normal: muted text
			line := lipgloss.NewStyle().
				Foreground(aimuxT.TextSecondary).
				Padding(0, 2).
				Render("  " + item.label)
			lines = append(lines, line)
		}
	}

	// Wrap in card with section title
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(aimuxT.Accent).
		Padding(0, 1).
		Render("Actions")

	content := lipgloss.JoinVertical(lipgloss.Left,
		append([]string{title, " "}, lines...)...,
	)

	return aimuxT.Card.Copy().Width(60).Render(content)
}

// renderFooterActions renders the unified keybinding bar with key • desc format.
// Keys in AccentPurple (bold), descriptions in muted gray, separated by " • ".
func renderFooterActions(bindings []struct{ key, desc string }) string {
	var parts []string
	for _, b := range bindings {
		part := aimuxT.FooterKey.Render(b.key) +
			aimuxT.FooterSep.Render(" • ") +
			aimuxT.FooterDesc.Render(b.desc)
		parts = append(parts, part)
	}
	return strings.Join(parts, "  ")
}
