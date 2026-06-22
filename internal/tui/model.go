package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// switch step definitions for the Switch flow stepper.
const (
	switchStepCLI          = 1
	switchStepProvider     = 2
	switchStepMapModels    = 3
	switchStepReviewConfig = 4
	switchStepConfirm      = 5
)

var switchStepLabels = map[int]string{
	switchStepCLI:          "Select Target CLI",
	switchStepProvider:     "Select Provider",
	switchStepMapModels:    "Map Models to Env Vars",
	switchStepReviewConfig: "Review Configuration",
	switchStepConfirm:      "Confirm & Apply",
}

// Minimum terminal dimensions for a usable layout
const (
	minTermWidth  = 50
	minTermHeight = 15
)

type viewType int

const (
	dashboardView viewType = iota
	providerListView
	addProviderView
	deleteProviderView
	switchTargetCLIView
	switchProviderView
	switchMapModelsView
	switchConfirmationView
	switchRegisterModelsView
	switchSelectModelsView
	switchManageBindingsView
	deleteBindingConfirmView
	manageCLIView
	editCLIPathView
	editProviderView
	restoreCLIView
	restoreBackupView
	switchAdvancedConfigView
	launchCLIView
	launchModelView
)

// uiLayout holds the precomputed screen regions (A3).
// Computed once per tea.WindowSizeMsg so every component knows its rect.
type uiLayout struct {
	headerH int // header bar height (1)
	footerH int // footer bar height (1)
	bodyW   int // body content width (terminal width minus horizontal padding)
	bodyH   int // body content height (terminal height minus header/footer)
}

// computeLayout derives the layout from the current terminal dimensions.
// Returns a zero-value layout if dimensions are not yet known.
func (m *model) computeLayout() uiLayout {
	if m.width == 0 || m.height == 0 {
		return uiLayout{}
	}
	headerH := 1
	footerH := 1
	return uiLayout{
		headerH: headerH,
		footerH: footerH,
		bodyW:   m.width - 4, // Padding(0, 2): 2 each side
		bodyH:   m.height - headerH - footerH,
	}
}

type (
	DashboardRefreshMsg struct{}

	SwitchToViewMsg struct {
		View viewType
	}

	FormResultMsg struct {
		View    viewType
		Success bool
		Error   string
	}

	notificationMsg struct {
		message  string
		isError  bool          // kept for backwards compat
		severity string        // "info", "warn", "error" — overrides isError when set
		ttl      time.Duration // 0 = use default per severity
	}

	clearNotificationMsg struct{}

	retryFetchResultMsg struct {
		diff *application.FetchDiff
		err  error
	}

	testConnectivityResultMsg struct {
		err error
	}

	applyResultMsg struct {
		result *domain.BackupResult
		err    error
	}
)

type keyMap struct {
	Up     key.Binding
	Down   key.Binding
	Enter  key.Binding
	Esc    key.Binding
	Quit   key.Binding
	Add    key.Binding
	Delete key.Binding
	Retry  key.Binding
	Test   key.Binding
	Edit   key.Binding
	Help   key.Binding
	Undo   key.Binding
	Launch key.Binding
}

var menuKeys = keyMap{
	Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Enter:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
	Esc:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	Add:    key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add provider")),
	Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete provider")),
	Retry:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "retry fetch")),
	Test:   key.NewBinding(key.WithKeys("t"), key.WithHelp("t", "test")),
	Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit provider")),
	Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Undo:   key.NewBinding(key.WithKeys("Z"), key.WithHelp("Z", "undo last apply")),
	Launch: key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "launch agent")),
}

// ShortHelp implements help.KeyMap (A1).
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Enter, k.Esc, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap (A1).
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Esc},
		{k.Add, k.Delete, k.Edit, k.Retry, k.Test},
		{k.Undo, k.Help, k.Quit},
	}
}

type model struct {
	providerUseCases *application.ProviderUseCases
	switchUseCases   *application.SwitchUseCases

	help help.Model // persistent help footer (A1)

	currentView viewType
	width       int
	height      int

	menuSelected int

	providers            []domain.Provider
	activeMultiplexes    []domain.ActiveMultiplex
	targetCLIs           []domain.TargetCLI
	allModels            []domain.ProviderModel
	switchProviderModels []domain.ProviderModel

	form *huh.Form

	addProviderResult  AddProviderResult
	editProviderResult EditProviderResult
	deleteConfirm      bool

	switchTargetCLIID          int64
	switchProviderID           int64
	switchEnvVars              []string
	switchExtractFn            func() MapModelsResult
	switchRegisteredModels     []string
	switchRegisterResult       RegisterModelsResult
	switchEditModelsResult     EditModelsResult
	switchSingleModelResult    SelectSingleModelResult
	switchIsCopilot            bool
	switchBackupPath           string
	switchDryRun               *application.DryRunResult
	switchDryRunCurrentConfig  string                   // current config file content (for diff view)
	switchModelMetadataSummary []string                 // display lines for advanced config review
	switchUsesEnvMapping       bool                     // true: Claude Code/Codex map env→model; false: pi/OpenCode list models directly
	switchInManageMode         bool                     // true when adding another provider to a CLI that already has bindings
	switchCLIBindings          []domain.ActiveMultiplex // bound providers for selected CLI
	switchRemoveMode           bool                     // true when picking a provider to remove
	switchDeleteConfirm        bool                     // true when confirming binding deletion
	switchSelectedBindingIdx   int                      // selected binding index in manage view

	switchStep       int    // current stepper step (1-based, 0=hidden)
	switchTotalSteps int    // total steps (0=hidden)
	switchStepLabel  string // current step label

	selectedProviderID int64

	selectedCLIID     int64
	editCLIPathResult EditCLIPathResult

	launchCLIName          string              // CLI name selected for launching via TUI
	launchProviderID       int64               // provider ID for launch
	launchModelName        string              // single model override for launch
	launchModelMappings    map[string]string   // env→model mappings for launch
	launchRegisteredModels []string            // registered models for launch

	showHelp bool // when true, render help overlay instead of current view

	lastUndoCLI string // CLI name for quick undo (Z key), empty = nothing to undo

	spinner    spinner.Model
	loading    bool   // true when an async operation is in progress
	loadingMsg string // contextual message shown with spinner

	notification      string
	notificationIsMsg bool

	restoreCLIName      string
	restoreSelectedPath string
	restoreBackups      []application.BackupOption

	// Diff viewport
	diffViewport viewport.Model // scrollable viewport for the dry-run diff
	diffContent  string         // rendered diff text set into the viewport

	// ponytail: updateInfo removed — unused. Re-add when update notification is implemented.

	version string // semver without "v" prefix, set at startup from main
}

func NewModel(providerUseCases *application.ProviderUseCases, switchUseCases *application.SwitchUseCases, version string) *model {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	s.Style = lipgloss.NewStyle().Foreground(aimuxT.Accent)

	h := help.New()
	h.Styles.ShortKey = lipgloss.NewStyle().Foreground(aimuxT.Accent)
	h.Styles.ShortDesc = lipgloss.NewStyle().Foreground(aimuxT.TextSecondary)
	h.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(aimuxT.TextMuted)
	h.ShowAll = false

	vp := viewport.New(0, 0)
	// ponytail: no custom keymap for viewport — parent routes ↑/k ↓/j directly.
	vp.Style = aimuxT.Viewport

	return &model{
		providerUseCases: providerUseCases,
		switchUseCases:   switchUseCases,
		version:          version,
		currentView:      dashboardView,
		menuSelected:     menuItemManageProviders,
		spinner:          s,
		help:             h,
		diffViewport:     vp,
	}
}

// loadingCmd sets the loading state and starts the spinner.
func (m *model) loadingCmd(msg string) tea.Cmd {
	m.loading = true
	m.loadingMsg = msg
	return func() tea.Msg {
		return spinner.TickMsg{Time: time.Now()}
	}
}

// setForm stores a form and applies terminal width AND height so forms fit
// the screen (A3). Uses bodyW/bodyH from the computed layout so forms don't
// overflow the header/footer chrome.
func (m *model) setForm(f *huh.Form) {
	m.form = f
	layout := m.computeLayout()
	w := m.width
	h := 0
	if layout.bodyW > 0 {
		w = layout.bodyW
		h = layout.bodyH
	}
	m.form = m.form.WithWidth(w)
	if h > 0 {
		m.form = m.form.WithHeight(h)
	}
}

// padded wraps content with terminal width and bg color so the TUI fills
// the screen instead of sitting in a corner.
// Note: per-line bg painting breaks huh form internals, so we only apply
// bg+width at the wrapper level. Full-screen fill is handled by chrome().
func (m *model) padded(content string) string {
	if m.width == 0 {
		return content
	}
	return lipgloss.NewStyle().
		Background(aimuxT.BgBase).
		Width(m.width).
		Padding(1, 2).
		Render(content)
}

// padRightBg appends bg-colored spaces to the right of s until it reaches
// width. Safe for ANSI content: only APPENDS, never re-renders existing
// content (unlike the broken line-by-line approach).
func padRightBg(s string, width int, bg lipgloss.Color) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	pad := strings.Repeat(" ", width-w)
	return s + lipgloss.NewStyle().Background(bg).Render(pad)
}

// fillBody pads body content to exactly bodyH lines with bg, filling the
// remaining screen. Side-rail prefix removed — bg fills the whole viewport
// cleanly. Right-fill is append-only (preserves ANSI from huh forms).
func (m *model) fillBody(content string, bodyH int) string {
	if m.width == 0 {
		return content
	}
	bg := aimuxT.BgBase
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = padRightBg(line, m.width, bg)
	}
	emptyLine := padRightBg("", m.width, bg)
	for len(lines) < bodyH {
		lines = append(lines, emptyLine)
	}
	if len(lines) > bodyH && bodyH > 0 {
		lines = lines[:bodyH]
	}
	return strings.Join(lines, "\n")
}

func (m *model) Init() tea.Cmd {
	// Safety: clear loading state after 3s if refreshData never returns
	return tea.Batch(
		m.refreshData,
		m.loadingCmd("Loading..."),
		tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
			return DashboardRefreshMsg{}
		}),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Cascade resize to all dimension-dependent subcomponents
		m.help.Width = msg.Width

		// Resize the active form (huh) with new body dimensions
		if m.form != nil {
			m.setForm(m.form)
		}

		// Resize the diff viewport (switch confirmation with dry-run)
		if m.diffContent != "" {
			bodyH := m.height - 2
			// renderConfirmationView has ~3 info-header lines before viewport
			vpHeight := bodyH - 3
			if vpHeight < 5 {
				vpHeight = 5
			}
			m.diffViewport.Width = m.width - 6
			m.diffViewport.Height = vpHeight
			m.diffViewport.SetContent(m.diffContent)
		}

		// For very small terminals, ignore further processing —
		// View() renders the contingency message above min size check.
		if m.width < minTermWidth || m.height < minTermHeight {
			return m, nil
		}
	}

	// Help overlay: ? toggles it on (works in forms too); any key dismisses (A1).
	if k, ok := msg.(tea.KeyMsg); ok {
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if key.Matches(k, menuKeys.Help) {
			m.showHelp = true
			return m, nil
		}
	}

	if m.form != nil {
		// Intercept Esc before huh processes it.
		// huh treats Esc as "previous field" inside multi-field forms,
		// but the user expects Esc = go back regardless of cursor position.
		switch k := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(k, menuKeys.Esc) {
				m.form = nil
				prev := m.previousView()
				m.currentView = prev
				cmd := m.reenterFormForView(prev)
				return m, cmd
			}
		}

		form, cmd := m.form.Update(msg)
		f, ok := form.(*huh.Form)
		if !ok {
			return m, cmd
		}
		m.setForm(f)

		if f.State == huh.StateCompleted {
			return m.handleFormCompletion()
		}
		if f.State == huh.StateAborted {
			m.form = nil
			prev := m.previousView()
			m.currentView = prev
			cmd := m.reenterFormForView(prev)
			return m, cmd
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case spinner.TickMsg:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case DashboardRefreshMsg:
		m.loading = false
		m.loadingMsg = ""
		// Auto-navigate to provider list on first run (0 providers)
		if len(m.providers) == 0 && m.currentView == dashboardView && m.notification == "" {
			m.currentView = providerListView
			return m, func() tea.Msg {
				return notificationMsg{
					message:  "Add your first provider to get started. Press 'a' to begin.",
					isError:  false,
					severity: "info",
					ttl:      10 * time.Second,
				}
			}
		}
		return m, nil

	case SwitchToViewMsg:
		m.currentView = msg.View
		return m, m.enterView(msg.View)

	case notificationMsg:
		m.notification = msg.message
		// Determine severity
		sev := msg.severity
		if sev == "" {
			if msg.isError {
				sev = "error"
			} else {
				sev = "info"
			}
		}
		// Icon per severity
		icon := "✓"
		switch sev {
		case "warn":
			icon = "⚠"
			m.notification = icon + " " + m.notification
			log.Printf("TUI warn: %s", msg.message)
		case "error":
			icon = "✗"
			m.notification = icon + " " + m.notification
			log.Printf("TUI error: %s", msg.message)
		default:
			m.notification = icon + " " + m.notification
		}
		m.notificationIsMsg = (sev != "error")
		// Per-severity TTL with message-level override
		ttl := msg.ttl
		if ttl == 0 {
			switch sev {
			case "info":
				ttl = 3 * time.Second
			case "warn":
				ttl = 5 * time.Second
			case "error":
				ttl = 0 // persist until Esc
			}
		}
		if ttl > 0 {
			return m, tea.Tick(ttl, func(_ time.Time) tea.Msg {
				return clearNotificationMsg{}
			})
		}
		return m, nil

	case clearNotificationMsg:
		m.notification = ""
		return m, nil

	case applyResultMsg:
		m.loading = false
		m.loadingMsg = ""
		m.switchDryRun = nil
		m.diffContent = ""
		m.diffViewport.SetContent("")
		if msg.err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Apply failed: %s", msg.err.Error()), isError: true}
			}
		}
		if msg.result != nil {
			m.switchBackupPath = msg.result.BackupPath
		}
		// Track last applied CLI for quick undo
		for _, c := range m.targetCLIs {
			if c.ID == m.switchTargetCLIID {
				m.lastUndoCLI = c.Name
				break
			}
		}
		return m, func() tea.Msg {
			msg := "Profile activated successfully"
			if m.lastUndoCLI != "" {
				msg += " · Z to undo"
			}
			return notificationMsg{message: msg, isError: false}
		}

	case retryFetchResultMsg:
		m.loading = false
		m.loadingMsg = ""
		var cmds []tea.Cmd
		cmds = append(cmds, func() tea.Msg {
			m.refreshData()
			return DashboardRefreshMsg{}
		})
		if msg.err != nil {
			cmds = append(cmds, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Retry failed: %s", msg.err.Error()), isError: true}
			})
		} else if msg.diff != nil {
			diffStr := fmt.Sprintf("Models refreshed: +%d added, -%d removed, %d total",
				msg.diff.Added, msg.diff.Removed, msg.diff.Total)
			if msg.diff.Error != "" {
				diffStr = msg.diff.Error
			}
			cmds = append(cmds, func() tea.Msg {
				return notificationMsg{message: diffStr, isError: false}
			})
		} else {
			cmds = append(cmds, func() tea.Msg {
				return notificationMsg{message: "Models refreshed", isError: false}
			})
		}
		return m, tea.Batch(cmds...)

	case testConnectivityResultMsg:
		m.loading = false
		var cmds []tea.Cmd
		if msg.err != nil {
			cmds = append(cmds, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Connectivity: %s", msg.err.Error()), isError: true}
			})
		} else {
			cmds = append(cmds, func() tea.Msg {
				return notificationMsg{message: "Connectivity OK", isError: false}
			})
		}
		cmds = append(cmds, func() tea.Msg {
			m.refreshData()
			return DashboardRefreshMsg{}
		})
		return m, tea.Batch(cmds...)

	default:
		return m, nil
	}
}

func (m *model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Loading guard: block ALL navigation input during async operations.
	// Only Quit (Ctrl+C / q) is allowed to exit cleanly.
	if m.loading {
		if key.Matches(msg, menuKeys.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}

	// Quick undo (Z key) — restore last backup
	if key.Matches(msg, menuKeys.Undo) && m.lastUndoCLI != "" {
		m.notification = ""
		cliName := m.lastUndoCLI
		m.lastUndoCLI = ""
		return m, func() tea.Msg {
			bp, err := m.switchUseCases.RestoreLatest(cliName)
			if err != nil {
				return notificationMsg{message: fmt.Sprintf("Undo failed: %s", err.Error()), isError: true}
			}
			return notificationMsg{message: fmt.Sprintf("Undone: restored %s", bp), isError: false}
		}
	}

	// Dismiss persistent notification on any key press
	if m.notification != "" && !m.notificationIsMsg {
		m.notification = ""
	}

	switch m.currentView {
	case dashboardView:
		switch {
		case key.Matches(msg, menuKeys.Up):
			if m.menuSelected > 0 {
				m.menuSelected--
				m.skipDisabledMenuItems()
			}
		case key.Matches(msg, menuKeys.Down):
			if m.menuSelected < menuItemCount-1 {
				m.menuSelected++
				m.skipDisabledMenuItems()
			}
		case key.Matches(msg, menuKeys.Enter):
			return m.handleMenuSelection()
		case key.Matches(msg, menuKeys.Launch):
			return m.startLaunchFlow()
		case key.Matches(msg, menuKeys.Quit):
			return m, tea.Quit
		}

	case providerListView:
		switch {
		case key.Matches(msg, menuKeys.Enter):
			if m.selectedProviderID > 0 && len(m.providers) > 0 {
				m.switchProviderID = m.selectedProviderID
				return m.startSwitchFlow()
			}
		case key.Matches(msg, menuKeys.Add):
			m.currentView = addProviderView
			m.addProviderResult = AddProviderResult{}
			m.setForm(NewAddProviderForm(&m.addProviderResult))
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Delete):
			if m.selectedProviderID > 0 {
				m.currentView = deleteProviderView
				m.deleteConfirm = false
				providerName := m.getProviderName(m.selectedProviderID)
				m.setForm(NewDeleteConfirmForm(providerName, &m.deleteConfirm))
				return m, m.form.Init()
			}
		case key.Matches(msg, menuKeys.Retry):
			if m.selectedProviderID > 0 {
				providerName := m.getProviderName(m.selectedProviderID)
				return m, tea.Batch(
					m.loadingCmd(fmt.Sprintf("Fetching models from %s...", providerName)),
					func() tea.Msg {
						diff, err := m.providerUseCases.RetryFetch(m.selectedProviderID)
						return retryFetchResultMsg{diff: diff, err: err}
					},
				)
			}
		case key.Matches(msg, menuKeys.Test):
			if m.selectedProviderID > 0 {
				providerName := m.getProviderName(m.selectedProviderID)
				return m, tea.Batch(
					m.loadingCmd(fmt.Sprintf("Testing %s...", providerName)),
					func() tea.Msg {
						err := m.providerUseCases.TestConnectivity(m.selectedProviderID)
						return testConnectivityResultMsg{err: err}
					},
				)
			}
		case key.Matches(msg, menuKeys.Edit):
			if m.selectedProviderID > 0 {
				m.editProviderResult = EditProviderResult{}
				provider := m.getProvider(m.selectedProviderID)
				if provider != nil {
					m.currentView = editProviderView
					m.setForm(NewEditProviderForm(*provider, &m.editProviderResult))
					return m, m.form.Init()
				}
			}
		case key.Matches(msg, menuKeys.Esc), key.Matches(msg, menuKeys.Quit):
			m.currentView = dashboardView
		case key.Matches(msg, menuKeys.Up):
			m.selectedProviderID = m.prevProviderID(m.selectedProviderID)
		case key.Matches(msg, menuKeys.Down):
			m.selectedProviderID = m.nextProviderID(m.selectedProviderID)
		}

	case switchManageBindingsView:
		switch {
		case key.Matches(msg, menuKeys.Enter):
			return m.editSelectedBinding()
		case key.Matches(msg, menuKeys.Add):
			m.currentView = switchProviderView
			providers, _ := m.providerUseCases.List()
			if len(providers) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers available", isError: false, severity: "warn"}
				}
			}
			m.setForm(NewSelectProviderForm(providers, &m.switchProviderID))
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Edit):
			return m.editSelectedBinding()
		case key.Matches(msg, menuKeys.Delete):
			if len(m.switchCLIBindings) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers to remove", isError: true}
				}
			}
			idx := m.switchSelectedBindingIdx
			if idx < 0 || idx >= len(m.switchCLIBindings) {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: false, severity: "warn"}
				}
			}
			b := m.switchCLIBindings[idx]
			m.switchProviderID = b.ProviderID
			m.switchDeleteConfirm = false
			providerName := b.ProviderName
			m.currentView = deleteBindingConfirmView
			m.setForm(NewDeleteConfirmForm(providerName, &m.switchDeleteConfirm))
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Up):
			if m.switchSelectedBindingIdx > 0 {
				m.switchSelectedBindingIdx--
			}
		case key.Matches(msg, menuKeys.Down):
			if m.switchSelectedBindingIdx < len(m.switchCLIBindings)-1 {
				m.switchSelectedBindingIdx++
			}
		case key.Matches(msg, menuKeys.Esc):
			m.switchRemoveMode = false
			m.switchInManageMode = false
			m.switchCLIBindings = nil
			m.currentView = dashboardView
			return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
		}

	case switchAdvancedConfigView:
		switch {
		case key.Matches(msg, menuKeys.Esc):
			if m.switchInManageMode {
				return m.enterManageBindingsView()
			}
			if m.switchUsesEnvMapping {
				m.currentView = switchMapModelsView
				form, extractFn := NewMapModelsForm(m.switchEnvVars, m.switchProviderModels)
				m.switchExtractFn = extractFn
				m.setForm(form)
				return m, m.form.Init()
			}
			// Rebuild model selection with previous choices pre-selected
			if m.switchIsCopilot {
				defaultModel := ""
				if len(m.switchRegisteredModels) > 0 {
					defaultModel = m.switchRegisteredModels[0]
				}
				m.switchSingleModelResult = SelectSingleModelResult{ModelName: defaultModel}
				m.setForm(NewSelectSingleModelForm(m.switchProviderModels, &m.switchSingleModelResult))
			} else {
				preselected := make(map[string]bool)
				for _, name := range m.switchRegisteredModels {
					preselected[name] = true
				}
				m.switchRegisterResult = RegisterModelsResult{}
				m.setForm(NewRegisterModelsForm(m.switchProviderModels, preselected, &m.switchRegisterResult))
			}
			m.currentView = switchSelectModelsView
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Enter):
			if m.switchInManageMode {
				name := m.getProviderName(m.switchProviderID)
				m.enterManageBindingsView()
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Provider '%s' added", name), isError: false}
				}
			}
			return m.proceedToDryRun()
		}

	case switchConfirmationView:
		switch {
		// Scroll keys: forward to viewport
		case key.Matches(msg, menuKeys.Up), key.Matches(msg, menuKeys.Down):
			if m.switchDryRun != nil {
				var cmd tea.Cmd
				m.diffViewport, cmd = m.diffViewport.Update(msg)
				return m, cmd
			}
			// fallthrough when dry run is nil
			return m, nil
		case msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown ||
			msg.Type == tea.KeyHome || msg.Type == tea.KeyEnd:
			if m.switchDryRun != nil {
				var cmd tea.Cmd
				m.diffViewport, cmd = m.diffViewport.Update(msg)
				return m, cmd
			}
			return m, nil
		case key.Matches(msg, menuKeys.Esc):
			m.switchDryRun = nil
			m.diffContent = ""
			m.switchBackupPath = ""
			m.resetSwitchState()
			m.currentView = dashboardView
			return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
		case key.Matches(msg, menuKeys.Enter):
			if m.switchDryRun == nil {
				m.diffContent = ""
				m.switchBackupPath = ""
				m.resetSwitchState()
				m.currentView = dashboardView
				return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
			}
			pid := m.switchProviderID
			if m.switchInManageMode {
				pid = 0 // apply all
			}
			cliName := ""
			for _, c := range m.targetCLIs {
				if c.ID == m.switchTargetCLIID {
					cliName = c.Name
					break
				}
			}
			return m, tea.Batch(
				m.loadingCmd(fmt.Sprintf("Applying binding to %s...", cliName)),
				func() tea.Msg {
					result, err := m.switchUseCases.Apply(m.switchTargetCLIID, pid)
					return applyResultMsg{result: result, err: err}
				},
			)
		}

	default:
	}
	return m, nil
}

func (m *model) handleMenuSelection() (tea.Model, tea.Cmd) {
	switch m.menuSelected {
	case menuItemSwitch:
		return m.startSwitchFlow()
	case menuItemLaunch:
		return m.startLaunchFlow()
	case menuItemManageProviders:
		m.currentView = providerListView
		if len(m.providers) > 0 {
			m.selectedProviderID = m.providers[0].ID
		}
		return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
	case menuItemManageCLIs:
		clis, err := m.switchUseCases.ListTargetCLIs()
		if err != nil || len(clis) == 0 {
			return m, func() tea.Msg {
				return notificationMsg{message: "No target CLIs configured", isError: true}
			}
		}
		m.targetCLIs = clis
		m.currentView = manageCLIView
		m.selectedCLIID = 0
		m.setForm(NewSelectCLIForm(clis, &m.selectedCLIID))
		return m, m.form.Init()
	case menuItemRestore:
		return m.enterRestoreFlow()
	case menuItemExit:
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) startSwitchFlow() (tea.Model, tea.Cmd) {
	m.resetSwitchState()
	m.currentView = switchTargetCLIView

	clis, err := m.switchUseCases.ListTargetCLIs()
	if err != nil || len(clis) == 0 {
		return m, func() tea.Msg {
			return notificationMsg{message: "No target CLIs configured", isError: true}
		}
	}

	m.targetCLIs = clis
	m.switchTargetCLIID = 0
	m.setForm(NewSelectTargetCLIForm(clis, &m.switchTargetCLIID))
	m.form.WithHeight(10)

	return m, m.form.Init()
}

func (m *model) enterView(view viewType) tea.Cmd {
	switch view {
	case providerListView:
		return func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
	case switchTargetCLIView:
		return m.startSwitchFlowCmd()
	default:
		return nil
	}
}

func (m *model) startSwitchFlowCmd() tea.Cmd {
	clis, err := m.switchUseCases.ListTargetCLIs()
	if err != nil || len(clis) == 0 {
		return func() tea.Msg {
			return notificationMsg{message: "No target CLIs configured", isError: true}
		}
	}
	m.targetCLIs = clis
	return nil
}

// startLaunchFlow lets the user pick CLI + provider on the fly, then
// shows available models to pick before launching with env vars.
func (m *model) startLaunchFlow() (tea.Model, tea.Cmd) {
	if len(m.providers) == 0 {
		return m, func() tea.Msg {
			return notificationMsg{message: "No providers configured. Add one first.", isError: false, severity: "warn"}
		}
	}

	clis, err := m.switchUseCases.ListTargetCLIs()
	if err != nil || len(clis) == 0 {
		return m, func() tea.Msg {
			return notificationMsg{message: "No target CLIs configured", isError: true}
		}
	}
	m.targetCLIs = clis

	m.launchCLIName = ""
	m.launchProviderID = 0
	m.launchModelName = ""

	cliOpts := make([]huh.Option[string], len(clis))
	for i, c := range clis {
		cliOpts[i] = huh.NewOption(c.Name, c.Name)
	}

	m.setForm(huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Launch Agent").
				Description("Select a CLI to launch").
				Options(cliOpts...).
				Value(&m.launchCLIName),
		).Title("1. Target CLI"),
		huh.NewGroup(
			huh.NewSelect[int64]().
				Title("Select Provider").
				Filtering(true).
				Options(m.providerOptions()...).
				Value(&m.launchProviderID),
		).Title("2. Provider"),
	).WithTheme(HuhTheme()))

	m.currentView = launchCLIView
	return m, m.form.Init()
}

// launchShowModels loads models for the selected provider and shows a
// multi-select model form. Called after launchCLIView form completes.
func (m *model) launchShowModels() (tea.Model, tea.Cmd) {
	models, err := m.switchUseCases.GetModelsForProvider(m.launchProviderID)
	if err != nil || len(models) == 0 {
		return m.launchAgent()
	}

	// Find the selected CLI to determine model mapping style
	var cliMutator string
	for _, c := range m.targetCLIs {
		if c.Name == m.launchCLIName {
			cliMutator = c.Mutator
			break
		}
	}

	usesEnvMapping := cliMutator == "claude-settings-json" || cliMutator == "codex-config-toml"
	m.switchUsesEnvMapping = usesEnvMapping

	// Reset model data
	m.launchModelMappings = make(map[string]string)
	m.launchRegisteredModels = nil

	if usesEnvMapping {
		// Env-mapping CLI (Claude Code, Codex): map each env var to a model
		var envVars []string
		for _, c := range m.targetCLIs {
			if c.Name == m.launchCLIName && c.EnvVars != "" {
				json.Unmarshal([]byte(c.EnvVars), &envVars)
				break
			}
		}
		if len(envVars) == 0 {
			envVars = []string{"ANTHROPIC_MODEL"}
		}

		m.switchEnvVars = envVars
		m.switchExtractFn = nil
		m.switchProviderModels = models

		form, extractFn := NewMapModelsForm(envVars, models)
		m.switchExtractFn = extractFn
		m.setForm(form)
	} else {
		// Multi-model CLI (pi, opencode): multi-select models
		m.switchRegisterResult = RegisterModelsResult{}
		m.setForm(NewSelectModelsForm(models, &m.switchRegisterResult))
	}

	m.currentView = launchModelView
	return m, m.form.Init()
}

// launchAgent writes the launch request and quits the TUI.
func (m *model) launchAgent() (tea.Model, tea.Cmd) {
	// Build model data: for env-mapping CLIs, use the mappings;
	// for multi-model CLIs, use registered models list.
	var modelData string
	if m.switchUsesEnvMapping && m.switchExtractFn != nil {
		mapped := m.switchExtractFn()
		if data, err := json.Marshal(mapped.Mappings); err == nil {
			modelData = string(data)
		}
	} else if len(m.switchRegisterResult.RegisteredModels) > 0 {
		if data, err := json.Marshal(m.switchRegisterResult.RegisteredModels); err == nil {
			modelData = string(data)
		}
	} else if m.launchModelName != "" {
		modelData = `{"ANTHROPIC_MODEL":"` + m.launchModelName + `"}`
	}

	req := map[string]string{
		"cli":      m.launchCLIName,
		"provider": m.getProviderName(m.launchProviderID),
		"models":   modelData,
	}
	launchPath := launchRequestPath()
	if lp := launchPath; lp != "" {
		if data, err := json.Marshal(req); err == nil {
			os.WriteFile(lp, data, 0600)
		}
	}
	m.launchCLIName = ""
	m.launchProviderID = 0
	m.launchModelName = ""
	return m, tea.Quit
}

// providerOptions returns huh options for all active providers.
func (m *model) providerOptions() []huh.Option[int64] {
	opts := make([]huh.Option[int64], 0, len(m.providers))
	for _, p := range m.providers {
		label := p.Name
		if p.Status == "error" {
			label += " [ERROR]"
		}
		opts = append(opts, huh.NewOption(label, p.ID))
	}
	return opts
}

func (m *model) enterRestoreFlow() (tea.Model, tea.Cmd) {
	clis, err := m.switchUseCases.ListTargetCLIs()
	if err != nil || len(clis) == 0 {
		return m, func() tea.Msg {
			return notificationMsg{message: "No target CLIs configured", isError: true}
		}
	}
	m.targetCLIs = clis
	m.currentView = restoreCLIView
	return m, m.startRestoreCLIForm()
}

// reenterFormForView creates the appropriate form for re-entering a view after
// Esc/abort in a single-select form. Returns nil if no form is needed.
func (m *model) reenterFormForView(view viewType) tea.Cmd {
	switch view {
	case manageCLIView:
		if len(m.targetCLIs) > 0 {
			m.selectedCLIID = m.targetCLIs[0].ID
		}
		return nil
	case switchManageBindingsView:
		m.switchRemoveMode = false
		bindings, _ := m.switchUseCases.ListBindingsForCLI(m.switchTargetCLIID)
		m.switchCLIBindings = bindings
		m.switchSelectedBindingIdx = 0
		return nil
	case switchTargetCLIView:
		m.switchTargetCLIID = 0
		m.setForm(NewSelectTargetCLIForm(m.targetCLIs, &m.switchTargetCLIID))
		m.form.WithHeight(10)
		return m.form.Init()
	case switchProviderView:
		providers, err := m.providerUseCases.List()
		if err != nil || len(providers) == 0 {
			return func() tea.Msg {
				return notificationMsg{message: "No providers available", isError: false, severity: "warn"}
			}
		}
		m.setForm(NewSelectProviderForm(providers, &m.switchProviderID))
		return m.form.Init()
	case restoreCLIView:
		return m.startRestoreCLIForm()
	default:
		return nil
	}
}

func (m *model) startRestoreCLIForm() tea.Cmd {
	m.selectedCLIID = 0
	m.restoreCLIName = ""
	m.restoreSelectedPath = ""
	m.restoreBackups = nil
	m.setForm(NewSelectCLIForm(m.targetCLIs, &m.selectedCLIID))
	return m.form.Init()
}

func (m *model) enterSwitchView(view viewType) tea.Cmd {
	switch view {
	case switchTargetCLIView:
		m.switchTargetCLIID = 0
		m.setForm(NewSelectTargetCLIForm(m.targetCLIs, &m.switchTargetCLIID))
		m.form.WithHeight(10)
		return m.form.Init()
	case switchProviderView:
		providers, err := m.providerUseCases.List()
		if err != nil || len(providers) == 0 {
			return func() tea.Msg {
				return notificationMsg{message: "No providers available", isError: false, severity: "warn"}
			}
		}
		m.setForm(NewSelectProviderForm(providers, &m.switchProviderID))
		return m.form.Init()
	}
	return nil
}

func (m *model) handleFormCompletion() (tea.Model, tea.Cmd) {
	switch m.currentView {
	case addProviderView:
		m.form = nil
		m.currentView = providerListView
		name := strings.TrimSpace(m.addProviderResult.Name)
		baseURL := strings.TrimSpace(m.addProviderResult.BaseURL)
		discoveryURL := strings.TrimSpace(m.addProviderResult.DiscoveryURL)
		apiKey := strings.TrimSpace(m.addProviderResult.APIKey)
		authToken := strings.TrimSpace(m.addProviderResult.AuthToken)
		dcw := parseContextWindowStr(m.addProviderResult.DefaultContextWindowStr)
		_, err := m.providerUseCases.Add(name, baseURL, discoveryURL, apiKey, authToken, dcw)
		if err != nil {
			return m, tea.Batch(func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Add failed: %s", err.Error()), isError: true}
			})
		}
		return m, tea.Batch(func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }, func() tea.Msg {
			return notificationMsg{message: fmt.Sprintf("Provider '%s' added", name), isError: false}
		})

	case editProviderView:
		m.form = nil
		m.currentView = providerListView
		baseURL := strings.TrimSpace(m.editProviderResult.BaseURL)
		discoveryURL := strings.TrimSpace(m.editProviderResult.DiscoveryURL)
		apiKey := strings.TrimSpace(m.editProviderResult.APIKey)
		authToken := strings.TrimSpace(m.editProviderResult.AuthToken)
		dcw := parseContextWindowStr(m.editProviderResult.DefaultContextWindowStr)
		if err := m.providerUseCases.Update(m.selectedProviderID, baseURL, discoveryURL, apiKey, authToken, dcw); err != nil {
			return m, tea.Batch(func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Update failed: %s", err.Error()), isError: true}
			})
		}
		return m, tea.Batch(func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }, func() tea.Msg {
			return notificationMsg{message: "Provider updated", isError: false}
		})

	case deleteProviderView:
		m.form = nil
		m.currentView = providerListView
		if m.deleteConfirm {
			if err := m.providerUseCases.Delete(m.selectedProviderID); err != nil {
				return m, tea.Batch(func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Delete failed: %s", err.Error()), isError: true}
				})
			}
			m.selectedProviderID = 0
		}
		return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }

	case switchTargetCLIView:
		m.form = nil

		// Check if this CLI uses env mapping (Claude Code, Codex).
		// Env-mapping CLIs always go to provider selection (replacing any existing).
		// pi/OpenCode/Copilot go to manage view when they already have bindings.
		usesEnv := false
		for _, c := range m.targetCLIs {
			if c.ID == m.switchTargetCLIID {
				usesEnv = c.Mutator == "claude-settings-json" || c.Mutator == "codex-config-toml"
				break
			}
		}

		if !usesEnv {
			bindings, _ := m.switchUseCases.ListBindingsForCLI(m.switchTargetCLIID)
			if len(bindings) > 0 {
				return m.enterManageBindingsView()
			}
		}

		// Go straight to provider selection
		m.switchInManageMode = false
		m.currentView = switchProviderView
		providers, err := m.providerUseCases.List()
		if err != nil || len(providers) == 0 {
			return m, func() tea.Msg {
				return notificationMsg{message: "No providers available", isError: false, severity: "warn"}
			}
		}
		m.setForm(NewSelectProviderForm(providers, &m.switchProviderID))
		return m, m.form.Init()

	case switchProviderView:
		m.form = nil

		// Remove mode: form complete → go to confirmation
		if m.switchRemoveMode {
			m.switchRemoveMode = false
			if m.switchProviderID == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: false, severity: "warn"}
				}
			}
			m.switchDeleteConfirm = false
			providerName := m.getProviderName(m.switchProviderID)
			m.currentView = deleteBindingConfirmView
			m.setForm(NewDeleteConfirmForm(providerName, &m.switchDeleteConfirm))
			return m, m.form.Init()
		}

		var targetCLI *domain.TargetCLI
		for _, c := range m.targetCLIs {
			if c.ID == m.switchTargetCLIID {
				targetCLI = &c
				break
			}
		}
		if targetCLI == nil {
			return m, func() tea.Msg {
				return notificationMsg{message: "Target CLI not found", isError: false, severity: "warn"}
			}
		}

		var envVars []string
		if err := json.Unmarshal([]byte(targetCLI.EnvVars), &envVars); err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: "Failed to parse env vars", isError: true}
			}
		}
		m.switchEnvVars = envVars

		models, err := m.switchUseCases.GetModelsForProvider(m.switchProviderID)
		if err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Failed to get models: %s", err.Error()), isError: true}
			}
		}
		if len(models) == 0 {
			// Back to provider selection — don't leave user stuck
			m.currentView = switchProviderView
			providers, listErr := m.providerUseCases.List()
			if listErr == nil && len(providers) > 0 {
				m.setForm(NewSelectProviderForm(providers, &m.switchProviderID))
				return m, tea.Batch(
					m.form.Init(),
					func() tea.Msg {
						return notificationMsg{message: "No models for this provider. Try retrying fetch.", isError: false, severity: "warn"}
					},
				)
			}
			return m, func() tea.Msg {
				return notificationMsg{message: "No models available for this provider", isError: false, severity: "warn"}
			}
		}
		m.switchProviderModels = models

		// Determine flow: env var mapping (Claude/Codex), single model (Copilot),
		// or multi-select (pi/OpenCode)
		m.switchUsesEnvMapping = targetCLI.Mutator == "claude-settings-json" || targetCLI.Mutator == "codex-config-toml"
		m.switchIsCopilot = targetCLI.Mutator == "copilot-shell-profile"

		if m.switchUsesEnvMapping {
			m.currentView = switchMapModelsView
			form, extractFn := NewMapModelsForm(envVars, models)
			m.switchExtractFn = extractFn
			m.setForm(form)
			return m, m.form.Init()
		}

		if m.switchIsCopilot {
			// Copilot uses a single model via COPILOT_MODEL
			m.switchSingleModelResult = SelectSingleModelResult{}
			m.currentView = switchSelectModelsView
			m.setForm(NewSelectSingleModelForm(models, &m.switchSingleModelResult))
			return m, m.form.Init()
		}

		// pi/OpenCode: multi-select, all pre-selected by default
		m.switchProviderModels = models
		m.switchRegisterResult = RegisterModelsResult{}
		m.currentView = switchSelectModelsView
		m.setForm(NewSelectModelsForm(models, &m.switchRegisterResult))
		return m, m.form.Init()

	case switchSelectModelsView:
		m.form = nil
		// Collect selected models from whichever form was active:
		// NewSelectModelsForm     → m.switchRegisterResult.RegisteredModels
		// NewEditModelsForm       → m.switchEditModelsResult.SelectedModels
		// NewSelectSingleModelForm → m.switchSingleModelResult.ModelName
		selected := m.switchRegisterResult.RegisteredModels
		if len(selected) == 0 {
			selected = m.switchEditModelsResult.SelectedModels
		}
		if len(selected) == 0 && m.switchSingleModelResult.ModelName != "" {
			selected = []string{m.switchSingleModelResult.ModelName}
		}
		custom := m.switchEditModelsResult.CustomModels
		if len(custom) > 0 {
			parts := strings.Split(custom, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					selected = append(selected, p)
				}
			}
		}
		if len(selected) == 0 {
			if m.switchIsCopilot {
				m.setForm(NewSelectSingleModelForm(m.switchProviderModels, &m.switchSingleModelResult))
			} else {
				m.setForm(NewSelectModelsForm(m.switchProviderModels, &m.switchRegisterResult))
			}
			m.currentView = switchSelectModelsView
			return m, m.form.Init()
		}
		m.switchRegisteredModels = selected
		mappings := map[string]string{"_registered": strings.Join(selected, ",")}
		if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, mappings); err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
			}
		}
		m.switchModelMetadataSummary = buildMetadataSummary(selected, m.switchProviderID, m.switchUseCases)
		// Save config content BEFORE Apply so the diff shows what changed.
		m.switchDryRunCurrentConfig = ""
		for _, c := range m.targetCLIs {
			if c.ID == m.switchTargetCLIID && c.ConfigPath != "" {
				data, err := os.ReadFile(c.ConfigPath)
				if err == nil && len(data) > 0 {
					m.switchDryRunCurrentConfig = string(data)
				}
				break
			}
		}
		if _, err := m.switchUseCases.Apply(m.switchTargetCLIID, 0); err != nil {
			log.Printf("config regenerate after model selection failed: %v", err)
		}
		if m.switchInManageMode {
			// After edit/add in manage mode → show dry run with preview
			return m.proceedToDryRun()
		}
		// Use SwitchToViewMsg to prevent Enter key bounce from the completed form
		return m, func() tea.Msg {
			return SwitchToViewMsg{View: switchAdvancedConfigView}
		}

	case switchMapModelsView:
		m.form = nil
		result := m.switchExtractFn()

		// Save mappings and derive registered models from them
		if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, result.Mappings); err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
			}
		}

		seen := make(map[string]bool)
		var registered []string
		for _, v := range result.Mappings {
			if v != "" && !seen[v] {
				seen[v] = true
				registered = append(registered, v)
			}
		}
		m.switchRegisteredModels = registered

		m.switchModelMetadataSummary = buildMetadataSummary(registered, m.switchProviderID, m.switchUseCases)
		return m, func() tea.Msg {
			return SwitchToViewMsg{View: switchAdvancedConfigView}
		}

	case manageCLIView:
		m.form = nil
		if m.selectedCLIID == 0 {
			m.currentView = dashboardView
			return m, nil
		}
		var cli *domain.TargetCLI
		for i := range m.targetCLIs {
			if m.targetCLIs[i].ID == m.selectedCLIID {
				cli = &m.targetCLIs[i]
				break
			}
		}
		if cli == nil {
			m.currentView = dashboardView
			return m, nil
		}
		m.currentView = editCLIPathView
		m.editCLIPathResult = EditCLIPathResult{}
		m.setForm(NewEditCLIPathForm(cli, &m.editCLIPathResult))
		return m, m.form.Init()

	case editCLIPathView:
		m.form = nil
		m.currentView = dashboardView
		r := m.editCLIPathResult
		if r.ConfigPath != "" {
			if err := m.switchUseCases.UpdateCLIConfig(r.CLIID, r.ConfigPath, r.MutatorConfig, r.BinaryPath); err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Update failed: %s", err.Error()), isError: true}
				}
			}
		}
		return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }

	case deleteBindingConfirmView:
		m.form = nil
		m.currentView = switchManageBindingsView
		if m.switchDeleteConfirm {
			log.Printf("removing binding: cli=%d provider=%d", m.switchTargetCLIID, m.switchProviderID)
			if err := m.switchUseCases.RemoveBinding(m.switchTargetCLIID, m.switchProviderID); err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Remove failed: %s", err.Error()), isError: true}
				}
			}
			log.Printf("regenerating config after remove")
			if _, err := m.switchUseCases.Apply(m.switchTargetCLIID, 0); err != nil {
				log.Printf("config regenerate failed: %v", err)
			}
			bindings, _ := m.switchUseCases.ListBindingsForCLI(m.switchTargetCLIID)
			m.switchCLIBindings = bindings
			if len(bindings) == 0 {
				if err := m.switchUseCases.ClearCLIConfig(m.switchTargetCLIID); err != nil {
					log.Printf("clear config failed: %v", err)
				}
				m.switchInManageMode = false
				m.switchCLIBindings = nil
				m.currentView = dashboardView
				return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
			}
			return m.enterManageBindingsView()
		}
		return m.enterManageBindingsView()

	case restoreCLIView:
		m.form = nil
		if m.selectedCLIID == 0 {
			m.currentView = dashboardView
			return m, nil
		}
		var cliName string
		for _, c := range m.targetCLIs {
			if c.ID == m.selectedCLIID {
				cliName = c.Name
				break
			}
		}
		if cliName == "" {
			m.currentView = dashboardView
			return m, func() tea.Msg {
				return notificationMsg{message: "CLI not found", isError: false, severity: "warn"}
			}
		}
		m.restoreCLIName = cliName
		backups, err := m.switchUseCases.BackupOptions(cliName)
		if err != nil {
			m.currentView = dashboardView
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Failed to list backups: %s", err.Error()), isError: true}
			}
		}
		if len(backups) == 0 {
			m.currentView = dashboardView
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("No backups for '%s'", cliName), isError: false}
			}
		}
		m.restoreBackups = backups
		m.currentView = restoreBackupView
		m.setForm(NewRestoreBackupForm(backups, &m.restoreSelectedPath))
		return m, m.form.Init()

	case restoreBackupView:
		m.form = nil
		m.currentView = dashboardView
		if m.restoreSelectedPath == "" {
			return m, nil
		}
		if err := m.switchUseCases.RestoreBackup(m.restoreCLIName, m.restoreSelectedPath); err != nil {
			return m, tea.Batch(
				func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} },
				func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Restore failed: %s", err.Error()), isError: true}
				},
			)
		}
		return m, tea.Batch(
			func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} },
			func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Backup restored for '%s'", m.restoreCLIName), isError: false}
			},
		)

	case launchCLIView:
		m.form = nil
		if m.launchCLIName == "" || m.launchProviderID == 0 {
			m.currentView = dashboardView
			return m, nil
		}
		return m.launchShowModels()

	case launchModelView:
		m.form = nil
		return m.launchAgent()
	}

	return m, nil
}

// launchRequestPath returns the path to the launch request file.
// Used to communicate CLI launch requests from TUI to main.go.
func launchRequestPath() string {
	configDir := os.Getenv("HOME")
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, ".config", "aimux", ".launch")
}

var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(aimuxT.Accent).
		Padding(0, 2)
)

func (m *model) renderDashboardSummary() string {
	activeProv := 0
	errorProv := 0
	for _, p := range m.providers {
		switch p.Status {
		case "active":
			activeProv++
		case "error":
			errorProv++
		}
	}

	activeCLIs := 0
	cliWithProviders := make(map[int64]bool)
	for _, am := range m.activeMultiplexes {
		if am.TargetCLIID != 0 {
			cliWithProviders[am.TargetCLIID] = true
		}
	}
	for _, cli := range m.targetCLIs {
		if cliWithProviders[cli.ID] {
			activeCLIs++
		}
	}
	inactiveCLIs := len(m.targetCLIs) - activeCLIs

	cyanNum := lipgloss.NewStyle().Foreground(aimuxT.Cyan).Bold(true)
	mutedNum := lipgloss.NewStyle().Foreground(aimuxT.TextMuted)

	provStr := cyanNum.Render(fmt.Sprintf("%d active", activeProv))
	if errorProv > 0 {
		provStr += lipgloss.NewStyle().Foreground(aimuxT.Red).Render(fmt.Sprintf(", %d error", errorProv))
	}

	cliStr := cyanNum.Render(fmt.Sprintf("%d active", activeCLIs)) +
		mutedNum.Render(fmt.Sprintf(", %d inactive", inactiveCLIs))

	labelStyle := lipgloss.NewStyle().Foreground(aimuxT.TextSecondary)
	cardBody := lipgloss.JoinVertical(lipgloss.Left,
		aimuxT.CardTitle.Render("Summary"),
		"",
		labelStyle.Render("Providers  ")+provStr,
		labelStyle.Render("CLIs       ")+cliStr,
	)
	// Responsive width: 50% of terminal, min 40, max 60
	w := m.width / 2
	if w < 40 {
		w = 40
	}
	if w > 60 {
		w = 60
	}
	return aimuxT.Card.Width(w).Render(cardBody)
}

// isCenteredView returns true if the current view should use centered layout mode.
// Most views use centered layout for consistent aesthetics. Only diff views use
// full-width fluid layout to accommodate side-by-side panels.
func (m *model) isCenteredView() bool {
	switch m.currentView {
	case switchConfirmationView:
		// Diff view needs full-width for side-by-side panels
		return false
	default:
		// All other views use centered layout
		return true
	}
}

func (m *model) View() string {
	m.syncSwitchStep()

	// Terminal too small: show contingency message instead of broken layout
	if m.width > 0 && m.height > 0 && (m.width < minTermWidth || m.height < minTermHeight) {
		return m.renderTooSmall()
	}

	// Help overlay takes over the full screen (A1).
	if m.showHelp {
		return m.renderHelpScreen()
	}

	layout := m.computeLayout()

	// No terminal dimensions yet — return raw content.
	if m.width == 0 {
		return m.renderBodyContent()
	}

	// Compose: header + body + footer.
	header := m.renderHeader()

	// A8: loading overlay — centered spinner + message, blocking content
	if m.loading {
		body := m.renderLoadingOverlay(layout)
		footer := m.renderFooter()
		return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	}

	bodyRaw := m.renderBodyContent()

	footer := m.renderFooter()

	// Hybrid Adaptive Layout: alternate between centered and fluid modes
	if m.isCenteredView() {
		// Centered mode: fixed-width content floating in viewport center.
		// Forms use Top vertical alignment so search/filter bar stays visible.
		vPos := lipgloss.Center
		if m.form != nil {
			vPos = lipgloss.Top
		}
		body := lipgloss.Place(m.width, layout.bodyH, lipgloss.Center, vPos, bodyRaw)
		return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	}

	// Fluid mode: full-width content stretching to terminal edges
	// Confirmation view with dry-run renders its own scrollable viewport.
	if m.currentView == switchConfirmationView && m.switchDryRun != nil {
		bodyRaw = m.renderConfirmationView()
		return lipgloss.JoinVertical(lipgloss.Left, header, bodyRaw, footer)
	}
	body := m.fillBody(bodyRaw, layout.bodyH)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderBodyContent returns the view-specific content WITHOUT chrome wrapping.
// The caller (View) wraps it with header/footer/fill.
func (m *model) renderBodyContent() string {
	// Forms
	if m.form != nil {
		content := m.form.View()
		if m.switchTotalSteps > 0 {
			content = m.renderSwitchStepper() + "\n" + content
		}
		return content
	}

	switch m.currentView {
	case dashboardView:
		summary := m.renderDashboardSummary()
		menu := RenderMenu(m.menuSelected, len(m.providers) > 0)

		// ASCII art logo — full AIMUX logotype
		logoBlock := RenderLogo(0, m.version)

		// Welcome message (first-run only) — shorter, more directive
		var welcome string
		if len(m.providers) == 0 {
			welcome = aimuxT.Help.Render("Welcome to aimux! Press 'a' in the provider list to add your first provider.")
		}

		var parts []string
		parts = append(parts, logoBlock, "")
		if summary != "" {
			parts = append(parts, summary, "")
		}
		if welcome != "" {
			parts = append(parts, welcome, "")
		}
		parts = append(parts, menu)

		content := lipgloss.JoinVertical(lipgloss.Left, parts...)
		return content

	case providerListView:
		return RenderProviderList(m.providers, m.selectedProviderID, m.width, m.allModels, m.activeMultiplexes)

	case switchManageBindingsView:
		content := m.renderManageBindings()
		return lipgloss.JoinVertical(lipgloss.Left, m.renderSwitchStepper(), content)

	case switchAdvancedConfigView:
		content := m.renderAdvancedConfigReview()
		return lipgloss.JoinVertical(lipgloss.Left, m.renderSwitchStepper(), content)

	case switchConfirmationView:
		return m.renderConfirmationView()

	default:
		return "Loading..."
	}
}

// renderConfirmationView renders the dry-run or post-apply confirmation.
// Dry-run mode uses a scrollable viewport with a compact diff.
// Post-apply mode shows a static success message.
func (m *model) renderConfirmationView() string {
	if m.switchDryRun != nil {
		var b strings.Builder
		b.WriteString(aimuxT.Title.Render("Dry-run Preview"))
		b.WriteString("\n")
		b.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("Target CLI:  %s  ·  Config:  %s", m.switchDryRun.CLIName, m.switchDryRun.ConfigPath)))
		b.WriteString("\n\n")
		b.WriteString(m.diffViewport.View())
		return b.String()
	}

	var sb strings.Builder
	sb.WriteString(aimuxT.Title.Render("Profile Activated"))
	sb.WriteString("\n\n")
	sb.WriteString(aimuxT.ItemDesc.Render("The config has been written and multiplex is active."))
	sb.WriteString("\n")
	if m.switchBackupPath != "" {
		sb.WriteString(aimuxT.ItemDesc.Render("Backup saved to:"))
		sb.WriteString("\n")
		sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("  %s", m.switchBackupPath)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

// renderHeader renders the 1-line header bar: app name + view breadcrumb
// with a clean native lipgloss.ThickBorder bottom edge. No side rails.
func (m *model) renderHeader() string {
	title := "aimux"
	switch m.currentView {
	case dashboardView:
		title += "  ·  Dashboard"
	case providerListView:
		title += "  ·  Manage Providers"
	case addProviderView:
		title += "  ·  Add Provider"
	case editProviderView:
		title += "  ·  Edit Provider"
	case deleteProviderView:
		title += "  ·  Delete Provider"
	case switchTargetCLIView, switchProviderView, switchMapModelsView,
		switchSelectModelsView, switchAdvancedConfigView,
		switchConfirmationView, switchManageBindingsView:
		title += "  ·  Switch"
		if m.switchStepLabel != "" {
			title += "  ·  " + m.switchStepLabel
		}
	case manageCLIView, editCLIPathView:
		title += "  ·  Manage CLIs"
	case restoreCLIView, restoreBackupView:
		title += "  ·  Restore"
	case launchCLIView:
		title += "  ·  Launch"
	case launchModelView:
		title += "  ·  Launch · Select Model"
	}

	// logo + breadcrumb, padded
	logo := lipgloss.NewStyle().Bold(true).Foreground(aimuxT.Accent).Render("◆ aimux ◆")
	crumb := lipgloss.NewStyle().Foreground(aimuxT.TextSecondary).Render(title[5:]) // strip "aimux"
	head := lipgloss.JoinHorizontal(lipgloss.Left, logo, "  ", crumb)

	// wrap in a top + bottom thick border, full width
	bar := lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 2).
		Border(lipgloss.ThickBorder(), false, true, false, false). // bottom only
		BorderForeground(aimuxT.Border).
		Render(head)

	return bar
}

// renderFooter renders the 1-line footer: help keybindings or notification.
// Uses the unified nav bar style (purple bold key, muted desc, " • " separator).
// No side rails — clean full-width bg.
func (m *model) renderToastBanner() string {
	msg := m.notification
	maxW := m.width - 6
	if maxW < 10 {
		maxW = 10
	}
	if lipgloss.Width(msg) > maxW {
		msg = truncateInline(msg, maxW)
	}

	var toastFg lipgloss.Color
	if strings.HasPrefix(msg, "\u2717") {
		toastFg = aimuxT.Red
	} else if strings.HasPrefix(msg, "⚠") {
		toastFg = aimuxT.Warn
	} else {
		toastFg = aimuxT.Green
	}

	toastStyle := lipgloss.NewStyle().Foreground(toastFg).Bold(true)
	rendered := toastStyle.Render("  " + msg)

	return lipgloss.NewStyle().
		Width(m.width).
		Border(lipgloss.ThickBorder(), true, true, false, false).
		BorderForeground(aimuxT.Border).
		Padding(0, 2).
		Render(padRightBg(rendered, m.width-4, aimuxT.BgBase))
}

func (m *model) renderFooter() string {
	if m.notification != "" {
		return m.renderToastBanner()
	}

	bar := lipgloss.NewStyle().
		Width(m.width).
		Border(lipgloss.ThickBorder(), true, false, false, false).
		BorderForeground(aimuxT.Border).
		Padding(0, 2).
		Render(m.renderNavBar())

	return bar
}

// renderTooSmall renders a contingency message when the terminal is too small
// for the TUI layout. Cleanly asks the user to resize instead of showing a
// collapsed or broken layout.
func (m *model) renderTooSmall() string {
	msg := fmt.Sprintf("Terminal too small (%dx%d). Resize to at least %dx%d.",
		m.width, m.height, minTermWidth, minTermHeight)
	prompt := "Stretch the window or press q/Ctrl+C to quit."

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(aimuxT.Warn).Render("⚠  Terminal Too Small"),
		"",
		lipgloss.NewStyle().Foreground(aimuxT.TextSecondary).Render(msg),
		lipgloss.NewStyle().Foreground(aimuxT.TextMuted).Render(prompt),
	)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderLoadingOverlay shows a centered spinner + contextual message.
// Covers the entire body area so the user sees progress feedback
// and accidental inputs are visually blocked (keyboard guard is
// in handleKeyMsg).
func (m *model) renderLoadingOverlay(layout uiLayout) string {
	spin := m.spinner.View()
	msg := m.loadingMsg
	if msg == "" {
		msg = "Working..."
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().
			Foreground(aimuxT.Accent).
			Render(fmt.Sprintf("%s %s", spin, msg)),
		"",
		lipgloss.NewStyle().
			Foreground(aimuxT.TextMuted).
			Render("(input disabled — please wait)"),
	)

	return lipgloss.Place(m.width, layout.bodyH,
		lipgloss.Center, lipgloss.Center,
		content,
	)
}

// renderNavBar renders a row of key+description pairs with the unified style:
// key = AccentPurple bold, description = TextMuted, separator = " • ".
// Renders nothing if pairs is empty.
func (m *model) renderNavBar() string {
	keys := m.contextualKeys()
	sep := aimuxT.FooterSep.Render(" • ")
	parts := make([]string, 0, len(keys)*2)
	for i, k := range keys {
		if i > 0 {
			parts = append(parts, sep)
		}
		parts = append(parts, aimuxT.FooterKey.Render(k.key))
		parts = append(parts, " ")
		parts = append(parts, aimuxT.FooterDesc.Render(k.desc))
	}
	return strings.Join(parts, "")
}

// navPair is one keybind display pair.
type navPair struct{ key, desc string }

// contextualKeys returns the keybinds relevant to the current view.
// Form views fall through to a minimal "nav/confirm/back" set.
func (m *model) contextualKeys() []navPair {
	if m.form != nil {
		return []navPair{
			{"tab", "next"},
			{"↑/↓", "move"},
			{"enter", "confirm"},
			{"esc", "back"},
		}
	}
	switch m.currentView {
	case dashboardView:
		return []navPair{
			{"↑/↓", "navigate"},
			{"enter", "select"},
			{"?", "help"},
			{"q", "quit"},
		}
	case providerListView:
		return []navPair{
			{"↑/↓", "navigate"},
			{"enter", "switch"},
			{"a", "add"},
			{"e", "edit"},
			{"d", "delete"},
			{"r", "retry"},
			{"t", "test"},
			{"esc", "back"},
		}
	case switchConfirmationView:
		return []navPair{
			{"↑/↓", "scroll"},
			{"pgup/pgdn", "scroll"},
			{"enter", "apply"},
			{"esc", "back"},
		}
	case switchManageBindingsView:
		return []navPair{
			{"↑/↓", "navigate"},
			{"enter", "edit"},
			{"d", "remove"},
			{"a", "add"},
			{"esc", "back"},
		}
	default:
		return []navPair{
			{"enter", "confirm"},
			{"esc", "back"},
			{"?", "help"},
		}
	}
}

// renderHelpScreen renders the full help overlay (A1: revives dead `?`).
func (m *model) renderHelpScreen() string {
	content := m.renderHelpOverlay()
	if m.width == 0 {
		return content
	}
	layout := m.computeLayout()
	header := m.renderHeader()
	body := m.fillBody(content, layout.bodyH)
	full := m.help.FullHelpView(menuKeys.FullHelp())
	footer := lipgloss.NewStyle().
		Width(m.width).
		Border(lipgloss.ThickBorder(), true, false, false, false).
		BorderForeground(aimuxT.Border).
		Padding(0, 2).
		Render(padRightBg(full, m.width-4, aimuxT.BgBase))
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *model) refreshData() tea.Msg {
	if m.providerUseCases == nil || m.switchUseCases == nil {
		return DashboardRefreshMsg{}
	}

	providers, err := m.providerUseCases.List()
	if err != nil {
		return DashboardRefreshMsg{}
	}
	m.providers = providers

	active, err := m.switchUseCases.ListActiveMultiplexes()
	if err != nil {
		return DashboardRefreshMsg{}
	}
	m.activeMultiplexes = active

	clis, err := m.switchUseCases.ListTargetCLIs()
	if err == nil {
		m.targetCLIs = clis
	}

	allModels, err := m.switchUseCases.ListAllModels()
	if err == nil {
		m.allModels = allModels
	}

	return DashboardRefreshMsg{}
}

func (m *model) previousView() viewType {
	switch m.currentView {
	case addProviderView, deleteProviderView, editProviderView:
		return providerListView
	case switchAdvancedConfigView:
		if m.switchInManageMode {
			return switchManageBindingsView
		}
		if m.switchUsesEnvMapping {
			return switchMapModelsView
		}
		return switchSelectModelsView
	case switchMapModelsView:
		if m.switchInManageMode {
			return switchManageBindingsView
		}
		return switchProviderView
	case switchSelectModelsView:
		if m.switchInManageMode {
			return switchManageBindingsView
		}
		return switchProviderView
	case switchRegisterModelsView:
		return switchMapModelsView
	case switchProviderView:
		if m.switchRemoveMode {
			return switchManageBindingsView
		}
		if m.switchInManageMode {
			return switchManageBindingsView
		}
		return switchTargetCLIView
	case manageCLIView:
		return dashboardView
	case deleteBindingConfirmView:
		return switchManageBindingsView
	case switchManageBindingsView:
		return dashboardView
	case switchTargetCLIView, switchConfirmationView:
		return dashboardView
	case restoreCLIView:
		return dashboardView
	case restoreBackupView:
		return restoreCLIView
	case launchCLIView, launchModelView:
		return dashboardView
	default:
		return dashboardView
	}
}

func (m *model) getProviderName(id int64) string {
	for _, p := range m.providers {
		if p.ID == id {
			return p.Name
		}
	}
	return "Unknown"
}

func (m *model) getProvider(id int64) *domain.Provider {
	for _, p := range m.providers {
		if p.ID == id {
			return &p
		}
	}
	return nil
}

func (m *model) nextProviderID(current int64) int64 {
	found := false
	for _, p := range m.providers {
		if found {
			return p.ID
		}
		if p.ID == current {
			found = true
		}
	}
	if len(m.providers) > 0 {
		return m.providers[0].ID
	}
	return 0
}

func (m *model) prevProviderID(current int64) int64 {
	var prev int64
	for _, p := range m.providers {
		if p.ID == current {
			if prev != 0 {
				return prev
			}
			return current
		}
		prev = p.ID
	}
	if len(m.providers) > 0 {
		return m.providers[len(m.providers)-1].ID
	}
	return 0
}

// skipDisabledMenuItems adjusts menuSelected to skip disabled menu items.
// "Bind CLI" disabled when no providers. "Launch" always enabled if providers exist.
func (m *model) skipDisabledMenuItems() {
	hasProviders := len(m.providers) > 0

	if m.menuSelected == menuItemSwitch && !hasProviders {
		if m.menuSelected < menuItemCount-1 {
			m.menuSelected = menuItemLaunch
		}
	}
	if m.menuSelected == menuItemLaunch && !hasProviders {
		if m.menuSelected < menuItemCount-1 {
			m.menuSelected = menuItemManageProviders
		}
	}
	// Clamp
	if m.menuSelected < 0 {
		m.menuSelected = 0
	}
	if m.menuSelected >= menuItemCount {
		m.menuSelected = menuItemCount - 1
	}
}

func (m *model) nextCLIID(current int64) int64 {
	found := false
	for _, c := range m.targetCLIs {
		if found {
			return c.ID
		}
		if c.ID == current {
			found = true
		}
	}
	if len(m.targetCLIs) > 0 {
		return m.targetCLIs[0].ID
	}
	return 0
}

func (m *model) prevCLIID(current int64) int64 {
	var prev int64
	for _, c := range m.targetCLIs {
		if c.ID == current {
			if prev != 0 {
				return prev
			}
			return current
		}
		prev = c.ID
	}
	if len(m.targetCLIs) > 0 {
		return m.targetCLIs[len(m.targetCLIs)-1].ID
	}
	return 0
}

// parseRegisteredModels extracts model list from a binding's model_mappings JSON.
func parseRegisteredModels(mappingsJSON string) []string {
	var mps map[string]string
	if err := json.Unmarshal([]byte(mappingsJSON), &mps); err != nil {
		return nil
	}
	if reg, ok := mps["_registered"]; ok && reg != "" {
		return strings.Split(reg, ",")
	}
	// Fallback: return unique mapping values (skip _ keys)
	seen := make(map[string]bool)
	var models []string
	for k, v := range mps {
		if strings.HasPrefix(k, "_") || v == "" || seen[v] {
			continue
		}
		seen[v] = true
		models = append(models, v)
	}
	return models
}

// resetSwitchState clears all switch-related fields to avoid stale state
// when returning to the dashboard or starting a new switch flow.
func (m *model) resetSwitchState() {
	m.switchTargetCLIID = 0
	m.switchProviderID = 0
	m.switchEnvVars = nil
	m.switchExtractFn = nil
	m.switchRegisteredModels = nil
	m.switchRegisterResult = RegisterModelsResult{}
	m.switchEditModelsResult = EditModelsResult{}
	m.switchSingleModelResult = SelectSingleModelResult{}
	m.switchIsCopilot = false
	m.switchBackupPath = ""
	m.switchDryRun = nil
	m.switchModelMetadataSummary = nil
	m.switchUsesEnvMapping = false
	m.switchInManageMode = false
	m.switchCLIBindings = nil
	m.switchRemoveMode = false
	m.switchDeleteConfirm = false
	m.switchSelectedBindingIdx = 0
	m.switchProviderModels = nil
	m.switchStep = 0
	m.switchTotalSteps = 0
	m.switchStepLabel = ""
	m.diffContent = ""
	m.diffViewport.SetContent("")
}

// editSelectedBinding opens the model selection form for the selected binding.
// Used by both Enter and 'e' in the manage bindings view.
func (m *model) editSelectedBinding() (tea.Model, tea.Cmd) {
	if len(m.switchCLIBindings) == 0 {
		return m, func() tea.Msg {
			return notificationMsg{message: "No provider selected", isError: false, severity: "warn"}
		}
	}
	editIdx := m.switchSelectedBindingIdx
	if editIdx < 0 || editIdx >= len(m.switchCLIBindings) {
		return m, func() tea.Msg {
			return notificationMsg{message: "No provider selected", isError: false, severity: "warn"}
		}
	}
	bp := m.switchCLIBindings[editIdx]
	m.switchProviderID = bp.ProviderID
	currentModels := parseRegisteredModels(bp.ModelMappings)
	models, err := m.switchUseCases.GetModelsForProvider(bp.ProviderID)
	if err != nil {
		return m, func() tea.Msg {
			return notificationMsg{message: fmt.Sprintf("Failed to get models: %s", err.Error()), isError: true}
		}
	}

	// Determine CLI type to pick the right form
	for _, c := range m.targetCLIs {
		if c.ID == m.switchTargetCLIID && c.Mutator == "copilot-shell-profile" {
			// Copilot: single model select
			defaultModel := ""
			if len(currentModels) > 0 {
				defaultModel = currentModels[0]
			}
			m.switchSingleModelResult = SelectSingleModelResult{ModelName: defaultModel}
			m.setForm(NewSelectSingleModelForm(models, &m.switchSingleModelResult))
			m.currentView = switchSelectModelsView
			return m, m.form.Init()
		}
	}

	m.switchEditModelsResult = EditModelsResult{}
	m.setForm(NewEditModelsForm(models, currentModels, &m.switchEditModelsResult))
	m.currentView = switchSelectModelsView
	return m, m.form.Init()
}

func (m *model) proceedToDryRun() (tea.Model, tea.Cmd) {
	// In manage mode, apply all providers (0 = all).
	// In single mode, apply only the current provider.
	pid := m.switchProviderID
	if m.switchInManageMode {
		pid = 0
	}
	dryRun, err := m.switchUseCases.DryRun(m.switchTargetCLIID, pid)
	if err != nil {
		return m, func() tea.Msg {
			return notificationMsg{message: fmt.Sprintf("Dry-run failed: %s", err.Error()), isError: true}
		}
	}
	m.switchDryRun = dryRun
	m.switchBackupPath = ""

	// Read current config for diff preview if not already saved (e.g. by model selection handler).
	if m.switchDryRunCurrentConfig == "" && dryRun != nil && dryRun.ConfigPath != "" {
		data, err := os.ReadFile(dryRun.ConfigPath)
		if err == nil && len(data) > 0 {
			m.switchDryRunCurrentConfig = string(data)
		}
	}

	m.diffContent = generateCompactDiff(m.switchDryRunCurrentConfig, dryRun.EnvVars)
	m.diffViewport.SetContent(m.diffContent)
	// Size viewport for the current terminal dimensions if known.
	if m.width > 0 && m.height > 0 {
		bodyH := m.height - 2
		vpHeight := bodyH - 3
		if vpHeight < 5 {
			vpHeight = 5
		}
		m.diffViewport.Width = m.width - 6
		m.diffViewport.Height = vpHeight
	}

	m.currentView = switchConfirmationView
	return m, nil
}

// enterManageBindingsView refreshes the bound providers list for the selected
// CLI and transitions to the manage bindings view.
func (m *model) enterManageBindingsView() (tea.Model, tea.Cmd) {
	bindings, _ := m.switchUseCases.ListBindingsForCLI(m.switchTargetCLIID)
	m.switchCLIBindings = bindings
	m.switchRemoveMode = false
	m.switchInManageMode = len(bindings) > 0
	m.switchSelectedBindingIdx = 0
	m.currentView = switchManageBindingsView
	return m, nil
}

// renderManageBindings renders the provider bindings list for a CLI.
// Each binding is displayed as a bordered card with status and model mappings.
func (m *model) renderManageBindings() string {
	cliName := ""
	for _, tc := range m.targetCLIs {
		if tc.ID == m.switchTargetCLIID {
			cliName = tc.Name
			break
		}
	}

	title := aimuxT.CardTitle.Render("Provider Bindings · " + cliName)

	if len(m.switchCLIBindings) == 0 {
		empty := aimuxT.Muted.Render("No providers bound yet.")
		cardBody := lipgloss.JoinVertical(lipgloss.Left, title, "", empty)
		return aimuxT.Card.Copy().Width(m.width - 8).Render(cardBody)
	}

	var cards []string
	for i, bp := range m.switchCLIBindings {
		selected := i == m.switchSelectedBindingIdx

		// Build the model mappings string
		mappings := ""
		var mps map[string]string
		if err := json.Unmarshal([]byte(bp.ModelMappings), &mps); err == nil {
			models := []string{}
			for k, v := range mps {
				if k != "_registered" && v != "" {
					models = append(models, v)
				}
			}
			if len(models) > 0 {
				mappings = strings.Join(models, ", ")
			} else if reg, ok := mps["_registered"]; ok {
				mappings = reg
			}
			// ANSI-safe truncation: bounds the display width to 60 cells
			const mappingMaxW = 60
			mappings = truncateText(mappings, mappingMaxW)
		}

		// Style based on selection
		nameStyle := aimuxT.ItemTitle
		detailStyle := aimuxT.ItemDesc

		nameLine := nameStyle.Render(bp.ProviderName)
		if selected {
			nameLine = lipgloss.NewStyle().
				Foreground(aimuxT.Accent).
				Render("▸ ") + nameLine
		}

		cardContent := nameLine
		if mappings != "" {
			cardContent += "\n" + detailStyle.Render("Models: "+mappings)
		}
		cardContent += "\n" + detailStyle.Render("Activated: "+bp.ActivatedAt)

		cardStyle := aimuxT.Card.Copy().Width(60)
		if selected {
			cardStyle = cardStyle.BorderForeground(aimuxT.Accent)
		}
		cards = append(cards, cardStyle.Render(cardContent))
	}

	// Always vertical list for clear selection visibility
	grid := strings.Join(cards, "\n")

	return lipgloss.JoinVertical(lipgloss.Left, title, "", grid)
}

func (m *model) renderAdvancedConfigReview() string {
	title := aimuxT.CardTitle.Render("Advanced Model Configuration")
	subtitle := lipgloss.NewStyle().
		Foreground(aimuxT.TextSecondary).
		Render(fmt.Sprintf("Registered models · %d", len(m.switchRegisteredModels)))

	var content string
	if len(m.switchModelMetadataSummary) == 0 {
		content = aimuxT.Muted.Render("No advanced metadata available — default settings will be used.")
	} else {
		content = strings.Join(m.switchModelMetadataSummary, "\n")
	}

	cardBody := lipgloss.JoinVertical(lipgloss.Left,
		title,
		subtitle,
		"",
		content,
	)

	return aimuxT.Card.Copy().Width(m.width - 8).Render(cardBody)
}

// buildMetadataSummary builds display lines for model metadata.
func buildMetadataSummary(registeredModels []string, providerID int64, useCases *application.SwitchUseCases) []string {
	if len(registeredModels) == 0 {
		return nil
	}

	models, err := useCases.GetModelsForProvider(providerID)
	if err != nil {
		return []string{fmt.Sprintf("  Warning: could not load model metadata: %s", err)}
	}

	var lines []string
	for _, name := range registeredModels {
		// Find model metadata
		for _, m := range models {
			if m.ModelName == name && len(m.Metadata) > 0 {
				line := fmt.Sprintf("  • %s", name)
				if cw, ok := m.Metadata[domain.MetaContextWindow]; ok {
					line += fmt.Sprintf(" | ctx: %v", cw)
				}
				if mt, ok := m.Metadata[domain.MetaMaxTokens]; ok {
					line += fmt.Sprintf(" | max: %v", mt)
				}
				if r, ok := m.Metadata[domain.MetaReasoning]; ok && r == true {
					line += " | reasoning"
				}
				if cost, ok := m.Metadata[domain.MetaCost]; ok {
					line += fmt.Sprintf(" | cost: %s", config.FormatCost(cost))
				}
				if suffix, ok := m.Metadata[domain.MetaContextSuffix]; ok {
					line += fmt.Sprintf(" | suffix: %v", suffix)
				}
				lines = append(lines, line)
				break
			}
		}
	}

	if len(lines) == 0 {
		lines = append(lines, "  No metadata available for registered models.")
	}

	return lines
}

// parseContextWindowStr parses a user-supplied context window string (e.g. "1000000") into int64.
// Empty string = 0 (not set). Non-numeric = 0.
func parseContextWindowStr(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	if v < 0 {
		return 0
	}
	return v
}

// syncSwitchStep sets the stepper state based on the current switch view.
func (m *model) syncSwitchStep() {
	switch m.currentView {
	case switchTargetCLIView:
		m.switchStep = switchStepCLI
		m.switchTotalSteps = 5
		m.switchStepLabel = switchStepLabels[switchStepCLI]
	case switchProviderView:
		m.switchStep = switchStepProvider
		m.switchTotalSteps = 5
		m.switchStepLabel = switchStepLabels[switchStepProvider]
	case switchMapModelsView:
		m.switchStep = switchStepMapModels
		m.switchTotalSteps = 5
		m.switchStepLabel = switchStepLabels[switchStepMapModels]
	case switchSelectModelsView:
		m.switchStep = switchStepMapModels
		m.switchTotalSteps = 5
		m.switchStepLabel = "Select Models"
	case switchAdvancedConfigView:
		m.switchStep = switchStepReviewConfig
		m.switchTotalSteps = 5
		m.switchStepLabel = switchStepLabels[switchStepReviewConfig]
	case switchConfirmationView:
		m.switchStep = switchStepConfirm
		m.switchTotalSteps = 5
		m.switchStepLabel = switchStepLabels[switchStepConfirm]
	case switchManageBindingsView:
		m.switchStep = 0
		m.switchTotalSteps = 0
		m.switchStepLabel = ""
	default:
		// Reset for non-switch views
		m.switchStep = 0
		m.switchTotalSteps = 0
		m.switchStepLabel = ""
	}
}

// renderSwitchStepper renders a compact step indicator for the Switch flow.
// Delegates to the dedicated RenderStepIndicator component in stepper.go.
func (m *model) renderSwitchStepper() string {
	return RenderStepIndicator(m.switchStep, m.switchTotalSteps, m.switchStepLabel)
}

func (m *model) renderHelpOverlay() string {
	var b strings.Builder
	b.WriteString(aimuxT.Title.Render("Help & Shortcuts"))
	b.WriteString("\n\n")

	shortcuts := []struct {
		keys string
		desc string
	}{
		{"↑/↓  k/j", "Navigate list"},
		{"Enter", "Select / Confirm"},
		{"Esc", "Go back / Abort"},
		{"a", "Add provider"},
		{"d", "Delete / Remove"},
		{"e", "Edit models"},
		{"r", "Retry model fetch"},
		{"t", "Test connectivity"},
		{"?", "Toggle this help"},
		{"q  Ctrl+C", "Quit"},
	}

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(aimuxT.Accent).Padding(0, 1)
	descStyle := lipgloss.NewStyle().Foreground(aimuxT.TextSecondary).Padding(0, 1)

	for _, s := range shortcuts {
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Left,
			keyStyle.Render(fmt.Sprintf(" %-14s", s.keys)),
			descStyle.Render(s.desc),
		))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(aimuxT.Help.Render("Press any key to close"))

	return b.String()
}

// generateCompactDiff builds a scrollable diff view comparing the current config
// against the new env vars/model mappings.
//
// For env-var CLIs (Claude Code, Codex): keys like ANTHROPIC_API_KEY are matched
// in the config file by substring, showing each matched line as removed (- red)
// and the new assignment as added (+ green) with 3 context lines.
//
// For model-based CLIs (pi, OpenCode, Copilot): the _registered key lists models.
// These are rendered as human-readable additions after a "▌ model changes" header.
//
// Large identical sections are collapsed into a dimmed placeholder line.
func generateCompactDiff(currentConfig string, envVars map[string]string) string {
	if len(envVars) == 0 {
		return aimuxT.Muted.Render("  No changes to display.")
	}

	lines := strings.Split(strings.TrimRight(currentConfig, "\n"), "\n")
	hasConfig := len(lines) > 0 && !(len(lines) == 1 && lines[0] == "")

	// Separate meta keys (starting with _) from regular env var keys.
	var metaKeys []string
	var metaVals []string
	var regularKeys []string
	regularVals := make(map[string]string)
	for k, v := range envVars {
		if strings.HasPrefix(k, "_") {
			metaKeys = append(metaKeys, k)
			metaVals = append(metaVals, v)
		} else {
			regularKeys = append(regularKeys, k)
			regularVals[k] = v
		}
	}

	type change struct {
		idx     int    // line index in config (-1 = pure addition)
		oldLine string // matched config line (empty for pure addition)
		newLine string // formatted as "KEY = value"
		meta    bool   // true if this is a meta-key addition (shown after config preview)
	}

	var matchedChanges []change // changes found in config (regular keys that matched)
	var unmatChanges []change   // regular keys NOT found in config
	var metaChanges []change    // meta-key additions (shown last)

	// Match regular env var keys against config lines
	for _, key := range regularKeys {
		newLine := fmt.Sprintf("%s = %s", key, regularVals[key])
		found := false
		if hasConfig {
			for i, line := range lines {
				if strings.Contains(line, key) {
					matchedChanges = append(matchedChanges, change{idx: i, oldLine: line, newLine: newLine})
					found = true
					break
				}
			}
		}
		if !found {
			unmatChanges = append(unmatChanges, change{idx: -1, oldLine: "", newLine: newLine})
		}
	}

	// Meta keys (_registered etc.) go last, after config preview
	for i, key := range metaKeys {
		if key == "_registered" {
			models := strings.Split(metaVals[i], ",")
			for _, m := range models {
				m = strings.TrimSpace(m)
				if m != "" {
					metaChanges = append(metaChanges, change{idx: -1, oldLine: "", newLine: fmt.Sprintf("register model:  %s", m), meta: true})
				}
			}
		} else {
			metaChanges = append(metaChanges, change{idx: -1, oldLine: "", newLine: fmt.Sprintf("%s = %s", key, metaVals[i]), meta: true})
		}
	}

	hasAnyChanges := len(matchedChanges) > 0 || len(unmatChanges) > 0 || len(metaChanges) > 0
	if !hasAnyChanges {
		return aimuxT.Muted.Render("  No changes detected.")
	}

	// Sort matched changes by line index
	sort.Slice(matchedChanges, func(i, j int) bool {
		return matchedChanges[i].idx < matchedChanges[j].idx
	})

	ctxBefore := 3
	ctxAfter := 3
	var sb strings.Builder

	// ── Section 1: matched env-var changes in config ──
	if len(matchedChanges) > 0 && hasConfig {
		sb.WriteString(aimuxT.DiffHeader.Render("▌ current config"))
		sb.WriteString("\n")

		lastIdx := -2
		for _, ch := range matchedChanges {
			// Context before the change
			gap := ch.idx - lastIdx - 1
			if lastIdx >= 0 && gap > (ctxBefore+ctxAfter+1) {
				sb.WriteString(aimuxT.DiffMuted.Render(fmt.Sprintf("  ... (%d líneas idénticas ocultas) ...", gap)))
				sb.WriteString("\n")
				start := ch.idx - ctxBefore
				if start < 0 {
					start = 0
				}
				for j := start; j < ch.idx; j++ {
					sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
					sb.WriteString("\n")
				}
			} else if lastIdx >= 0 && gap > 0 {
				for j := lastIdx + 1; j < ch.idx; j++ {
					sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
					sb.WriteString("\n")
				}
			} else if lastIdx < 0 && ch.idx > ctxBefore {
				sb.WriteString(aimuxT.DiffMuted.Render(fmt.Sprintf("  ... (%d líneas idénticas ocultas) ...", ch.idx-ctxBefore)))
				sb.WriteString("\n")
				start := ch.idx - ctxBefore
				for j := start; j < ch.idx; j++ {
					sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
					sb.WriteString("\n")
				}
			} else if lastIdx < 0 && ch.idx > 0 {
				for j := 0; j < ch.idx; j++ {
					sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
					sb.WriteString("\n")
				}
			}

			// The change itself
			if ch.oldLine != "" {
				sb.WriteString(aimuxT.DiffRemoved.Render(fmt.Sprintf("- %s", ch.oldLine)))
				sb.WriteString("\n")
			}
			sb.WriteString(aimuxT.DiffAdded.Render(fmt.Sprintf("+ %s", ch.newLine)))
			sb.WriteString("\n")

			// Context after the change
			end := ch.idx + ctxAfter
			if end >= len(lines) {
				end = len(lines) - 1
			}
			for j := ch.idx + 1; j <= end; j++ {
				sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
				sb.WriteString("\n")
			}
			lastIdx = end
		}

		// Trailing collapsed content after last change
		if lastIdx >= 0 && lastIdx < len(lines)-1 {
			trailGap := len(lines) - 1 - lastIdx
			if trailGap > ctxAfter+1 {
				sb.WriteString(aimuxT.DiffMuted.Render(fmt.Sprintf("  ... (%d líneas idénticas ocultas) ...", trailGap)))
				sb.WriteString("\n")
			} else {
				for j := lastIdx + 1; j < len(lines); j++ {
					sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", lines[j])))
					sb.WriteString("\n")
				}
			}
		}
	}

	// ── Section 2: config preview when no regular keys matched ──
	// This happens for model-based CLIs where only meta keys exist.
	// Every line is shown — the viewport handles scrolling for large files.
	noMatchedChanges := len(matchedChanges) == 0 && len(unmatChanges) == 0
	if hasConfig && noMatchedChanges && len(metaChanges) > 0 {
		sb.WriteString(aimuxT.DiffHeader.Render("▌ current config"))
		sb.WriteString("\n")
		for _, line := range lines {
			sb.WriteString(aimuxT.DiffContext.Render(fmt.Sprintf("  %s", line)))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// ── Section 3: unmatched regular keys (env vars not found in config) ──
	if len(unmatChanges) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		for _, ch := range unmatChanges {
			sb.WriteString(aimuxT.DiffAdded.Render(fmt.Sprintf("+ %s", ch.newLine)))
			sb.WriteString("\n")
		}
	}

	// ── Section 4: meta-key additions (model registrations, always last) ──
	if len(metaChanges) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(aimuxT.DiffHeader.Render("▌ model changes"))
		sb.WriteString("\n")
		for _, ch := range metaChanges {
			sb.WriteString(aimuxT.DiffAdded.Render(fmt.Sprintf("+ %s", ch.newLine)))
			sb.WriteString("\n")
		}
	}

	result := sb.String()
	if result == "" {
		return aimuxT.DiffMuted.Render("  (no visible changes)")
	}
	return result
}
