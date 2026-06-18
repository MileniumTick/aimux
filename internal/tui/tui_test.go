package tui

import (
	"strings"
	"testing"

	"github.com/jchavarriam/aimux/internal/domain"
)

func TestRenderTable_EmptyState(t *testing.T) {
	result := RenderTable(nil, nil, nil, 0)
	if !strings.Contains(result, "No CLIs configured") {
		t.Error("expected 'No CLIs configured' in empty table, got: " + result)
	}
}

func TestRenderTable_WithData(t *testing.T) {
	providers := []domain.Provider{
		{ID: 1, Name: "TestProvider", Status: "active"},
	}
	multiplexes := []domain.ActiveMultiplex{
		{
			TargetCLIID:   1,
			ProviderID:    1,
			ProviderName:  "TestProvider",
			ModelMappings: `{"test": "model-1"}`,
			CLIName:       "claude-code",
		},
	}
	targetCLIs := []domain.TargetCLI{
		{ID: 1, Name: "claude-code", ConfigPath: "/path/to/config"},
	}

	result := RenderTable(providers, multiplexes, targetCLIs, 0)
	if !strings.Contains(result, "ACTIVE") {
		t.Error("expected 'ACTIVE' status in table")
	}
	if !strings.Contains(result, "TestProvider") {
		t.Error("expected provider name in table")
	}
	if !strings.Contains(result, "claude-code") {
		t.Error("expected 'claude-code' CLI in table")
	}
}

func TestRenderMenu_Items(t *testing.T) {
	result := RenderMenu(0, true)
	if !strings.Contains(result, "Switch") {
		t.Error("expected 'Switch' in menu")
	}
	if !strings.Contains(result, "Manage Providers") {
		t.Error("expected 'Manage Providers' in menu")
	}
	if !strings.Contains(result, "Exit") {
		t.Error("expected 'Exit' in menu")
	}
}

func TestRenderMenu_SwitchDisabled(t *testing.T) {
	result := RenderMenu(1, false)
	if !strings.Contains(result, "Switch") {
		t.Error("expected 'Switch' in menu even when disabled")
	}
}

func TestMenuItemCount(t *testing.T) {
	if MenuItemCount() != 4 {
		t.Errorf("expected 4 menu items, got %d", MenuItemCount())
	}
}

func TestRenderProviderList_Empty(t *testing.T) {
	result := RenderProviderList(nil, 0, 0)
	if !strings.Contains(result, "No providers configured") {
		t.Error("expected empty state message")
	}
}

func TestRenderProviderList_WithProviders(t *testing.T) {
	providers := []domain.Provider{
		{ID: 1, Name: "Provider A", BaseURL: "https://a.test", Status: "active"},
		{ID: 2, Name: "Provider B", BaseURL: "https://b.test", Status: "error"},
	}

	result := RenderProviderList(providers, 1, 0)
	if !strings.Contains(result, "Provider A") {
		t.Error("expected 'Provider A' in list")
	}
	if !strings.Contains(result, "Provider B") {
		t.Error("expected 'Provider B' in list")
	}
	if !strings.Contains(result, "OK") {
		t.Error("expected 'OK' status marker")
	}
	if !strings.Contains(result, "ERROR") {
		t.Error("expected 'ERROR' status marker")
	}
}

func TestNewModel(t *testing.T) {
	m := NewModel(nil, nil)
	if m == nil {
		t.Fatal("NewModel returned nil")
	}
	if m.currentView != dashboardView {
		t.Errorf("expected dashboardView, got %d", m.currentView)
	}
}
