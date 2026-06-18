package tui

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
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
	manageCLIView
	editCLIPathView
	editProviderView
	restoreCLIView
	restoreBackupView
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

	switchTargetCLIID      int64
	switchProviderID       int64
	switchEnvVars          []string
	switchExtractFn        func() MapModelsResult
	switchRegisterResult   RegisterModelsResult
	switchRegisteredModels []string
	switchBackupPath       string
	switchDryRun           *application.DryRunResult

	selectedProviderID int64

	selectedCLIID     int64
	editCLIPathResult EditCLIPathResult

	loading bool
	spinner  spinner.Model

	notification      string
	notificationIsMsg bool

	restoreCLIName       string
	restoreSelectedPath  string
	restoreBackups       []application.BackupOption

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
		spinner:          spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(spinnerStyle)),
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
				if prev == switchTargetCLIView || prev == switchProviderView {
					return m, m.enterSwitchView(prev)
				}
				if prev == manageCLIView {
					return m, m.enterManageCLIView()
				}
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

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

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
					return m, tea.Batch(
						func() tea.Msg {
							err := m.providerUseCases.TestConnectivity(m.selectedProviderID)
							return testConnectivityResultMsg{err: err}
						},
						m.spinner.Tick,
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

	case switchConfirmationView:
		switch {
		case key.Matches(msg, menuKeys.Esc):
			m.switchDryRun = nil
			m.switchBackupPath = ""
			m.currentView = dashboardView
			return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
		case key.Matches(msg, menuKeys.Enter):
			if m.switchDryRun == nil {
				m.switchBackupPath = ""
				m.currentView = dashboardView
				return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }
			}
			applyResult, err := m.switchUseCases.Apply(m.switchTargetCLIID, m.switchProviderID)
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
		return m.enterManageCLIs()
	case menuItemRestore:
		return m.enterRestoreFlow()
	case menuItemExit:
		return m, tea.Quit
	}
	return m, nil
}

func (m *model) startSwitchFlow() (tea.Model, tea.Cmd) {
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

func (m *model) enterManageCLIs() (tea.Model, tea.Cmd) {
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

func (m *model) enterManageCLIView() tea.Cmd {
	clis, err := m.switchUseCases.ListTargetCLIs()
	if err != nil || len(clis) == 0 {
		return func() tea.Msg {
			return notificationMsg{message: "No target CLIs configured", isError: true}
		}
	}
	m.targetCLIs = clis
	m.selectedCLIID = 0
	m.form = NewSelectCLIForm(clis, &m.selectedCLIID)
	return m.form.Init()
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
		m.currentView = switchMapModelsView

		form, extractFn := NewMapModelsForm(envVars, models)
		m.switchExtractFn = extractFn
		m.form = form
		return m, m.form.Init()

	case switchMapModelsView:
		m.form = nil
		result := m.switchExtractFn()

		if err := m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, result.Mappings); err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Bind failed: %s", err.Error()), isError: true}
			}
		}

		// Build pre-selected set from mapped models
		preSelected := make(map[string]bool, len(result.Mappings))
		for _, v := range result.Mappings {
			if v != "" {
				preSelected[v] = true
			}
		}

		m.switchRegisterResult = RegisterModelsResult{}
		m.currentView = switchRegisterModelsView
		m.form = NewRegisterModelsForm(m.switchProviderModels, preSelected, &m.switchRegisterResult)
		return m, m.form.Init()

	case switchRegisterModelsView:
		m.form = nil

		// Update model_mappings to include _registered list
		m.switchRegisteredModels = m.switchRegisterResult.RegisteredModels
		if len(m.switchRegisteredModels) > 0 {
			currentMappings, err := m.switchUseCases.GetBoundModels(m.switchTargetCLIID)
			if err == nil {
				// Add _registered to the stored mappings
				updated := make(map[string]string, len(currentMappings)+1)
				for k, v := range currentMappings {
					updated[k] = v
				}
				// Store as comma-separated in a single key
				registeredStr := ""
				for i, r := range m.switchRegisteredModels {
					if i > 0 {
						registeredStr += ","
					}
					registeredStr += r
				}
				updated["_registered"] = registeredStr
				_ = m.switchUseCases.BindProfile(m.switchTargetCLIID, m.switchProviderID, updated)
			}
		}

		dryRun, err := m.switchUseCases.DryRun(m.switchTargetCLIID, m.switchProviderID)
		if err != nil {
			return m, func() tea.Msg {
				return notificationMsg{message: fmt.Sprintf("Dry-run failed: %s", err.Error()), isError: true}
			}
		}
		m.switchDryRun = dryRun
		m.switchBackupPath = ""
		m.currentView = switchConfirmationView
		return m, nil

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
		if m.editCLIPathResult.ConfigPath != "" {
			if err := m.switchUseCases.UpdateCLIConfigPath(m.editCLIPathResult.CLIID, m.editCLIPathResult.ConfigPath); err != nil {
				return m, func() tea.Msg {
					return notificationMsg{message: fmt.Sprintf("Failed to update CLI path: %s", err.Error()), isError: true}
				}
			}
		}
		m.currentView = dashboardView
		return m, func() tea.Msg { m.refreshData(); return DashboardRefreshMsg{} }

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

func (m *model) View() string {
	width := m.width
	if width < 1 {
		width = 80
	}

	if m.form != nil {
		return m.form.View()
	}

	var body string
	viewTitle := ""
	viewHints := ""

	switch m.currentView {
	case dashboardView:
		viewTitle = "Dashboard"
		viewHints = "↑/↓ navigate · Enter select · q quit"

		table := RenderTable(m.providers, m.activeMultiplexes, m.targetCLIs, width)
		menu := RenderMenu(m.menuSelected, len(m.providers) > 0)
		body = lipgloss.NewStyle().PaddingTop(2).Render(
			lipgloss.JoinVertical(lipgloss.Left, table, "\n", menu),
		)

	case providerListView:
		viewTitle = "Providers"
		viewHints = "↑/↓ navigate · Enter Switch · a add · d delete · e edit · r retry · t test · Esc back"
		body = lipgloss.NewStyle().PaddingTop(2).Render(
			RenderProviderList(m.providers, m.selectedProviderID, width),
		)

	case switchConfirmationView:
		viewTitle = "Switch"
		viewHints = "Enter Apply · Esc Abort"
		if m.switchDryRun != nil {
			envBlock := ""
			for k, v := range m.switchDryRun.EnvVars {
				if v != "" {
					envBlock += fmt.Sprintf("\n    %s = %s", k, v)
				}
			}
			body = fmt.Sprintf(
				"\n  Dry-run — the following will be applied:\n\n  Target CLI:  %s\n  Config:      %s\n  Env vars:%s\n",
				m.switchDryRun.CLIName, m.switchDryRun.ConfigPath, envBlock,
			)
		} else {
			body = "\n  Profile activated successfully!\n\n  The config has been written and multiplex is active."
			if m.switchBackupPath != "" {
				body += fmt.Sprintf("\n  Backup saved to:\n  %s", m.switchBackupPath)
			}
		}
		body = lipgloss.NewStyle().PaddingTop(2).Render(body)

	default:
		body = "Loading..."
	}

	// Header
	body = lipgloss.JoinVertical(lipgloss.Left,
		AppHeader(width, viewTitle),
		body,
	)

	// Notification
	if n := Notification(m.notification, !m.notificationIsMsg); n != "" {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", n)
	}

	// Status bar with optional spinner
	if m.loading {
		viewHints = m.spinner.View() + " " + viewHints
	}
	body = lipgloss.JoinVertical(lipgloss.Left, body, StatusBar(width, viewHints))

	return body
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
	case manageCLIView:
		return dashboardView
	case editCLIPathView:
		return manageCLIView
	case switchMapModelsView:
		return switchProviderView
	case switchRegisterModelsView:
		return switchMapModelsView
	case switchProviderView:
		return switchTargetCLIView
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

func (m *model) isSingleSelectForm() bool {
	return m.currentView == switchTargetCLIView ||
		m.currentView == switchProviderView ||
		m.currentView == manageCLIView ||
		m.currentView == editCLIPathView ||
		m.currentView == switchMapModelsView
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
