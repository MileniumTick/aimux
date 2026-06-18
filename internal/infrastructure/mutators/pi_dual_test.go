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
		ApiType: domain.ApiTypeAnthropic,
	}
}

func defaultPiConfig() map[string]any {
	return map[string]any{
		"provider_id": "bifrost",
	}
}

func TestPiDualJSON_WritesModelsJSON(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")

	os.WriteFile(modelsPath, []byte(`{"version": 1}`), 0644)

	mappings := map[string]string{
		"DEFAULT_MODEL": "bifrost-sonnet",
		"FAST_MODEL":    "bifrost-haiku",
	}

	result, err := m.Mutate(dir, mappings, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected backup path")
	}

	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)

	if bifrost["baseUrl"] != "https://bifrost.example.com/v1" {
		t.Errorf("expected baseUrl, got %v", bifrost["baseUrl"])
	}
	if bifrost["apiKey"] != "sk-pi-test-key" {
		t.Errorf("expected apiKey, got %v", bifrost["apiKey"])
	}
	// auto-derived from provider.ApiType (Anthropic)
	if bifrost["api"] != "anthropic-messages" {
		t.Errorf("expected api 'anthropic-messages', got %v", bifrost["api"])
	}

	models := bifrost["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	modelIDs := make(map[string]bool)
	for _, m := range models {
		entry := m.(map[string]any)
		modelIDs[entry["id"].(string)] = true
	}
	if !modelIDs["bifrost-sonnet"] {
		t.Error("expected bifrost-sonnet model")
	}
	if !modelIDs["bifrost-haiku"] {
		t.Error("expected bifrost-haiku model")
	}
}

func TestPiDualJSON_PreservesExistingProviders(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()
	modelsPath := filepath.Join(dir, "models.json")

	initial := `{
		"providers": {
			"anthropic": {
				"baseUrl": "https://api.anthropic.com",
				"apiKey": "sk-ant-old",
				"api": "anthropic-messages",
				"models": [{"id": "claude-sonnet-4", "name": "Claude Sonnet 4"}]
			}
		}
	}`
	os.WriteFile(modelsPath, []byte(initial), 0644)

	mappings := map[string]string{"DEFAULT_MODEL": "bifrost-model"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	if _, exists := providers["anthropic"]; !exists {
		t.Error("expected anthropic provider preserved")
	}
	if _, exists := providers["bifrost"]; !exists {
		t.Error("expected bifrost provider added")
	}

	// Verify anthropic entry was preserved intact
	anthropic := providers["anthropic"].(map[string]any)
	if anthropic["apiKey"] != "sk-ant-old" {
		t.Error("expected anthropic apiKey preserved")
	}
}

func TestPiDualJSON_MissingProviderID(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	_, err := m.Mutate(dir, map[string]string{}, defaultPiProvider(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing provider_id")
	}
}

func TestPiDualJSON_APIOverride(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	cfg := map[string]any{
		"provider_id": "bifrost",
		"api":         "openai-completions",
	}

	mappings := map[string]string{"M": "m1"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	modelsPath := filepath.Join(dir, "models.json")
	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	bifrost := providers["bifrost"].(map[string]any)
	// override wins over auto-derived "anthropic-messages"
	if bifrost["api"] != "openai-completions" {
		t.Errorf("expected override 'openai-completions', got %v", bifrost["api"])
	}
}

func TestPiDualJSON_CustomPath(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	modelsPath := filepath.Join(dir, "custom_models.json")

	cfg := map[string]any{
		"provider_id": "bifrost",
		"models_path": modelsPath,
	}

	mappings := map[string]string{"DEFAULT_MODEL": "bifrost-sonnet"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		t.Error("expected custom models.json to be created")
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
	models := bifrost["models"].([]any)
	if len(models) != 0 {
		t.Errorf("expected empty models, got %d", len(models))
	}
}

func TestPiDualJSON_NoAuthFile(t *testing.T) {
	m := &PiDualJSON{}
	dir := t.TempDir()

	_, err := m.Mutate(dir, map[string]string{"M": "m1"}, defaultPiProvider(), defaultPiConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	authPath := filepath.Join(dir, "auth.json")
	if _, err := os.Stat(authPath); !os.IsNotExist(err) {
		t.Error("auth.json should not be created — apiKey lives in models.json")
	}
}

func TestPiDualJSON_MetadataEnrichment(t *testing.T) {
	// When _model_metadata is present, model entries include context_window,
	// max_tokens, reasoning, and input_modalities.
	m := &PiDualJSON{}
	dir := t.TempDir()

	cfg := map[string]any{
		"provider_id": "deepseek",
		"_model_metadata": map[string]any{
			"deepseek-v4-pro": map[string]any{
				"context_window":   float64(1_000_000),
				"max_tokens":       float64(384_000),
				"reasoning":        true,
				"input_modalities": []any{"text"},
			},
		},
	}

	mappings := map[string]string{"DEFAULT_MODEL": "deepseek-v4-pro"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	modelsPath := filepath.Join(dir, "models.json")
	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	ds := providers["deepseek"].(map[string]any)
	models := ds["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}

	entry := models[0].(map[string]any)
	if entry["context_window"] != float64(1_000_000) {
		t.Errorf("expected context_window 1000000, got %v", entry["context_window"])
	}
	if entry["max_tokens"] != float64(384_000) {
		t.Errorf("expected max_tokens 384000, got %v", entry["max_tokens"])
	}
	if entry["reasoning"] != true {
		t.Errorf("expected reasoning true, got %v", entry["reasoning"])
	}
	if input, ok := entry["input"].([]any); !ok || len(input) != 1 || input[0] != "text" {
		t.Errorf("expected input [text], got %v", entry["input"])
	}
}

func TestPiDualJSON_NoMetadataFallback(t *testing.T) {
	// Without _model_metadata, model entries only have id and name.
	m := &PiDualJSON{}
	dir := t.TempDir()

	cfg := map[string]any{"provider_id": "deepseek"}

	mappings := map[string]string{"DEFAULT_MODEL": "deepseek-v4-pro"}
	_, err := m.Mutate(dir, mappings, defaultPiProvider(), cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	modelsPath := filepath.Join(dir, "models.json")
	content, _ := os.ReadFile(modelsPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	providers := root["providers"].(map[string]any)
	ds := providers["deepseek"].(map[string]any)
	models := ds["models"].([]any)
	entry := models[0].(map[string]any)

	if entry["id"] != "deepseek-v4-pro" {
		t.Errorf("expected id, got %v", entry["id"])
	}
	if _, exists := entry["context_window"]; exists {
		t.Error("expected no context_window without metadata")
	}
}
