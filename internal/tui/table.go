package tui

import (
	"encoding/json"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// RenderTable renders the dashboard status table, no help hints in body.
func RenderTable(providers []domain.Provider, activeMultiplexes []domain.ActiveMultiplex, targetCLIs []domain.TargetCLI, termWidth int) string {
	activeByCLI := make(map[int64]domain.ActiveMultiplex)
	for _, am := range activeMultiplexes {
		activeByCLI[am.TargetCLIID] = am
	}

	availWidth := termWidth
	if availWidth < 72 {
		availWidth = 72
	}

	cliW := availWidth * 28 / 100
	provW := availWidth * 28 / 100
	statW := 10
	modW := availWidth - cliW - provW - statW

	minCLI, minProv, minStat, minMod := 14, 10, 6, 12
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
		modW -= total - availWidth
		if modW < minMod {
			modW = minMod
		}
	}

	var b strings.Builder

	// Header
	b.WriteString(Row(
		Col{Style: headerStyle, Text: "CLI", Width: cliW},
		Col{Style: headerStyle, Text: "Provider", Width: provW},
		Col{Style: headerStyle, Text: "Models", Width: modW},
		Col{Style: headerStyle, Text: "Status", Width: statW},
	))
	b.WriteString("\n")
	b.WriteString(Divider(cliW, provW, modW, statW))

	if len(targetCLIs) == 0 {
		b.WriteString("\n")
		b.WriteString(RowAlt(0,
			Col{Style: rowEvenStyle, Text: "---", Width: cliW},
			Col{Style: rowEvenStyle, Text: "---", Width: provW},
			Col{Style: rowEvenStyle, Text: "---", Width: modW},
			Col{Style: inactiveStyle, Text: "INACTIVE", Width: statW},
		))
		b.WriteString("\n  ")
		b.WriteString(EmptyState("", "No CLIs configured — add one in Manage CLIs", availWidth))
		return b.String()
	}

	for i, cli := range targetCLIs {
		b.WriteString("\n")
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

			b.WriteString(RowAlt(i,
				Col{Style: rowEvenStyle, Text: cli.Name, Width: cliW},
				Col{Style: rowEvenStyle, Text: providerName, Width: provW},
				Col{Style: rowEvenStyle, Text: truncate(modelsStr, modW-1), Width: modW},
				Col{Style: activeStyle, Text: "ACTIVE", Width: statW},
			))
		} else {
			b.WriteString(RowAlt(i,
				Col{Style: rowEvenStyle, Text: cli.Name, Width: cliW},
				Col{Style: rowEvenStyle, Text: "---", Width: provW},
				Col{Style: rowEvenStyle, Text: "---", Width: modW},
				Col{Style: inactiveStyle, Text: "INACTIVE", Width: statW},
			))
		}
	}

	return b.String()
}

// RenderProviderList renders the provider management table, no hints in body.
func RenderProviderList(providers []domain.Provider, selectedID int64, termWidth int) string {
	if len(providers) == 0 {
		return "  " + EmptyState("", "No providers configured. Press 'a' to add one.", termWidth)
	}

	nameW := 18
	urlW := 30
	modelsW := 14
	statW := 8

	if termWidth > 0 {
		availWidth := termWidth - 4
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
	dispW := termWidth
	if dispW < 2 {
		dispW = 72
	}
	totalW := nameW + urlW + modelsW + statW
	if totalW > dispW-2 {
		overflow := totalW - (dispW - 2)
		urlW -= overflow
		if urlW < 12 {
			urlW = 12
		}
	}

	var b strings.Builder

	// Header
	b.WriteString(Row(
		Col{Style: headerStyle, Text: "Name", Width: nameW},
		Col{Style: headerStyle, Text: "Base URL", Width: urlW},
		Col{Style: headerStyle, Text: "Models", Width: modelsW},
		Col{Style: headerStyle, Text: "Status", Width: statW},
	))
	b.WriteString("\n")
	b.WriteString(Divider(nameW, urlW, modelsW, statW))

	for i, p := range providers {
		b.WriteString("\n")
		name := p.Name
		if p.ID == selectedID {
			name = "▸ " + name
		} else {
			name = "  " + name
		}

		baseURL := p.BaseURL
		urlDisplayLen := urlW - 4
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
			statusRender = errStyle
		}

		modelsStatus := "---"
		if p.Status == "active" {
			modelsStatus = "Yes"
		} else if p.Status == "error" {
			modelsStatus = "Failed"
		}

		if p.ID == selectedID {
			sel := selectedStyle
			if i%2 == 1 {
				sel = oddSelectedStyle
			}
			b.WriteString(RowPadded(sel,
				Col{Style: sel, Text: name, Width: nameW},
				Col{Style: sel, Text: baseURL, Width: urlW},
				Col{Style: sel, Text: modelsStatus, Width: modelsW},
				Col{Style: sel, Text: status, Width: statW},
			))
		} else {
			b.WriteString(RowAlt(i,
				Col{Style: rowEvenStyle, Text: name, Width: nameW},
				Col{Style: rowEvenStyle, Text: baseURL, Width: urlW},
				Col{Style: rowEvenStyle, Text: modelsStatus, Width: modelsW},
				Col{Style: statusRender, Text: status, Width: statW},
			))
		}
	}

	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
