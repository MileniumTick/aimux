package tui

import (
	"strings"
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
			rendered = append(rendered, "> "+menuSelectedStyle.Render(item.label))
		} else {
			rendered = append(rendered, "  "+menuNormalStyle.Render(item.label))
		}
	}

	b.WriteString("  ")
	b.WriteString(strings.Join(rendered, "\n  "))

	return b.String()
}
