package tui

import (
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle   = aimuxT.TableHeader
	dividerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	rowEvenStyle  = aimuxT.TableRow
	rowOddStyle   = aimuxT.TableRowAlt
	inactiveStyle = aimuxT.Inactive
	activeStyle   = aimuxT.TableActive
	errorStyle    = lipgloss.NewStyle().Foreground(aimuxT.Red)
)

// RenderProviderList renders the provider management list with status, model count, and usage info.
func RenderProviderList(providers []domain.Provider, selectedID int64, termWidth int, allModels []domain.ProviderModel, activeMultiplexes []domain.ActiveMultiplex) string {
	if len(providers) == 0 {
		return helpStyle.Render("No providers configured. Press 'a' to add one.")
	}

	// Build model count per provider
	modelCounts := make(map[int64]int)
	for _, m := range allModels {
		modelCounts[m.ProviderID]++
	}

	// Build set of in-use provider IDs
	inUse := make(map[int64]bool)
	for _, am := range activeMultiplexes {
		inUse[am.ProviderID] = true
	}

	var b strings.Builder
	b.WriteString("\n")

	for i, p := range providers {
		selected := p.ID == selectedID

		titleStyle := aimuxT.ItemTitle
		detailStyle := aimuxT.ItemDesc
		if selected {
			titleStyle = aimuxT.SelTitle
			detailStyle = aimuxT.SelDesc
		}

		// Status label
		statusLabel := "OK"
		statusFg := aimuxT.Green
		if p.Status == "error" {
			statusLabel = "ERROR"
			statusFg = aimuxT.Red
		}
		statusLabel = lipgloss.NewStyle().Foreground(statusFg).Render(statusLabel)

		// In-use badge
		useLabel := ""
		if inUse[p.ID] {
			useLabel = lipgloss.NewStyle().
				Foreground(aimuxT.AccentAlt).
				Render(" in use")
		}

		b.WriteString(titleStyle.Render(fmt.Sprintf(" %s  %s%s", p.Name, statusLabel, useLabel)))
		b.WriteString("\n")

		// URL display
		url := p.BaseURL
		maxURL := termWidth - 20
		if maxURL < 30 {
			maxURL = 30
		} else if maxURL > 70 {
			maxURL = 70
		}
		if len(url) > maxURL {
			url = url[:maxURL-3] + "..."
		}

		// Model count and type
		modelInfo := fmt.Sprintf("%d models", modelCounts[p.ID])
		if len(p.ApiType) > 0 {
			modelInfo += fmt.Sprintf(" · %s", p.ApiType)
		}

		b.WriteString(detailStyle.Render(fmt.Sprintf("  %s", url)))
		b.WriteString("\n")
		b.WriteString(detailStyle.Render(fmt.Sprintf("  %s", modelInfo)))
		b.WriteString("\n")

		if i < len(providers)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate · Enter = Switch · a add · d delete · e edit · r retry · t test · Esc back"))
	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
