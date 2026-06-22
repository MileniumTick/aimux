package tui

import (
	"strings"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func TestRenderMenu_Items(t *testing.T) {
	result := RenderMenu(0, true)
	if !strings.Contains(result, "Bind CLI") {
		t.Error("expected 'Bind CLI' in menu")
	}
	if !strings.Contains(result, "Launch") {
		t.Error("expected 'Launch' in menu")
	}
	if !strings.Contains(result, "Manage Providers") {
		t.Error("expected 'Manage Providers' in menu")
	}
	if !strings.Contains(result, "Exit") {
		t.Error("expected 'Exit' in menu")
	}
}

func TestRenderMenu_SwitchDisabled(t *testing.T) {
	result := RenderMenu(2, false)
	if !strings.Contains(result, "Bind CLI") {
		t.Error("expected 'Bind CLI' in menu even when disabled")
	}
}

func TestMenuItemCount(t *testing.T) {
	if MenuItemCount() != 6 {
		t.Errorf("expected 6 menu items, got %d", MenuItemCount())
	}
}

func TestRenderProviderList_Empty(t *testing.T) {
	result := RenderProviderList(nil, 0, 0, nil, nil)
	if !strings.Contains(result, "No providers configured") {
		t.Error("expected empty state message")
	}
}

func TestRenderProviderList_WithProviders(t *testing.T) {
	providers := []domain.Provider{
		{ID: 1, Name: "Provider A", BaseURL: "https://a.test", Status: "active"},
		{ID: 2, Name: "Provider B", BaseURL: "https://b.test", Status: "error"},
	}

	result := RenderProviderList(providers, 1, 0, nil, nil)
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
	m := NewModel(nil, nil, "test")
	if m == nil {
		t.Fatal("NewModel returned nil")
	}
	if m.currentView != dashboardView {
		t.Errorf("expected dashboardView, got %d", m.currentView)
	}
	if m.version != "test" {
		t.Errorf("expected version test, got %s", m.version)
	}
}
