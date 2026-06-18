package mutators

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func defaultPiProvider() domain.Provider {
	return domain.Provider{
		Name:    "Bifrost",
		BaseURL: "https://bifrost.example.com/v1",
		APIKey:  "sk-pi-test-key",
	}
}

func defaultPiConfig() map[string]any {
	return map[string]any{
		"provider_id":   "bifrost",
		"provider_type": "openai-compatible",
	}
}

func TestPiDualJSON_WritesModelsAndAuth(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	authPath := filepath.Join(dir, "auth.json")

	// Write initial content so backups are created
	os.WriteFile(modelsPath, []byte(`{"version": 1}`), 0644)
	os.WriteFile(authPath, []byte(`{"version": 1}`), 0644)

	mappings := map[string]string{
		"DEFAULT_MODEL": "bifrost-sonnet",
		"FAST_MODEL":    "bifrost-haiku",
	}

	// Use directory-based config_path
	result, err := m.Mutate(dir, mappings, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected backup path")
	}

	// Check models.json
	modelsContent, _ := os.ReadFile(modelsPath)
	var modelsRoot map[string]any
	json.Unmarshal(modelsContent, &modelsRoot)

	providers := modelsRoot["providers"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)

	if bifrost["type"] != "openai-compatible" {
		t.Errorf("expected type 'openai-compatible', got %v", bifrost["type"])
	}
	if bifrost["base_url"] != "https://bifrost.example.com/v1" {
		t.Errorf("expected base_url, got %v", bifrost["base_url"])
	}

	models := bifrost["models"].(map[string]any)
	if _, exists := models["bifrost-sonnet"]; !exists {
		t.Error("expected bifrost-sonnet model")
	}
	if _, exists := models["bifrost-haiku"]; !exists {
		t.Error("expected bifrost-haiku model")
	}

	// Check auth.json
	authContent, _ := os.ReadFile(authPath)
	var authRoot map[string]any
	json.Unmarshal(authContent, &authRoot)

	bifrostAuth := authRoot["bifrost"].(map[string]any)
	if bifrostAuth["type"] != "api_key" {
		t.Errorf("expected type 'api_key', got %v", bifrostAuth["type"])
	}
	if bifrostAuth["key"] != "sk-pi-test-key" {
		t.Errorf("expected key, got %v", bifrostAuth["key"])
	}
}

func TestPiDualJSON_PreservesExistingProviders(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")
	authPath := filepath.Join(dir, "auth.json")

	initialModels := `{
		"providers": {
			"anthropic": {
				"type": "anthropic",
				"base_url": "https://api.anthropic.com",
				"models": {"claude-sonnet-4": {"name": "Claude Sonnet 4"}}
			}
		}
	}`
	initialAuth := `{
		"anthropic": {"type": "api_key", "key": "sk-ant-old"}
	}`
	os.WriteFile(modelsPath, []byte(initialModels), 0644)
	os.WriteFile(authPath, []byte(initialAuth), 0644)

	mappings := map[string]string{"DEFAULT_MODEL": "bifrost-model"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	// Check models.json preserved
	modelsContent, _ := os.ReadFile(modelsPath)
	var modelsRoot map[string]any
	json.Unmarshal(modelsContent, &modelsRoot)

	providers := modelsRoot["providers"].(map[string]any)
	if _, exists := providers["anthropic"]; !exists {
		t.Error("expected anthropic provider preserved in models.json")
	}
	if _, exists := providers["bifrost"]; !exists {
		t.Error("expected bifrost provider added in models.json")
	}

	// Check auth.json preserved
	authContent, _ := os.ReadFile(authPath)
	var authRoot map[string]any
	json.Unmarshal(authContent, &authRoot)

	if _, exists := authRoot["anthropic"]; !exists {
		t.Error("expected anthropic auth preserved")
	}
}

func TestPiDualJSON_MissingProviderID(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	_, err := m.Mutate(dir, map[string]string{}, defaultPiProvider(), map[string]any{
		"provider_type": "openai-compatible",
	})
	if err == nil {
		t.Fatal("expected error for missing provider_id")
	}
}

func TestPiDualJSON_MissingProviderType(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	_, err := m.Mutate(dir, map[string]string{}, defaultPiProvider(), map[string]any{
		"provider_id": "bifrost",
	})
	if err == nil {
		t.Fatal("expected error for missing provider_type")
	}
}

func TestPiDualJSON_CustomPaths(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	modelsPath := filepath.Join(dir, "custom_models.json")
	authPath := filepath.Join(dir, "custom_auth.json")

	cfg := map[string]any{
		"provider_id":   "bifrost",
		"provider_type": "openai-compatible",
		"models_path":   modelsPath,
		"auth_path":     authPath,
	}

	mappings := map[string]string{"DEFAULT_MODEL": "bifrost-sonnet"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		t.Error("expected custom models.json to be created")
	}
	if _, err := os.Stat(authPath); os.IsNotExist(err) {
		t.Error("expected custom auth.json to be created")
	}
}

func TestPiDualJSON_EmptyMappings(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	_, err := m.Mutate(dir, map[string]string{}, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate with empty mappings failed: %v", err)
	}

	modelsPath := filepath.Join(dir, "models.json")
	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)
	models := bifrost["models"].(map[string]any)
	if len(models) != 0 {
		t.Errorf("expected empty models, got %d", len(models))
	}
}
