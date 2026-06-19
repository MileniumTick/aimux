package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RenderStepIndicator renders a horizontal step progress indicator.
// Shows completed (●), current (●), and future (○) steps with a label.
//
// Example output:
//
//	Step 2 of 5: Select Provider
//	● ● ○ ○ ○
func RenderStepIndicator(step, total int, label string) string {
	if total == 0 || step == 0 {
		return ""
	}

	// Build dots
	var dots []string
	for i := 1; i <= total; i++ {
		switch {
		case i < step:
			dots = append(dots, aimuxT.StepDotDone.Render("●"))
		case i == step:
			dots = append(dots, aimuxT.StepDotCurrent.Render("●"))
		default:
			dots = append(dots, aimuxT.StepDotFuture.Render("○"))
		}
	}

	// Join dots with spacing
	dotsLine := strings.Join(dots, "  ")

	// Header: "Step X of Y: Label"
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(aimuxT.TextPrimary).
		Render(fmt.Sprintf("Step %d of %d", step, total))

	if label != "" {
		header += lipgloss.NewStyle().
			Foreground(aimuxT.TextSecondary).
			Render(fmt.Sprintf(": %s", label))
	}

	// Combine header and dots vertically
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		lipgloss.NewStyle().Padding(0, 2).Render(dotsLine),
	)
}
