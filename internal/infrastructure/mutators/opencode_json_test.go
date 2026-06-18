package mutators

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func defaultOpenCodeProvider() domain.Provider {
	return domain.Provider{
		Name:    "Bifrost",
		BaseURL: "https://bifrost.example.com/v1",
		APIKey:  "sk-bifrost-test",
	}
}

func defaultOpenCodeConfig() map[string]any {
	return map[string]any{
		"provider_id": "bifrost",
	}
}

func TestOpenCodeProviderJSON_CreatesProviderEntry(t *testing.T) {
	m := &OpenCodeProviderJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")

	// Write initial content so a backup is created
	os.WriteFile(path, []byte(`{"schema": "opencode"}`), 0644)

	mappings := map[string]string{
		"DEFAULT_MODEL": "gpt-4o",
		"FAST_MODEL":    "gpt-4o-mini",
	}

	result, err := m.Mutate(path, mappings, defaultOpenCodeProvider(), defaultOpenCodeConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected backup path")
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["provider"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)

	if bifrost["name"] != "Bifrost" {
		t.Errorf("expected name 'Bifrost', got %v", bifrost["name"])
	}

	options := bifrost["options"].(map[string]any)
	if options["baseURL"] != "https://bifrost.example.com/v1" {
		t.Errorf("expected baseURL, got %v", options["baseURL"])
	}
	if options["apiKey"] != "sk-bifrost-test" {
		t.Errorf("expected apiKey, got %v", options["apiKey"])
	}

	models := bifrost["models"].(map[string]any)
	if _, exists := models["gpt-4o"]; !exists {
		t.Error("expected gpt-4o model")
	}
	if _, exists := models["gpt-4o-mini"]; !exists {
		t.Error("expected gpt-4o-mini model")
	}
}

func TestOpenCodeProviderJSON_PreservesOtherProviders(t *testing.T) {
	m := &OpenCodeProviderJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")

	initial := `{
		"$schema": "https://opencode.ai/config.json",
		"model": "claude-sonnet-4",
		"provider": {
			"anthropic": {
				"npm": "@ai-sdk/anthropic",
				"name": "Anthropic",
				"options": {"baseURL": "https://api.anthropic.com", "apiKey": "sk-ant-..."},
				"models": {"claude-sonnet-4": {"name": "Claude Sonnet 4"}}
			}
		}
	}`
	os.WriteFile(path, []byte(initial), 0644)

	mappings := map[string]string{"DEFAULT_MODEL": "gpt-4o"}
	_, err := m.Mutate(path, mappings, defaultOpenCodeProvider(), defaultOpenCodeConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	if root["$schema"] != "https://opencode.ai/config.json" {
		t.Error("expected $schema preserved")
	}

	providers := root["provider"].(map[string]any)
	if _, exists := providers["anthropic"]; !exists {
		t.Error("expected anthropic provider preserved")
	}
	if _, exists := providers["bifrost"]; !exists {
		t.Error("expected bifrost provider added")
	}
}

func TestOpenCodeProviderJSON_MissingProviderID(t *testing.T) {
	m := &OpenCodeProviderJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")
	os.WriteFile(path, []byte(`{}`), 0644)

	_, err := m.Mutate(path, map[string]string{}, defaultOpenCodeProvider(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing provider_id")
	}
}


func TestOpenCodeProviderJSON_EmptyMappings(t *testing.T) {
	m := &OpenCodeProviderJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "opencode.json")

	_, err := m.Mutate(path, map[string]string{}, defaultOpenCodeProvider(), defaultOpenCodeConfig())
	if err != nil {
		t.Fatalf("Mutate with empty mappings failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["provider"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)
	models := bifrost["models"].(map[string]any)
	if len(models) != 0 {
		t.Errorf("expected empty models, got %d", len(models))
	}
}
