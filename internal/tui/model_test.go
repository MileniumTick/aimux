package tui

import (
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModel_WindowSizeMsg(t *testing.T) {
	var m model
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m2 := result.(*model)
	if m2.width != 100 {
		t.Errorf("expected width 100, got %d", m2.width)
	}
	if m2.height != 40 {
		t.Errorf("expected height 40, got %d", m2.height)
	}
}

func TestModel_SwitchToViewMsg(t *testing.T) {
	var m model
	result, _ := m.Update(SwitchToViewMsg{View: providerListView})
	m2 := result.(*model)
	if m2.currentView != providerListView {
		t.Errorf("expected providerListView, got %v", m2.currentView)
	}
}

func TestModel_MenuNavigation_Down(t *testing.T) {
	// Add a provider so skipDisabledMenuItems has no effect
	m := model{
		currentView:  dashboardView,
		menuSelected: 0,
		providers:    []domain.Provider{{ID: 1, Name: "test"}},
	}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := result.(*model)
	if m2.menuSelected != 1 {
		t.Errorf("expected menuSelected 1, got %d", m2.menuSelected)
	}
}

func TestModel_MenuNavigation_DownAtEnd(t *testing.T) {
	m := model{
		currentView:  dashboardView,
		menuSelected: menuItemCount - 1,
		providers:    []domain.Provider{{ID: 1, Name: "test"}},
	}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := result.(*model)
	if m2.menuSelected != menuItemCount-1 {
		t.Errorf("expected menuSelected %d, got %d", menuItemCount-1, m2.menuSelected)
	}
}

func TestModel_MenuNavigation_Up(t *testing.T) {
	m := model{
		currentView:  dashboardView,
		menuSelected: 2,
		providers:    []domain.Provider{{ID: 1, Name: "test"}},
	}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := result.(*model)
	if m2.menuSelected != 1 {
		t.Errorf("expected menuSelected 1, got %d", m2.menuSelected)
	}
}

func TestModel_MenuNavigation_UpAtZero(t *testing.T) {
	m := model{
		currentView:  dashboardView,
		menuSelected: 0,
		providers:    []domain.Provider{{ID: 1, Name: "test"}},
	}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m2 := result.(*model)
	if m2.menuSelected != 0 {
		t.Errorf("expected menuSelected 0, got %d", m2.menuSelected)
	}
}

func TestModel_NotificationMsg(t *testing.T) {
	var m model
	result, cmd := m.Update(notificationMsg{message: "test notification"})
	m2 := result.(*model)
	want := "✓ test notification"
	if m2.notification != want {
		t.Errorf("expected %q, got %q", want, m2.notification)
	}
	if !m2.notificationIsMsg {
		t.Error("expected notificationIsMsg=true for info severity")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd (tick to clear after ttl)")
	}
}

func TestModel_ErrorNotification(t *testing.T) {
	var m model
	result, cmd := m.Update(notificationMsg{message: "error happened", isError: true})
	m2 := result.(*model)
	want := "✗ error happened"
	if m2.notification != want {
		t.Errorf("expected %q, got %q", want, m2.notification)
	}
	if m2.notificationIsMsg {
		t.Error("expected notificationIsMsg=false for error severity")
	}
	if cmd != nil {
		t.Error("expected nil cmd for persistent error notification")
	}
}

func TestModel_HelpToggle(t *testing.T) {
	m := model{currentView: dashboardView}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m2 := result.(*model)
	if !m2.showHelp {
		t.Error("expected showHelp=true after pressing '?'")
	}
}

func TestModel_HelpToggle_DismissOnAnyPress(t *testing.T) {
	m := model{showHelp: true}
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m2 := result.(*model)
	if m2.showHelp {
		t.Error("expected showHelp=false after pressing any key while help is showing")
	}
}

func TestModel_ClearNotification(t *testing.T) {
	m := model{notification: "✓ test"}
	result, _ := m.Update(clearNotificationMsg{})
	m2 := result.(*model)
	if m2.notification != "" {
		t.Errorf("expected empty notification, got %q", m2.notification)
	}
}

func TestModel_DashboardRefreshMsg_LoadingState(t *testing.T) {
	m := model{loading: true, loadingMsg: "fetching..."}
	result, _ := m.Update(DashboardRefreshMsg{})
	m2 := result.(*model)
	if m2.loading {
		t.Error("expected loading=false after DashboardRefreshMsg")
	}
	if m2.loadingMsg != "" {
		t.Errorf("expected empty loadingMsg, got %q", m2.loadingMsg)
	}
}

func TestModel_WindowSizeMsg_SmallTerminal(t *testing.T) {
	// Terminal below minimum size should still set width/height but not
	// trigger further processing (such as form resizing).
	var m model
	result, _ := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	m2 := result.(*model)
	if m2.width != 30 {
		t.Errorf("expected width 30, got %d", m2.width)
	}
	if m2.height != 10 {
		t.Errorf("expected height 10, got %d", m2.height)
	}
}

func TestModel_KeyIgnoredDuringLoading(t *testing.T) {
	m := model{
		loading:      true,
		currentView:  dashboardView,
		menuSelected: 0,
	}
	// Down key should be ignored while loading
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := result.(*model)
	if m2.menuSelected != 0 {
		t.Errorf("expected menuSelected 0 during loading, got %d", m2.menuSelected)
	}

	// Even Quit should be accepted during loading
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	_, ok := result.(*model)
	if !ok {
		t.Error("expected model returned for Quit during loading")
	}
}

func TestModel_NotificationTTL_RespectsCustom(t *testing.T) {
	var m model
	// Custom TTL of 100ms
	_, cmd := m.Update(notificationMsg{message: "custom", ttl: 100 * 1000000})
	if cmd == nil {
		t.Fatal("expected non-nil cmd for notification with custom ttl")
	}
	// Execute the tick to verify it produces a clearNotificationMsg
	msg := cmd()
	if _, ok := msg.(clearNotificationMsg); !ok {
		t.Errorf("expected clearNotificationMsg from tick, got %T", msg)
	}
}

func TestModel_DashboardRefreshMsg_EmptyProvidersAutoNavigate(t *testing.T) {
	m := model{
		currentView: dashboardView,
		providers:   nil,
	}
	result, cmd := m.Update(DashboardRefreshMsg{})
	m2 := result.(*model)
	if m2.currentView != providerListView {
		t.Errorf("expected auto-navigate to providerListView, got %v", m2.currentView)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for first-run notification")
	}
	msg := cmd()
	n, ok := msg.(notificationMsg)
	if !ok {
		t.Fatalf("expected notificationMsg, got %T", msg)
	}
	if n.message == "" {
		t.Error("expected non-empty first-run notification message")
	}
}

func TestModel_NotificationWarnSeverity(t *testing.T) {
	var m model
	result, cmd := m.Update(notificationMsg{message: "warning message", severity: "warn"})
	m2 := result.(*model)
	want := "⚠ warning message"
	if m2.notification != want {
		t.Errorf("expected %q, got %q", want, m2.notification)
	}
	if cmd == nil {
		t.Error("expected non-nil cmd for warn severity (tick with 5s ttl)")
	}
}
