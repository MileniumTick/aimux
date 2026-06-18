package mutators

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func defaultCopilotProvider() domain.Provider {
	return domain.Provider{
		Name:    "Bifrost",
		BaseURL: "https://bifrost.example.com/v1",
		APIKey:  "sk-copilot-key",
	}
}

func TestCopilotEnvFile_WritesEnvFile(t *testing.T) {
	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, "copilot", ".env")
	os.MkdirAll(filepath.Dir(path), 0700)

	mappings := map[string]string{
		"COPILOT_MODEL": "bifrost-sonnet",
	}

	cfg := map[string]any{
		"provider_type": "openai",
	}

	result, err := m.Mutate(path, mappings, defaultCopilotProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result != nil {
		t.Error("expected no backup for new .env file")
	}

	content, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")

	foundBaseURL := false
	foundType := false
	foundAPIKey := false
	foundModel := false
	for _, line := range lines {
		if line == "COPILOT_PROVIDER_BASE_URL=https://bifrost.example.com/v1" {
			foundBaseURL = true
		}
		if line == "COPILOT_PROVIDER_TYPE=openai" {
			foundType = true
		}
		if line == "COPILOT_PROVIDER_API_KEY=sk-copilot-key" {
			foundAPIKey = true
		}
		if line == "COPILOT_MODEL=bifrost-sonnet" {
			foundModel = true
		}
	}

	if !foundBaseURL {
		t.Error("expected COPILOT_PROVIDER_BASE_URL in .env")
	}
	if !foundType {
		t.Error("expected COPILOT_PROVIDER_TYPE=openai in .env")
	}
	if !foundAPIKey {
		t.Error("expected COPILOT_PROVIDER_API_KEY in .env")
	}
	if !foundModel {
		t.Error("expected COPILOT_MODEL in .env")
	}
}

func TestCopilotEnvFile_LocalProvider(t *testing.T) {
	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	cfg := map[string]any{
		"provider_type": "openai",
		"local":         true,
	}

	_, err := m.Mutate(path, map[string]string{"COPILOT_MODEL": "local-model"}, defaultCopilotProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "COPILOT_PROVIDER_API_KEY") {
		t.Error("expected no API key for local provider")
	}
}

func TestCopilotEnvFile_AnthropicType(t *testing.T) {
	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	cfg := map[string]any{
		"provider_type": "anthropic",
	}

	_, err := m.Mutate(path, map[string]string{"COPILOT_MODEL": "claude"}, defaultCopilotProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "COPILOT_PROVIDER_TYPE=anthropic") {
		t.Error("expected COPILOT_PROVIDER_TYPE=anthropic")
	}
}

func TestCopilotEnvFile_BackupExisting(t *testing.T) {
	// Backups are centralized outside the source directory; redirect them to a
	// temp root so the test is isolated and does not touch the real store.
	t.Setenv("AIMUX_BACKUP_ROOT", filepath.Join(t.TempDir(), "bk"))

	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	original := []byte("EXISTING=value\n")
	os.WriteFile(path, original, 0644)

	result, err := m.Mutate(path, map[string]string{"COPILOT_MODEL": "new-model"}, defaultCopilotProvider(), map[string]any{
		"provider_type": "openai",
	})
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected a backup path in BackupResult")
	}
	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Fatalf("backup file does not exist: %s", result.BackupPath)
	}
	got, _ := os.ReadFile(result.BackupPath)
	if string(got) != string(original) {
		t.Errorf("backup should match original, got %q", string(got))
	}
}

func TestCopilotEnvFile_DirectoryCreated(t *testing.T) {
	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", ".env")

	_, err := m.Mutate(path, map[string]string{"COPILOT_MODEL": "m"}, defaultCopilotProvider(), map[string]any{
		"provider_type": "openai",
	})
	if err != nil {
		t.Fatalf("Mutate should create directories: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected .env file to be created")
	}
}

func TestCopilotEnvFile_DefaultProviderType(t *testing.T) {
	m := &CopilotEnvFile{}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")

	_, err := m.Mutate(path, map[string]string{"COPILOT_MODEL": "m"}, defaultCopilotProvider(), map[string]any{})
	if err != nil {
		t.Fatalf("Mutate with empty config should use defaults: %v", err)
	}

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "COPILOT_PROVIDER_TYPE=openai") {
		t.Error("expected default provider type 'openai'")
	}
}
