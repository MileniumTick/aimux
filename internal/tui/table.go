package tui

import (
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

// RenderProviderList renders the provider management list with card-based styling.
// Each provider is displayed as a bordered card with status badge, URL, model count,
// and a logo indicator when a logo URL is available.
// deleteMode and deleteSet enable multi-select toggling for bulk delete.
func RenderProviderList(providers []domain.Provider, selectedID int64, termWidth int, allModels []domain.ProviderModel, activeMultiplexes []domain.ActiveMultiplex, deleteMode bool, deleteSet map[int64]bool) string {
	if len(providers) == 0 && !deleteMode && deleteSet == nil {
		msg := "No providers configured. Press 'a' to add one."
		if deleteMode {
			msg = "No providers match your search."
		}
		empty := lipgloss.NewStyle().
			Foreground(aimuxT.TextMuted).
			Padding(1, 2).
			Render(msg)
		return aimuxT.Card.Copy().Width(termWidth - 8).Render(empty)
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

	// Card width: terminal width minus outer padding
	cardW := termWidth - 8
	if cardW < 40 {
		cardW = 40
	}
	if cardW > 80 {
		cardW = 80
	}

	// Max text width within card: cardW minus border(2) minus padding(2)
	maxTextW := cardW - 4
	if maxTextW < 10 {
		maxTextW = 10
	}

	var cards []string

	for _, p := range providers {
		selected := p.ID == selectedID

		// Multi-select checkbox
		var checkBox string
		if deleteMode {
			if deleteSet[p.ID] {
				checkBox = lipgloss.NewStyle().Foreground(aimuxT.Accent).Bold(true).Render("[x] ")
			} else {
				checkBox = lipgloss.NewStyle().Foreground(aimuxT.TextMuted).Render("[ ] ")
			}
		}

		// Status badge
		var statusBadge string
		if p.Status == "error" {
			statusBadge = lipgloss.NewStyle().
				Foreground(aimuxT.Red).
				Bold(true).
				Render(" ERROR ")
		} else {
			statusBadge = lipgloss.NewStyle().
				Foreground(aimuxT.Green).
				Bold(true).
				Render(" OK ")
		}

		// In-use badge
		var useBadge string
		if inUse[p.ID] {
			useBadge = lipgloss.NewStyle().
				Foreground(aimuxT.Accent).
				Render("● in use")
		}

		// Selection indicator (arrow for normal mode, checkbox in multi-select)
		var selIndicator string
		if deleteMode {
			selIndicator = checkBox
		} else if selected {
			selIndicator = lipgloss.NewStyle().
				Foreground(aimuxT.Accent).
				Bold(true).
				Render("▸ ")
		} else {
			selIndicator = "  "
		}

		// Name line: indicator + truncated name + status + in-use
		nameStyle := aimuxT.ItemTitle
		// Leave ~20 cells for badges (OK + ● in use) and spacing
		nameMax := maxTextW - 20
		if nameMax < 8 {
			nameMax = 8
		}
		displayName := truncateText(p.Name, nameMax)
		nameLine := selIndicator + nameStyle.Render(displayName) + "  " + statusBadge
		if useBadge != "" {
			nameLine += "  " + useBadge
		}

		// URL line — ANSI-safe truncation
		urlDisplay := truncateText(p.BaseURL, maxTextW-4)
		urlStyle := aimuxT.ItemDesc
		urlLine := truncateTextStyle("  "+urlDisplay, urlStyle, maxTextW)

		// Model count line
		modelInfo := fmt.Sprintf("  %d models", modelCounts[p.ID])
		modelLine := urlStyle.Render(modelInfo)

		// Logo indicator — compact link when provider has a logo URL
		var logoLine string
		if p.LogoURL != "" {
			logoDisplay := truncateText(p.LogoURL, maxTextW-4)
			logoLine = truncateTextStyle("  "+logoDisplay, urlStyle.Copy().Faint(true).Italic(true), maxTextW)
		}

		// Build card content
		cardContent := lipgloss.JoinVertical(lipgloss.Left,
			nameLine,
			urlLine,
			modelLine,
		)
		if logoLine != "" {
			cardContent = lipgloss.JoinVertical(lipgloss.Left, cardContent, logoLine)
		}

		// Wrap in bordered card; selected gets accent border + ▸ indicator
		cardStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(aimuxT.Border).
			Padding(0, 1).
			Width(cardW)

		if selected {
			cardStyle = cardStyle.BorderForeground(aimuxT.Accent)
		}

		cards = append(cards, cardStyle.Render(cardContent))
	}

	// Join cards vertically with spacing
	return strings.Join(cards, "\n")
}
