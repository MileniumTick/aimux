package tui

import (
	"encoding/json"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/jchavarriam/aimux/internal/domain"
)

var (
	// Header: bold white on subtle gray, no border
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("236")).
			Padding(0, 2)

	// Divider line below header
	dividerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("237"))

	rowEvenStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("252"))

	rowOddStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("236"))

	inactiveStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	activeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#888888", Dark: "#555555"}).
			Italic(true).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("167"))
)

// RenderTable renders the status table showing CLIs and active multiplexes.
func RenderTable(providers []domain.Provider, activeMultiplexes []domain.ActiveMultiplex, targetCLIs []domain.TargetCLI, termWidth int) string {
	var b strings.Builder

	activeByCLI := make(map[int64]domain.ActiveMultiplex)
	for _, am := range activeMultiplexes {
		activeByCLI[am.TargetCLIID] = am
	}

	availWidth := termWidth
	if availWidth < 72 {
		availWidth = 72
	}

	// Columns: CLI 20%, Provider 25%, Models rest, Status 10
	statW := 8
	cliW := availWidth * 20 / 100
	provW := availWidth * 25 / 100
	modW := availWidth - cliW - provW - statW

	minCLI, minProv, minStat, minMod := 16, 10, 6, 15
	if cliW < minCLI {
		cliW = minCLI
	}
	if provW < minProv {
		provW = minProv
	}
	if statW < minStat {
		statW = minStat
	}
	if modW < minMod {
		modW = minMod
	}
	total := cliW + provW + modW + statW
	if total > availWidth {
		over := total - availWidth
		modW -= over
		if modW < minMod {
			modW = minMod
		}
	}

	// Header
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerStyle.Width(cliW).Render("CLI"),
		headerStyle.Width(provW).Render("Provider"),
		headerStyle.Width(modW).Render("Models"),
		headerStyle.Width(statW).Render("Status"),
	)
	divider := dividerStyle.Width(availWidth).Render(strings.Repeat("─", availWidth))

	if len(targetCLIs) == 0 {
		row := lipgloss.JoinHorizontal(lipgloss.Top,
			rowEvenStyle.Width(cliW).Render("---"),
			rowEvenStyle.Width(provW).Render("---"),
			rowEvenStyle.Width(modW).Render("---"),
			inactiveStyle.Width(statW).Render("INACTIVE"),
		)
		b.WriteString(lipgloss.JoinVertical(lipgloss.Left,
			header, divider, row,
		))
		b.WriteString("\n\n")
		b.WriteString(hintStyle.Render("No CLIs configured — run 'aimux init' first"))
		return b.String()
	}

	rows := []string{header, divider}
	for i, cli := range targetCLIs {
		rowStyle := rowEvenStyle
		if i%2 == 1 {
			rowStyle = rowOddStyle
		}

		am, hasActive := activeByCLI[cli.ID]
		if hasActive {
			mappings := make(map[string]string)
			modelsStr := "---"
			if err := json.Unmarshal([]byte(am.ModelMappings), &mappings); err == nil && len(mappings) > 0 {
				modelIDs := make([]string, 0, len(mappings))
				for _, v := range mappings {
					if v != "" {
						modelIDs = append(modelIDs, v)
					}
				}
				if len(modelIDs) > 0 {
					modelsStr = strings.Join(modelIDs, ", ")
				}
			}

			providerName := am.ProviderName
			if providerName == "" {
				providerName = "---"
			}

			row := lipgloss.JoinHorizontal(lipgloss.Top,
				rowStyle.Width(cliW).Render(cli.Name),
				rowStyle.Width(provW).Render(providerName),
				rowStyle.Width(modW).Render(truncate(modelsStr, modW-1)),
				activeStyle.Width(statW).Render("ACTIVE"),
			)
			rows = append(rows, row)
		} else {
			row := lipgloss.JoinHorizontal(lipgloss.Top,
				rowStyle.Width(cliW).Render(cli.Name),
				rowStyle.Width(provW).Render("---"),
				rowStyle.Width(modW).Render("---"),
				inactiveStyle.Width(statW).Render("INACTIVE"),
			)
			rows = append(rows, row)
		}
	}

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Enter = Select action · ↑/↓ navigate menu · q quit"))
	return b.String()
}

// RenderProviderList renders the provider management table.
func RenderProviderList(providers []domain.Provider, selectedID int64, termWidth int) string {
	if len(providers) == 0 {
		return hintStyle.Render("No providers configured. Press 'a' to add one.")
	}

	// Dynamic columns
	nameW := 18
	urlW := 28
	modelsW := 14
	statW := 8

	if termWidth > 0 {
		totalPadding := 6
		availWidth := termWidth - totalPadding
		minName := 10
		minURL := 12
		minStat := 6
		minModels := 8
		reserved := minName + minURL + minStat
		modAvail := availWidth - reserved
		if modAvail > minModels {
			nameW = minName
			urlW = availWidth - minName - minStat - minModels
			modelsW = modAvail / 2
			statW = minStat
			if urlW < minURL {
				urlW = minURL
			}
		}
	}

	var b strings.Builder
	header := lipgloss.JoinHorizontal(lipgloss.Top,
		headerStyle.Width(nameW).Render("Name"),
		headerStyle.Width(urlW).Render("Base URL"),
		headerStyle.Width(modelsW).Render("Models"),
		headerStyle.Width(statW).Render("Status"),
	)
	dispW := termWidth
	if dispW < 2 {
		dispW = 72
	}
	divider := dividerStyle.Width(dispW - 2).Render(strings.Repeat("─", dispW-2))

	rows := []string{header, divider}
	for i, p := range providers {
		rowStyle := rowEvenStyle
		if i%2 == 1 {
			rowStyle = rowOddStyle
		}

		name := p.Name
		if p.ID == selectedID {
			name = "> " + name
		} else {
			name = "  " + name
		}

		baseURL := p.BaseURL
		urlDisplayLen := urlW - 2
		if urlDisplayLen < 0 {
			urlDisplayLen = 0
		}
		if len(baseURL) > urlDisplayLen && urlDisplayLen > 3 {
			baseURL = baseURL[:urlDisplayLen-3] + "..."
		} else if len(baseURL) > urlDisplayLen {
			baseURL = baseURL[:urlDisplayLen]
		}
		baseURL = "  " + baseURL

		status := "OK"
		statusRender := activeStyle
		if p.Status == "error" {
			status = "ERROR"
			statusRender = errorStyle
		}

		modelsStatus := "---"
		if p.Status == "active" {
			modelsStatus = "Yes"
		} else if p.Status == "error" {
			modelsStatus = "Failed"
		}

		row := lipgloss.JoinHorizontal(lipgloss.Top,
			rowStyle.Width(nameW).Render(truncate(name, nameW-1)),
			rowStyle.Width(urlW).Render(baseURL),
			rowStyle.Width(modelsW).Render(modelsStatus),
			statusRender.Width(statW).Render(status),
		)
		rows = append(rows, row)
	}

	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("↑/↓ navigate · Enter = Switch · a add · d delete · e edit · r retry · t test · Esc back"))
	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
