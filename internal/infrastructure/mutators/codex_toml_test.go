package mutators

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jchavarriam/aimux/internal/domain"
)

func defaultCodexProvider() domain.Provider {
	return domain.Provider{
		Name:    "Bifrost",
		BaseURL: "https://bifrost.example.com",
		APIKey:  "sk-bifrost-key",
	}
}

func defaultCodexConfig() map[string]any {
	return map[string]any{
		"provider_id": "bifrost",
	}
}

func TestCodexConfigTOML_CreatesProviderSection(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Write initial content so a backup is created
	os.WriteFile(path, []byte(`[editor]
theme = "light"
`), 0644)

	mappings := map[string]string{
		"CODEX_MODEL": "gpt-5.4",
	}

	cfg := map[string]any{
		"provider_id": "bifrost",
		"wire_api":    "responses",
	}

	result, err := m.Mutate(path, mappings, defaultCodexProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected backup path")
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	if _, err := toml.Decode(string(content), &root); err != nil {
		t.Fatalf("should be valid TOML: %v", err)
	}

	if root["model"] != "gpt-5.4" {
		t.Errorf("expected model 'gpt-5.4', got %v", root["model"])
	}
	if root["model_provider"] != "bifrost" {
		t.Errorf("expected model_provider 'bifrost', got %v", root["model_provider"])
	}

	providers := root["model_providers"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)

	if bifrost["name"] != "Bifrost" {
		t.Errorf("expected name 'Bifrost', got %v", bifrost["name"])
	}
	if bifrost["base_url"] != "https://bifrost.example.com" {
		t.Errorf("expected base_url, got %v", bifrost["base_url"])
	}
	if bifrost["env_key"] != "CODEX_BIFROST_API_KEY" {
		t.Errorf("expected env_key 'CODEX_BIFROST_API_KEY', got %v", bifrost["env_key"])
	}
	if bifrost["wire_api"] != "responses" {
		t.Errorf("expected wire_api 'responses', got %v", bifrost["wire_api"])
	}
}

func TestCodexConfigTOML_PreservesOtherSections(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	initial := `
[editor]
theme = "dark"

[model_providers.anthropic]
name = "Anthropic"
base_url = "https://api.anthropic.com"
`
	os.WriteFile(path, []byte(initial), 0644)

	mappings := map[string]string{"CODEX_MODEL": "claude-sonnet-4"}
	_, err := m.Mutate(path, mappings, defaultCodexProvider(), defaultCodexConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	toml.Decode(string(content), &root)

	editor := root["editor"].(map[string]any)
	if editor["theme"] != "dark" {
		t.Error("expected editor.theme preserved")
	}

	providers := root["model_providers"].(map[string]any)
	if _, exists := providers["anthropic"]; !exists {
		t.Error("expected anthropic provider preserved")
	}
}

func TestCodexConfigTOML_MissingProviderID(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte(`{}`), 0644)

	_, err := m.Mutate(path, map[string]string{}, defaultCodexProvider(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing provider_id")
	}
}

func TestCodexConfigTOML_HandlesMissingFile(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "new_config.toml")
	os.MkdirAll(dir, 0755)

	mappings := map[string]string{"CODEX_MODEL": "claude-sonnet-4"}
	_, err := m.Mutate(path, mappings, defaultCodexProvider(), defaultCodexConfig())
	if err != nil {
		t.Fatalf("Mutate on missing file should create it: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected file to be created")
	}
}

func TestCodexConfigTOML_WritesEnvFile(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	mappings := map[string]string{"CODEX_MODEL": "gpt-4o"}
	_, err := m.Mutate(path, mappings, defaultCodexProvider(), defaultCodexConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	envPath := filepath.Join(dir, ".env")
	envContent, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("expected .env file to exist: %v", err)
	}

	if !strings.Contains(string(envContent), "CODEX_BIFROST_API_KEY=sk-bifrost-key") {
		t.Errorf("expected API key in .env file, got %q", string(envContent))
	}
}

func TestCodexConfigTOML_FirstModelUsed(t *testing.T) {
	m := &CodexConfigTOML{}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	mappings := map[string]string{"FIRST": "", "SECOND": "actual-model", "THIRD": "other-model"}
	_, err := m.Mutate(path, mappings, defaultCodexProvider(), defaultCodexConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	toml.Decode(string(content), &root)

	if root["model"] != "actual-model" && root["model"] != "other-model" {
		t.Errorf("expected one of the non-empty models, got %v", root["model"])
	}
}
