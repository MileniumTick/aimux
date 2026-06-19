package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// Canonical palette tokens — Synthwave Outrun aesthetic.
// All components MUST use these constants — never hardcode hex.
const (
	BaseBackground = lipgloss.Color("#0F051D") // ultra-deep purple/blue void
	SurfaceCard    = lipgloss.Color("#1A0F30") // subtle purple-gray for box bg
	BorderMuted    = lipgloss.Color("#3D2663") // mid purple for dividers
	TextPrimary    = lipgloss.Color("#FFFFFF") // pure white, high-contrast headings
	TextSecondary  = lipgloss.Color("#A894D3") // light lavender, labels/descriptions
	TextMuted      = lipgloss.Color("#5A4B76") // dark purple, secondary/disabled
	AccentPink     = lipgloss.Color("#FF007F") // neon laser fuchsia — primary accent (selections, keybinds)
	AccentCyan     = lipgloss.Color("#00F0FF") // electric cyan — secondary accent (data, counters, success)

	// Backwards-compatible aliases — kept so existing callers compile.
	// Values remapped to the synthwave scheme.
	AccentPurple    = AccentPink                // #FF007F — primary accent
	AccentPurpleDim = BorderMuted               // #3D2663 — de-emphasised accent
	AccentGreen     = AccentCyan                // #00F0FF — success/OK now cyan
	StatusError     = lipgloss.Color("#FF3B6B") // hot pink-red neon error
	StatusWarn      = lipgloss.Color("#FFB000") // amber neon warning
)

// aimuxT is the global theme instance — keeps the struct-based styles used
// throughout the package; all hex values resolve to the canonical tokens above.
var aimuxT = newAimuxTheme()

type aimuxTheme struct {
	// Palette (Synthwave Outrun — see const block above)
	Accent    lipgloss.Color // #FF007F — AccentPink (primary)
	AccentDim lipgloss.Color // #3D2663 — BorderMuted (de-emphasised)

	TextPrimary   lipgloss.Color // #FFFFFF
	TextSecondary lipgloss.Color // #A894D3
	TextMuted     lipgloss.Color // #5A4B76

	BgBase  lipgloss.Color // #0F051D
	Surface lipgloss.Color // #1A0F30
	Border  lipgloss.Color // #3D2663

	Green lipgloss.Color // #00F0FF — AccentCyan (success)
	Red   lipgloss.Color // #FF3B6B — neon red
	Warn  lipgloss.Color // #FFB000 — amber
	Cyan  lipgloss.Color // #00F0FF

	// Pre-built styles
	Header      lipgloss.Style
	Title       lipgloss.Style
	Help        lipgloss.Style
	Inactive    lipgloss.Style
	ItemTitle   lipgloss.Style
	ItemDesc    lipgloss.Style
	SelTitle    lipgloss.Style
	SelDesc     lipgloss.Style
	StatusOK    lipgloss.Style
	StatusWarn  lipgloss.Style
	StatusError lipgloss.Style
	Muted       lipgloss.Style // for empty-state text, "(no changes)", etc.

	// Card / container styles
	Card      lipgloss.Style // bordered panel with surface bg
	CardTitle lipgloss.Style // bold section label inside a card

	// Unified footer styles
	FooterKey  lipgloss.Style // keybind label (AccentPink, bold)
	FooterDesc lipgloss.Style // keybind description (TextSecondary)
	FooterSep  lipgloss.Style // " • " separator (BorderMuted)

	// Stepper dot styles
	StepDotDone    lipgloss.Style // completed step
	StepDotCurrent lipgloss.Style // active step
	StepDotFuture  lipgloss.Style // upcoming step

	// Diff view styles
	DiffAdded   lipgloss.Style // green for added lines
	DiffRemoved lipgloss.Style // red for removed lines
	DiffContext lipgloss.Style // muted for unchanged context lines
	DiffMuted   lipgloss.Style // collapse placeholder, dimmed
	Viewport    lipgloss.Style // bordered container for viewport
	DiffHeader  lipgloss.Style // section header in diff (e.g. "Current" / "New")
}

func newAimuxTheme() aimuxTheme {
	t := aimuxTheme{
		Accent:    AccentPurple,
		AccentDim: AccentPurpleDim,

		TextPrimary:   TextPrimary,
		TextSecondary: TextSecondary,
		TextMuted:     TextMuted,

		BgBase:  BaseBackground,
		Surface: SurfaceCard,
		Border:  BorderMuted,

		Green: AccentGreen,
		Red:   StatusError,
		Warn:  StatusWarn,
		Cyan:  AccentCyan,
	}

	t.Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Background(t.BgBase).
		Padding(0, 2)

	t.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Padding(0, 2)

	t.Help = lipgloss.NewStyle().
		Foreground(t.TextMuted).
		Padding(0, 1).
		Italic(true)

	t.Inactive = lipgloss.NewStyle().
		Foreground(t.TextMuted)

	t.ItemTitle = lipgloss.NewStyle().
		Foreground(t.TextSecondary).
		Padding(0, 1).
		Bold(true)

	t.ItemDesc = lipgloss.NewStyle().
		Foreground(t.TextMuted).
		Padding(0, 1)

	t.SelTitle = lipgloss.NewStyle().
		Foreground(t.TextPrimary).
		Background(t.Accent).
		Padding(0, 1).
		Bold(true)

	t.SelDesc = lipgloss.NewStyle().
		Foreground(t.TextPrimary).
		Background(t.Accent).
		Padding(0, 1)

	t.StatusOK = lipgloss.NewStyle().Foreground(t.Green).Bold(true)
	t.StatusWarn = lipgloss.NewStyle().Foreground(t.Warn).Bold(true)
	t.StatusError = lipgloss.NewStyle().Foreground(t.Red).Bold(true)

	t.Muted = lipgloss.NewStyle().
		Foreground(t.TextMuted)

	// Card styles — bordered panels with surface bg
	t.Card = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(1, 2)

	t.CardTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent).
		MarginBottom(1)

	// Unified footer
	t.FooterKey = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Accent)

	t.FooterDesc = lipgloss.NewStyle().
		Foreground(t.TextSecondary)

	t.FooterSep = lipgloss.NewStyle().
		Foreground(t.Border).
		Padding(0, 1)

	// Stepper dots
	t.StepDotDone = lipgloss.NewStyle().
		Foreground(t.Cyan)

	t.StepDotCurrent = lipgloss.NewStyle().
		Foreground(t.Accent).
		Bold(true)

	t.StepDotFuture = lipgloss.NewStyle().
		Foreground(t.TextMuted)

	// Diff view styles
	t.DiffAdded = lipgloss.NewStyle().
		Foreground(t.Green).
		Background(t.Surface)

	t.DiffRemoved = lipgloss.NewStyle().
		Foreground(t.Red)

	t.DiffContext = lipgloss.NewStyle().
		Foreground(t.TextSecondary)

	t.DiffMuted = lipgloss.NewStyle().
		Foreground(t.TextMuted).
		Italic(true)

	t.Viewport = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Border).
		Padding(0, 1)

	t.DiffHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.TextPrimary).
		Padding(0, 1)

	return t
}

// HuhTheme returns a huh theme styled for Synthwave Outrun.
// Uses ❯ as the focused field indicator in neon pink.
func HuhTheme() *huh.Theme {
	th := huh.ThemeBase()

	pink := lipgloss.Color("#FF007F")   // AccentPink
	cyan := lipgloss.Color("#00F0FF")   // AccentCyan
	muted := lipgloss.Color("#5A4B76")  // TextMuted
	dim := lipgloss.Color("#3D2663")    // BorderMuted (dim placeholder)
	darkBg := lipgloss.Color("#0F051D") // BaseBackground

	// Focused field: border in neon pink
	th.Focused.Base = th.Focused.Base.BorderForeground(pink)
	th.Focused.Card = th.Focused.Base
	th.Focused.Title = th.Focused.Title.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	th.Focused.NoteTitle = th.Focused.NoteTitle.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	th.Focused.Description = th.Focused.Description.Foreground(lipgloss.Color("#A894D3"))

	// Selectors: ❯ in pink
	th.Focused.SelectSelector = lipgloss.NewStyle().SetString("❯ ").Foreground(pink)
	th.Focused.MultiSelectSelector = lipgloss.NewStyle().SetString("❯ ").Foreground(pink)
	th.Focused.NextIndicator = th.Focused.NextIndicator.Foreground(pink)
	th.Focused.PrevIndicator = th.Focused.PrevIndicator.Foreground(pink)

	// Options — pink block for selected, muted circle for unselected
	th.Focused.Option = th.Focused.Option.Foreground(lipgloss.Color("#A894D3"))
	th.Focused.SelectedOption = th.Focused.SelectedOption.Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
	th.Focused.SelectedPrefix = lipgloss.NewStyle().SetString("● ").Foreground(pink)
	th.Focused.UnselectedPrefix = lipgloss.NewStyle().SetString("○ ").Foreground(muted)
	th.Focused.UnselectedOption = th.Focused.UnselectedOption.Foreground(lipgloss.Color("#A894D3"))

	// Text input: ❯ prompt in pink, cyan cursor
	th.Focused.TextInput.Prompt = lipgloss.NewStyle().SetString("❯ ").Foreground(pink)
	th.Focused.TextInput.Cursor = th.Focused.TextInput.Cursor.Foreground(cyan)
	th.Focused.TextInput.Placeholder = th.Focused.TextInput.Placeholder.Foreground(dim)
	th.Focused.TextInput.Text = th.Focused.TextInput.Text.Foreground(lipgloss.Color("#FFFFFF"))

	// Buttons — pink accent for focused
	th.Focused.FocusedButton = th.Focused.FocusedButton.Foreground(lipgloss.Color("#FFFFFF")).Background(pink).Bold(true).Padding(0, 2)
	th.Focused.BlurredButton = th.Focused.BlurredButton.Foreground(muted).Background(darkBg).Padding(0, 2)

	// Blurred = same as focused but without left border
	th.Blurred = th.Focused
	th.Blurred.Base = th.Focused.Base.Copy().BorderStyle(lipgloss.HiddenBorder()).PaddingLeft(0)
	th.Blurred.Card = th.Blurred.Base
	th.Blurred.MultiSelectSelector = lipgloss.NewStyle().SetString("  ")
	th.Blurred.NextIndicator = lipgloss.NewStyle()
	th.Blurred.PrevIndicator = lipgloss.NewStyle()

	// Group styles
	th.Group.Title = th.Focused.Title
	th.Group.Description = th.Focused.Description

	// Hide the thick border on textinput (lipgloss renders it on the outer Base)
	th.Blurred.TextInput = th.Focused.TextInput

	return th
}
