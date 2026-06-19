package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
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
)

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
		message string
		isError bool
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
}

type model struct {
	providerUseCases *application.ProviderUseCases
	switchUseCases   *application.SwitchUseCases

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

	showHelp    bool   // when true, render help overlay instead of current view
	exitConfirm bool   // true when waiting for second q to confirm quit
	lastUndoCLI string // CLI name for quick undo (Z key), empty = nothing to undo

	spinner    spinner.Model
	loading    bool   // true when an async operation is in progress
	loadingMsg string // contextual message shown with spinner

	notification      string
	notificationIsMsg bool

	restoreCLIName      string
	restoreSelectedPath string
	restoreBackups      []application.BackupOption

	// ponytail: updateInfo removed — unused. Re-add when update notification is implemented.
}

func NewModel(providerUseCases *application.ProviderUseCases, switchUseCases *application.SwitchUseCases) *model {
	s := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	s.Style = lipgloss.NewStyle().Foreground(aimuxT.AccentAlt)
	return &model{
		providerUseCases: providerUseCases,
		switchUseCases:   switchUseCases,
		currentView:      dashboardView,
		menuSelected:     menuItemManageProviders,
		spinner:          s,
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

func (m *model) Init() tea.Cmd {
	return tea.Batch(
		m.refreshData,
		m.loadingCmd("Loading..."),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	if m.form != nil {
		// Intercept Esc for single-select forms before huh processes it.
		// huh treats Esc as "previous field" and silently swallows it
		// when no previous field exists (single-group forms).
		switch k := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(k, menuKeys.Esc) && m.isSingleSelectForm() {
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
		m.form = f

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
		return m, nil

	case SwitchToViewMsg:
		m.currentView = msg.View
		return m, m.enterView(msg.View)

	case notificationMsg:
		m.notification = msg.message
		m.notificationIsMsg = !msg.isError
		if msg.isError {
			log.Printf("TUI error: %s", msg.message)
		}
		// Error notifications persist until dismissed (Esc). Success auto-dismiss.
		if !msg.isError {
			return m, tea.Tick(4*time.Second, func(_ time.Time) tea.Msg {
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
	// Help overlay: any key dismisses
	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Toggle help from any non-form view
	if key.Matches(msg, menuKeys.Help) {
		m.showHelp = true
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
		m.exitConfirm = false
	}

	// Exit confirm: any non-q key resets
	if m.exitConfirm {
		if key.Matches(msg, menuKeys.Quit) {
			return m, tea.Quit
		}
		m.exitConfirm = false
		m.notification = ""
		// Fall through to normal handling
	}

	switch m.currentView {
	case dashboardView:
		switch {
		case key.Matches(msg, menuKeys.Up):
			if m.menuSelected > 0 {
				m.menuSelected--
				if m.menuSelected == menuItemSwitch && len(m.providers) == 0 && m.menuSelected > 0 {
					m.menuSelected--
				}
			}
		case key.Matches(msg, menuKeys.Down):
			if m.menuSelected < menuItemCount-1 {
				m.menuSelected++
				if m.menuSelected == menuItemSwitch && len(m.providers) == 0 {
					m.menuSelected++
				}
			}
		case key.Matches(msg, menuKeys.Enter):
			return m.handleMenuSelection()
		case key.Matches(msg, menuKeys.Quit):
			if m.exitConfirm {
				return m, tea.Quit
			}
			m.exitConfirm = true
			m.notification = "Press q again to quit, or any other key to cancel"
			m.notificationIsMsg = false
			return m, nil
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
			m.form = NewAddProviderForm(&m.addProviderResult)
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Delete):
			if m.selectedProviderID > 0 {
				m.currentView = deleteProviderView
				m.deleteConfirm = false
				providerName := m.getProviderName(m.selectedProviderID)
				m.form = NewDeleteConfirmForm(providerName, &m.deleteConfirm)
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
					m.form = NewEditProviderForm(*provider, &m.editProviderResult)
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
			return m.proceedToDryRun()
		case key.Matches(msg, menuKeys.Add):
			m.currentView = switchProviderView
			providers, _ := m.providerUseCases.List()
			if len(providers) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers available", isError: true}
				}
			}
			m.form = NewSelectProviderForm(providers, &m.switchProviderID)
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Edit):
			if len(m.switchCLIBindings) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: true}
				}
			}
			editIdx := m.switchSelectedBindingIdx
			if editIdx < 0 || editIdx >= len(m.switchCLIBindings) {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: true}
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
			m.switchEditModelsResult = EditModelsResult{}
			m.form = NewEditModelsForm(models, currentModels, &m.switchEditModelsResult)
			m.currentView = switchSelectModelsView
			return m, m.form.Init()
		case key.Matches(msg, menuKeys.Delete):
			if len(m.switchCLIBindings) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers to remove", isError: true}
				}
			}
			idx := m.switchSelectedBindingIdx
			if idx < 0 || idx >= len(m.switchCLIBindings) {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: true}
				}
			}
			b := m.switchCLIBindings[idx]
			m.switchProviderID = b.ProviderID
			m.switchDeleteConfirm = false
			providerName := b.ProviderName
			m.currentView = deleteBindingConfirmView
			m.form = NewDeleteConfirmForm(providerName, &m.switchDeleteConfirm)
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
				m.form = form
				return m, m.form.Init()
			}
			m.currentView = switchProviderView
			providers, _ := m.providerUseCases.List()
			m.form = NewSelectProviderForm(providers, &m.switchProviderID)
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
		case key.Matches(msg, menuKeys.Esc):
			m.switchDryRun = nil
			m.switchBackupPath = ""
			m.resetSwitchState()
			m.currentView = dashboardView
			return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
		case key.Matches(msg, menuKeys.Enter):
			if m.switchDryRun == nil {
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
		m.form = NewSelectCLIForm(clis, &m.selectedCLIID)
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
	m.form = NewSelectTargetCLIForm(clis, &m.switchTargetCLIID)
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
		m.form = NewSelectTargetCLIForm(m.targetCLIs, &m.switchTargetCLIID)
		m.form.WithHeight(10)
		return m.form.Init()
	case switchProviderView:
		providers, err := m.providerUseCases.List()
		if err != nil || len(providers) == 0 {
			return func() tea.Msg {
				return notificationMsg{message: "No providers available", isError: true}
			}
		}
		m.form = NewSelectProviderForm(providers, &m.switchProviderID)
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
	m.form = NewSelectCLIForm(m.targetCLIs, &m.selectedCLIID)
	return m.form.Init()
}

func (m *model) enterSwitchView(view viewType) tea.Cmd {
	switch view {
	case switchTargetCLIView:
		m.switchTargetCLIID = 0
		m.form = NewSelectTargetCLIForm(m.targetCLIs, &m.switchTargetCLIID)
		m.form.WithHeight(10)
		return m.form.Init()
	case switchProviderView:
		providers, err := m.providerUseCases.List()
		if err != nil || len(providers) == 0 {
			return func() tea.Msg {
				return notificationMsg{message: "No providers available", isError: true}
			}
		}
		m.form = NewSelectProviderForm(providers, &m.switchProviderID)
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
		apiType := domain.ApiType(m.addProviderResult.ApiType)

		_, err := m.providerUseCases.Add(name, baseURL, discoveryURL, apiKey, authToken, apiType)
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
		apiType := domain.ApiType(m.editProviderResult.ApiType)

		if err := m.providerUseCases.Update(m.selectedProviderID, baseURL, discoveryURL, apiKey, authToken, apiType); err != nil {
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
				return notificationMsg{message: "No providers available", isError: true}
			}
		}
		m.form = NewSelectProviderForm(providers, &m.switchProviderID)
		return m, m.form.Init()

	case switchProviderView:
		m.form = nil

		// Remove mode: form complete → go to confirmation
		if m.switchRemoveMode {
			m.switchRemoveMode = false
			if m.switchProviderID == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No provider selected", isError: true}
				}
			}
			m.switchDeleteConfirm = false
			providerName := m.getProviderName(m.switchProviderID)
			m.currentView = deleteBindingConfirmView
			m.form = NewDeleteConfirmForm(providerName, &m.switchDeleteConfirm)
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
				return notificationMsg{message: "Target CLI not found", isError: true}
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
				m.form = NewSelectProviderForm(providers, &m.switchProviderID)
				return m, tea.Batch(
					m.form.Init(),
					func() tea.Msg {
						return notificationMsg{message: "No models for this provider. Try retrying fetch.", isError: true}
					},
				)
			}
			return m, func() tea.Msg {
				return notificationMsg{message: "No models available for this provider", isError: true}
			}
		}
		m.switchProviderModels = models

		// Determine flow: env var mapping (Claude/Codex) or direct model list (pi/OpenCode)
		m.switchUsesEnvMapping = targetCLI.Mutator == "claude-settings-json" || targetCLI.Mutator == "codex-config-toml"

		if m.switchUsesEnvMapping {
			m.currentView = switchMapModelsView
			form, extractFn := NewMapModelsForm(envVars, models)
			m.switchExtractFn = extractFn
			m.form = form
		} else {
			// pi/OpenCode/Copilot: auto-register all models
			registered := make([]string, 0, len(models))
			for _, mdl := range models {
				registered = append(registered, mdl.ModelName)
			}
			m.switchRegisteredModels = registered

			mappings := map[string]string{"_registered": strings.Join(registered, ",")}
			if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, mappings); err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
				}
			}

			m.switchModelMetadataSummary = buildMetadataSummary(registered, m.switchProviderID, m.switchUseCases)
			// Use SwitchToViewMsg to prevent Enter key bounce from provider form
			return m, func() tea.Msg {
				return SwitchToViewMsg{View: switchAdvancedConfigView}
			}
		}
		return m, m.form.Init()

	case switchSelectModelsView:
		m.form = nil
		selected := m.switchRegisterResult.RegisteredModels
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
			m.form = NewSelectModelsForm(m.switchProviderModels, &m.switchRegisterResult)
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
		if _, err := m.switchUseCases.Apply(m.switchTargetCLIID, 0); err != nil {
			log.Printf("config regenerate after model selection failed: %v", err)
		}
		m.currentView = switchAdvancedConfigView
		return m, nil

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
		m.form = NewEditCLIPathForm(cli, &m.editCLIPathResult)
		return m, m.form.Init()

	case editCLIPathView:
		m.form = nil
		m.currentView = dashboardView
		if m.editCLIPathResult.ConfigPath != "" {
			if err := m.switchUseCases.UpdateCLIConfigPath(m.editCLIPathResult.CLIID, m.editCLIPathResult.ConfigPath); err != nil {
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
				return notificationMsg{message: "CLI not found", isError: true}
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
		m.form = NewRestoreBackupForm(backups, &m.restoreSelectedPath)
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
	}

	return m, nil
}

var (
	logo = []string{
		`╭──────────────────────────╮`,
		`│       ◆  aimux  ◆        │`,
		`│    AI Multiplexer        │`,
		`╰──────────────────────────╯`,
	}

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(aimuxT.Accent).
			Padding(0, 2)

	viewPadding = lipgloss.NewStyle().Padding(1, 2)

	notifOKStyle = lipgloss.NewStyle().
			Background(aimuxT.BgBase).
			Foreground(aimuxT.Green).
			Padding(0, 2).
			Bold(true)

	notifErrStyle = lipgloss.NewStyle().
			Background(aimuxT.BgBase).
			Foreground(aimuxT.Red).
			Padding(0, 2).
			Bold(true)
)

func (m *model) renderDashboardSummary() string {
	var b strings.Builder

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

	provStr := fmt.Sprintf("%d active", activeProv)
	if errorProv > 0 {
		provStr += fmt.Sprintf(", %d error", errorProv)
	}

	// Build a bordered summary box
	summaryStyle := lipgloss.NewStyle().
		Foreground(aimuxT.TextSecondary).
		Padding(0, 2)

	b.WriteString(aimuxT.Help.Render("Summary"))
	b.WriteString("\n")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("Providers: %s", provStr)))
	b.WriteString("\n")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("CLIs:      %d active, %d inactive", activeCLIs, inactiveCLIs)))
	b.WriteString("\n\n")

	return b.String()
}

func (m *model) View() string {
	m.syncSwitchStep()

	// Wrap form views with stepper for switch flow steps.
	if m.form != nil {
		content := m.form.View()
		if m.switchTotalSteps > 0 {
			content = viewPadding.Render(m.renderSwitchStepper()) + "\n" + content
		}
		return content
	}

	if m.loading {
		spin := m.spinner.View()
		msg := m.loadingMsg
		if msg == "" {
			msg = "Working..."
		}
		bottom := aimuxT.Help.Render(fmt.Sprintf("%s %s", spin, msg))
		return viewPadding.Render(bottom)
	}

	var content string
	switch m.currentView {
	case dashboardView:
		summary := m.renderDashboardSummary()
		menu := RenderMenu(m.menuSelected, len(m.providers) > 0)
		// Render logo: first/last line in accent (border), middle lines in secondary
		borderStyle := lipgloss.NewStyle().Foreground(aimuxT.Accent).Padding(0, 2)
		contentStyle := lipgloss.NewStyle().Foreground(aimuxT.TextSecondary).Padding(0, 2)
		logoLines := []string{
			borderStyle.Render(logo[0]),
			contentStyle.Render(logo[1]),
			contentStyle.Render(logo[2]),
			borderStyle.Render(logo[3]),
		}
		logoStr := lipgloss.JoinVertical(lipgloss.Left, logoLines...)

		// Welcome message on first run (no providers configured)
		var welcome string
		if len(m.providers) == 0 {
			welcomeBox := lipgloss.NewStyle().
				Foreground(aimuxT.TextSecondary).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(aimuxT.Accent).
				Padding(1, 2).
				Render(
					lipgloss.JoinVertical(lipgloss.Left,
						lipgloss.NewStyle().Bold(true).Foreground(aimuxT.Accent).Render("Welcome to aimux! 🎯"),
						"",
						"Centralize your AI provider credentials and switch between",
						"providers for Claude Code, OpenCode, Codex, Copilot, and pi.",
						"",
						fmt.Sprintf("Press Enter on %s to add your first provider.",
							lipgloss.NewStyle().Bold(true).Foreground(aimuxT.Accent).Render("Manage Providers")),
					),
				)
			welcome = "\n" + welcomeBox + "\n"
		}

		content = lipgloss.JoinVertical(lipgloss.Left, logoStr, summary, welcome, menu)
		content = viewPadding.Render(content)

	case providerListView:
		content = RenderProviderList(m.providers, m.selectedProviderID, m.width, m.allModels, m.activeMultiplexes)
		content = viewPadding.Render(content)

	case switchManageBindingsView:
		content = m.renderManageBindings()
		content = lipgloss.JoinVertical(lipgloss.Left, m.renderSwitchStepper(), content)
		content = viewPadding.Render(content)

	case switchAdvancedConfigView:
		content = m.renderAdvancedConfigReview()
		content = lipgloss.JoinVertical(lipgloss.Left, m.renderSwitchStepper(), content)
		content = viewPadding.Render(content)

	case switchConfirmationView:
		if m.switchDryRun != nil {
			var sb strings.Builder
			sb.WriteString(aimuxT.Title.Render("Dry-run Preview"))
			sb.WriteString("\n\n")
			sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("Target CLI:  %s", m.switchDryRun.CLIName)))
			sb.WriteString("\n")
			sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("Config:      %s", m.switchDryRun.ConfigPath)))
			sb.WriteString("\n\n")

			// New env vars to be written
			sb.WriteString(aimuxT.Help.Render("New configuration:"))
			sb.WriteString("\n")
			for k, v := range m.switchDryRun.EnvVars {
				if v != "" {
					sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("  %s = %s", k, v)))
					sb.WriteString("\n")
				}
			}

			// Diff: show current config content if available
			if m.switchDryRunCurrentConfig != "" {
				sb.WriteString("\n")
				sb.WriteString(aimuxT.Help.Render("Current config:"))
				sb.WriteString("\n")
				lines := strings.Split(m.switchDryRunCurrentConfig, "\n")
				maxLines := 15
				if len(lines) > maxLines {
					lines = lines[:maxLines]
					lines = append(lines, "  ...")
				}
				for _, line := range lines {
					sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("  │ %s", line)))
					sb.WriteString("\n")
				}
			}

			sb.WriteString("\n")
			sb.WriteString(aimuxT.Help.Render("Enter = Apply · Esc = Abort"))
			content = viewPadding.Render(sb.String())
		} else {
			var sb strings.Builder
			sb.WriteString(aimuxT.Title.Render("Profile Activated"))
			sb.WriteString("\n\n")
			sb.WriteString(aimuxT.ItemDesc.Render("The config has been written and multiplex is active."))
			sb.WriteString("\n")
			if m.switchBackupPath != "" {
				sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("Backup saved to:")))
				sb.WriteString("\n")
				sb.WriteString(aimuxT.ItemDesc.Render(fmt.Sprintf("  %s", m.switchBackupPath)))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			sb.WriteString(aimuxT.Help.Render("Press Enter or Esc to return to dashboard"))
			content = viewPadding.Render(sb.String())
		}

	default:
		content = "Loading..."
	}

	// Prepend stepper for other switch views not covered above (already added for manage/advancedConfig/confirmation)
	if m.switchTotalSteps > 0 && content != "" {
		switch m.currentView {
		case switchTargetCLIView, switchProviderView, switchMapModelsView, switchSelectModelsView:
			// These use forms and are handled above.
		default:
			// Already handled above for manageView/advancedConfig/confirmation.
		}
	}

	if m.notification != "" {
		style := notifErrStyle
		if m.notificationIsMsg {
			style = notifOKStyle
		}
		bar := style.Width(m.width).Render("  " + m.notification)
		content = lipgloss.JoinVertical(lipgloss.Left, content, "\n", bar)
	}

	return content
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
		return switchRegisterModelsView
	case switchMapModelsView:
		if m.switchInManageMode {
			return switchManageBindingsView
		}
		return switchProviderView
	case switchSelectModelsView:
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

	// Read current config for diff preview
	m.switchDryRunCurrentConfig = ""
	if dryRun != nil && dryRun.ConfigPath != "" {
		data, err := os.ReadFile(dryRun.ConfigPath)
		if err == nil && len(data) > 0 {
			m.switchDryRunCurrentConfig = string(data)
		}
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

// renderAdvancedConfigReview builds a human-readable summary of the model
// metadata that will be written to the target CLI's config.
func (m *model) renderManageBindings() string {
	var b strings.Builder
	cliName := ""
	for _, tc := range m.targetCLIs {
		if tc.ID == m.switchTargetCLIID {
			cliName = tc.Name
			break
		}
	}

	// Title
	b.WriteString(aimuxT.Title.Render("Provider Bindings — " + cliName))
	b.WriteString("\n\n")

	if len(m.switchCLIBindings) == 0 {
		b.WriteString("  No providers bound yet.")
		b.WriteString("\n")
	} else {
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
				if len(mappings) > 60 {
					mappings = mappings[:57] + "..."
				}
			}

			titleStyle := aimuxT.ItemTitle
			detailStyle := aimuxT.ItemDesc
			if selected {
				titleStyle = aimuxT.SelTitle
				detailStyle = aimuxT.SelDesc
			}

			// Render title line
			b.WriteString(titleStyle.Render(" " + bp.ProviderName))
			b.WriteString("\n")

			// Render description lines
			if mappings != "" {
				b.WriteString(detailStyle.Render(fmt.Sprintf("  %s", mappings)))
				b.WriteString("\n")
			}
			b.WriteString(detailStyle.Render(fmt.Sprintf("  %s", bp.ActivatedAt)))
			b.WriteString("\n")

			if i < len(m.switchCLIBindings)-1 {
				spacer := aimuxT.Inactive.Copy().
					Foreground(aimuxT.TextDim).
					Render("  ")
				b.WriteString(spacer)
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(aimuxT.Help.Render("↑/↓ navigate · a add · d remove · e edit models · Enter apply all · Esc back"))
	b.WriteString("\n")
	return b.String()
}

func (m *model) renderAdvancedConfigReview() string {
	var b strings.Builder
	b.WriteString(aimuxT.Title.Render("Advanced Model Configuration"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("  Registered models: %d\n\n", len(m.switchRegisteredModels)))

	if len(m.switchModelMetadataSummary) == 0 {
		b.WriteString(aimuxT.ItemDesc.Render("  No advanced metadata available for these models."))
		b.WriteString("\n")
		b.WriteString(aimuxT.ItemDesc.Render("  Default settings will be used."))
		b.WriteString("\n")
	} else {
		for _, line := range m.switchModelMetadataSummary {
			b.WriteString(aimuxT.ItemDesc.Render("  " + line))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(aimuxT.Help.Render("Enter = Proceed to apply · Esc = Back to model selection"))
	return b.String()
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
		m.switchTotalSteps = 4
		m.switchStepLabel = "Select Models"
	case switchAdvancedConfigView:
		if m.switchUsesEnvMapping {
			m.switchStep = switchStepReviewConfig
			m.switchTotalSteps = 5
			m.switchStepLabel = switchStepLabels[switchStepReviewConfig]
		} else {
			m.switchStep = switchStepReviewConfig - 1
			m.switchTotalSteps = 4
			m.switchStepLabel = "Review Configuration"
		}
	case switchConfirmationView:
		if m.switchUsesEnvMapping {
			m.switchStep = switchStepConfirm
			m.switchTotalSteps = 5
			m.switchStepLabel = switchStepLabels[switchStepConfirm]
		} else {
			m.switchStep = switchStepConfirm - 1
			m.switchTotalSteps = 4
			m.switchStepLabel = "Confirm & Apply"
		}
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
func (m *model) renderSwitchStepper() string {
	if m.switchTotalSteps == 0 {
		return ""
	}

	dotDone := aimuxT.SelTitle.Render("●")
	dotEmpty := aimuxT.Inactive.Render("○")
	dotCurrent := lipgloss.NewStyle().
		Foreground(aimuxT.AccentAlt).
		Bold(true).
		Render("◉")

	var dots []string
	for i := 1; i <= m.switchTotalSteps; i++ {
		if i < m.switchStep {
			dots = append(dots, dotDone)
		} else if i == m.switchStep {
			dots = append(dots, dotCurrent)
		} else {
			dots = append(dots, dotEmpty)
		}
	}
	dotStr := strings.Join(dots, " ")

	title := fmt.Sprintf("Step %d/%d: %s", m.switchStep, m.switchTotalSteps, m.switchStepLabel)
	titleStr := aimuxT.Help.Render(title)

	return lipgloss.JoinVertical(lipgloss.Left,
		titleStr,
		"  "+dotStr,
	)
}

func (m *model) renderHelpOverlay() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		aimuxT.Title.Render("Help & Shortcuts"),
		"",
		aimuxT.ItemDesc.Render("  ↑/↓ k/j  — Navigate list"),
		aimuxT.ItemDesc.Render("  Enter    — Select / Confirm"),
		aimuxT.ItemDesc.Render("  Esc      — Go back / Abort"),
		aimuxT.ItemDesc.Render("  a        — Add provider"),
		aimuxT.ItemDesc.Render("  d        — Delete / Remove"),
		aimuxT.ItemDesc.Render("  e        — Edit models"),
		aimuxT.ItemDesc.Render("  r        — Retry model fetch"),
		aimuxT.ItemDesc.Render("  t        — Test connectivity"),
		aimuxT.ItemDesc.Render("  ?        — Toggle this help"),
		aimuxT.ItemDesc.Render("  q / Ctrl+C  — Quit"),
		"",
		aimuxT.Help.Render("Press any key to close"),
	)
}

func (m *model) isSingleSelectForm() bool {
	return m.currentView == switchTargetCLIView ||
		m.currentView == switchProviderView ||
		m.currentView == switchMapModelsView ||
		m.currentView == switchSelectModelsView ||
		m.currentView == manageCLIView ||
		m.currentView == deleteBindingConfirmView ||
		m.currentView == restoreCLIView ||
		m.currentView == restoreBackupView
}
