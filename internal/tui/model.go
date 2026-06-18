package tui

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
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
	switchBackupPath           string
	switchDryRun               *application.DryRunResult
	switchModelMetadataSummary []string                 // display lines for advanced config review
	switchUsesEnvMapping       bool                     // true: Claude Code/Codex map env→model; false: pi/OpenCode list models directly
	switchInManageMode         bool                     // true when adding another provider to a CLI that already has bindings
	switchCLIBindings          []domain.ActiveMultiplex // bound providers for selected CLI
	switchRemoveMode           bool                     // true when picking a provider to remove
	switchDeleteConfirm        bool                     // true when confirming binding deletion

	selectedProviderID int64

	selectedCLIID     int64
	editCLIPathResult EditCLIPathResult

	loading bool

	notification      string
	notificationIsMsg bool

	restoreCLIName      string
	restoreSelectedPath string
	restoreBackups      []application.BackupOption

	updateInfo UpdateInfo
}

type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
}

func NewModel(providerUseCases *application.ProviderUseCases, switchUseCases *application.SwitchUseCases) *model {
	return &model{
		providerUseCases: providerUseCases,
		switchUseCases:   switchUseCases,
		currentView:      dashboardView,
		menuSelected:     menuItemManageProviders,
	}
}

func (m *model) Init() tea.Cmd {
	return m.refreshData
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

	case DashboardRefreshMsg:
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
		return m, tea.Tick(4*time.Second, func(_ time.Time) tea.Msg {
			return clearNotificationMsg{}
		})

	case clearNotificationMsg:
		m.notification = ""
		return m, nil

	case retryFetchResultMsg:
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
	if m.notification != "" && key.Matches(msg, menuKeys.Esc) {
		m.notification = ""
		return m, nil
	}

	switch m.currentView {
	case dashboardView:
		switch {
		case key.Matches(msg, menuKeys.Up):
			if m.menuSelected > 0 {
				m.menuSelected--
				if m.menuSelected == menuItemSwitch && len(m.providers) == 0 {
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
				return m, func() tea.Msg {
					diff, err := m.providerUseCases.RetryFetch(m.selectedProviderID)
					return retryFetchResultMsg{diff: diff, err: err}
				}
			}
		case key.Matches(msg, menuKeys.Test):
			if m.selectedProviderID > 0 {
				m.loading = true
				return m, func() tea.Msg {
					err := m.providerUseCases.TestConnectivity(m.selectedProviderID)
					return testConnectivityResultMsg{err: err}
				}
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
		case key.Matches(msg, menuKeys.Delete):
			if len(m.switchCLIBindings) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers to remove", isError: true}
				}
			}
			m.currentView = switchProviderView
			var bound []domain.Provider
			for _, b := range m.switchCLIBindings {
				p, err := m.providerUseCases.Get(b.ProviderID)
				if err == nil {
					bound = append(bound, p)
				}
			}
			if len(bound) == 0 {
				return m, func() tea.Msg {
					return notificationMsg{message: "No providers to remove", isError: true}
				}
			}
			m.form = NewSelectProviderToRemoveForm(bound, &m.switchProviderID)
			m.switchRemoveMode = true
			return m, m.form.Init()
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
			applyResult, err := m.switchUseCases.Apply(m.switchTargetCLIID, pid)
			m.switchDryRun = nil
			if err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Apply failed: %s", err.Error()), isError: true}
				}
			}
			if applyResult != nil {
				m.switchBackupPath = applyResult.BackupPath
			} else {
				m.switchBackupPath = ""
			}
			return m, func() tea.Msg {
				return notificationMsg{message: "Profile activated successfully", isError: false}
			}
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
		name := trimSpaces(m.addProviderResult.Name)
		baseURL := trimSpaces(m.addProviderResult.BaseURL)
		discoveryURL := trimSpaces(m.addProviderResult.DiscoveryURL)
		apiKey := trimSpaces(m.addProviderResult.APIKey)
		authToken := trimSpaces(m.addProviderResult.AuthToken)
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
		baseURL := trimSpaces(m.editProviderResult.BaseURL)
		discoveryURL := trimSpaces(m.editProviderResult.DiscoveryURL)
		apiKey := trimSpaces(m.editProviderResult.APIKey)
		authToken := trimSpaces(m.editProviderResult.AuthToken)
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
			// pi/OpenCode/Copilot: skip env mapping, go to model selection
			// These CLIs don't use env var → model mapping; they list models directly.
			// Store only _registered metadata so the mutator knows which models to include.
			registered := make([]string, 0, len(models))
			for _, mdl := range models {
				registered = append(registered, mdl.ModelName)
			}
			m.switchRegisteredModels = registered

			// Mapping just stores _registered metadata — no env var keys needed.
			mappings := map[string]string{"_registered": strings.Join(registered, ",")}
			if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, mappings); err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
				}
			}

			m.currentView = switchAdvancedConfigView

			m.switchModelMetadataSummary = buildMetadataSummary(registered, m.switchProviderID, m.switchUseCases)
			return m, nil
		}
		return m, m.form.Init()

	case switchMapModelsView:
		m.form = nil
		result := m.switchExtractFn()

		if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, result.Mappings); err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
			}
		}

		// Auto-register all non-empty model IDs from mappings — no redundant checkbox step.
		// Collect unique model IDs from mapping values (variant names like "model:variant").
		seen := make(map[string]bool)
		var registered []string
		for _, v := range result.Mappings {
			if v != "" && !seen[v] {
				seen[v] = true
				registered = append(registered, v)
			}
		}
		m.switchRegisteredModels = registered

		// Store _registered list in bindings
		if len(registered) > 0 {
			currentMappings, err := m.switchUseCases.GetBoundModels(m.switchTargetCLIID)
			if err == nil {
				updated := make(map[string]string, len(currentMappings)+1)
				for k, v := range currentMappings {
					updated[k] = v
				}
				updated["_registered"] = strings.Join(registered, ",")
				_ = m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, updated)
			}
		}

		// Build model metadata summary and go to advanced config review
		m.switchModelMetadataSummary = buildMetadataSummary(registered, m.switchProviderID, m.switchUseCases)
		m.currentView = switchAdvancedConfigView
		return m, nil

	case manageCLIView:
		m.form = nil
		m.currentView = dashboardView
		return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }

	case editCLIPathView:
		m.form = nil
		m.currentView = manageCLIView
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
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Padding(0, 2)

	viewPadding = lipgloss.NewStyle().PaddingTop(2)

	notifOKStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("42")).
			Padding(0, 2).
			Bold(true)

	notifErrStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("167")).
			Padding(0, 2).
			Bold(true)
)

func (m *model) View() string {
	if m.form != nil {
		return m.form.View()
	}

	var content string
	switch m.currentView {
	case dashboardView:
		table := RenderTable(m.providers, m.activeMultiplexes, m.targetCLIs, m.width)
		menu := RenderMenu(m.menuSelected, len(m.providers) > 0)
		title := titleStyle.Render("aimux")
		content = lipgloss.JoinVertical(lipgloss.Left, title, table, menu)
		content = viewPadding.Render(content)

	case providerListView:
		content = RenderProviderList(m.providers, m.selectedProviderID, m.width)
		content = viewPadding.Render(content)

	case switchManageBindingsView:
		content = m.renderManageBindings()
		content = viewPadding.Render(content)

	case switchAdvancedConfigView:
		content = m.renderAdvancedConfigReview()
		content = viewPadding.Render(content)

	case switchConfirmationView:
		if m.switchDryRun != nil {
			envBlock := ""
			for k, v := range m.switchDryRun.EnvVars {
				if v != "" {
					envBlock += fmt.Sprintf("\n    %s = %s", k, v)
				}
			}
			content = fmt.Sprintf(
				"\n\n  Dry-run — the following will be applied:\n\n  Target CLI:  %s\n  Config:      %s\n  Env vars:%s\n\n  %s\n",
				m.switchDryRun.CLIName, m.switchDryRun.ConfigPath, envBlock,
				helpStyle.Render("Enter = Apply · Esc = Abort"),
			)
		} else {
			content = fmt.Sprintf(
				"\n\n  Profile activated successfully!\n\n  The config has been written and multiplex is active.\n",
			)
			if m.switchBackupPath != "" {
				content += fmt.Sprintf("\n  Backup saved to:\n  %s\n", m.switchBackupPath)
			}
			content += fmt.Sprintf("\n  %s\n\n",
				helpStyle.Render("Press Enter or Esc to return to dashboard"),
			)
		}
		content = viewPadding.Render(content)

	default:
		content = "Loading..."
	}

	if m.notification != "" {
		style := notifErrStyle
		if m.notificationIsMsg {
			style = notifOKStyle
		}
		bar := style.Width(m.width).Render("  " + m.notification)
		content = lipgloss.JoinVertical(lipgloss.Center, content, "\n", bar)
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

// resetSwitchState clears all switch-related fields to avoid stale state
// when returning to the dashboard or starting a new switch flow.
func (m *model) resetSwitchState() {
	m.switchTargetCLIID = 0
	m.switchProviderID = 0
	m.switchEnvVars = nil
	m.switchExtractFn = nil
	m.switchRegisteredModels = nil
	m.switchBackupPath = ""
	m.switchDryRun = nil
	m.switchModelMetadataSummary = nil
	m.switchUsesEnvMapping = false
	m.switchInManageMode = false
	m.switchCLIBindings = nil
	m.switchRemoveMode = false
	m.switchDeleteConfirm = false
	m.switchProviderModels = nil
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
	b.WriteString(fmt.Sprintf("\n  Provider Bindings — %s\n\n", cliName))

	if len(m.switchCLIBindings) == 0 {
		b.WriteString("  No providers bound yet.\n")
	} else {
		for i, bp := range m.switchCLIBindings {
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
			}
			b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, bp.ProviderName))
			if mappings != "" && len(mappings) > 60 {
				mappings = mappings[:57] + "..."
			}
			if mappings != "" {
				b.WriteString(fmt.Sprintf("     Models: %s\n", mappings))
			}
			b.WriteString(fmt.Sprintf("     Bound: %s\n", bp.ActivatedAt))
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("[A] Add provider  [D] Remove  [Enter] Apply all  [Esc] Back"))
	b.WriteString("\n")
	return b.String()
}

func (m *model) renderAdvancedConfigReview() string {
	var b strings.Builder
	b.WriteString("\n  Advanced Model Configuration\n\n")
	b.WriteString(fmt.Sprintf("  Registered models: %d\n\n", len(m.switchRegisteredModels)))

	if len(m.switchModelMetadataSummary) == 0 {
		b.WriteString("  No advanced metadata available for these models.\n")
		b.WriteString("  Default settings will be used.\n")
	} else {
		for _, line := range m.switchModelMetadataSummary {
			b.WriteString("  " + line + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Enter = Proceed to apply · Esc = Back to model selection"))
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

func (m *model) isSingleSelectForm() bool {
	return m.currentView == switchTargetCLIView ||
		m.currentView == switchProviderView ||
		m.currentView == manageCLIView ||
		m.currentView == deleteBindingConfirmView ||
		m.currentView == restoreCLIView ||
		m.currentView == restoreBackupView
}

func trimSpaces(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
