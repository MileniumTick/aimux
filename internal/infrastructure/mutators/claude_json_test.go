package mutators

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func defaultClaudeProvider() domain.Provider {
	return domain.Provider{
		Name:    "TestProvider",
		BaseURL: "https://api.test.com/v1",
		APIKey:  "sk-test-key-12345",
	}
}

func defaultClaudeConfig() map[string]any {
	return make(map[string]any)
}

func TestClaudeSettingsJSON_ExistingSettings(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"allowWrites": true, "theme": "dark"}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-20250514",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "claude-haiku-3-20250313",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "claude-sonnet-4-20250514",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "",
	}

	result, err := m.Mutate(path, mappings, defaultClaudeProvider(), defaultClaudeConfig())
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Fatal("expected backup path after mutate")
	}
	if _, err := os.Stat(result.BackupPath); os.IsNotExist(err) {
		t.Fatalf("backup file does not exist: %s", result.BackupPath)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read result: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if root["allowWrites"] != true {
		t.Error("expected 'allowWrites' preserved")
	}
	if root["theme"] != "dark" {
		t.Error("expected 'theme' preserved")
	}

	env, ok := root["env"].(map[string]any)
	if !ok {
		t.Fatal("expected 'env' block to exist")
	}

	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "claude-sonnet-4-20250514" {
		t.Errorf("expected sonnet model in env, got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}

	if _, exists := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; exists {
		t.Error("empty model mapping should be excluded from env block")
	}

	if env["ANTHROPIC_API_KEY"] != "sk-test-key-12345" {
		t.Errorf("expected 'sk-test-key-12345' in env, got %v", env["ANTHROPIC_API_KEY"])
	}

	if _, exists := root["ANTHROPIC_API_KEY"]; exists {
		t.Error("root ANTHROPIC_API_KEY should have been removed")
	}
}

func TestClaudeSettingsJSON_EmptyMappings(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "",
	}

	// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
		t.Fatalf("Mutate with empty mappings failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	env, ok := root["env"].(map[string]any)
	if !ok {
		t.Fatal("expected 'env' block to exist even with empty mappings")
	}

		if len(env) != 1 {
			t.Errorf("expected only ANTHROPIC_API_KEY in env, got %d entries", len(env))
		}
		if env["ANTHROPIC_API_KEY"] != "sk-test-key-12345" {
			t.Errorf("expected api key in env, got %v", env["ANTHROPIC_API_KEY"])
		}
		if _, exists := env["ANTHROPIC_BASE_URL"]; exists {
			t.Error("ANTHROPIC_BASE_URL should not be present when BaseURL is empty")
		}
}

func TestClaudeSettingsJSON_APIKeyCleanup(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"ANTHROPIC_API_KEY": "old-key", "env": {"ANTHROPIC_DEFAULT_SONNET_MODEL": "old-model"}}`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "new-model",
	}

	// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	if _, exists := root["ANTHROPIC_API_KEY"]; exists {
		t.Error("root ANTHROPIC_API_KEY must be removed (security invariant)")
	}

	env := root["env"].(map[string]any)
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "new-model" {
		t.Errorf("expected new model in env, got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["ANTHROPIC_API_KEY"] != "sk-test-key-12345" {
		t.Errorf("expected new api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestClaudeSettingsJSON_AtomicWrite(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"key": "original"}`
	os.WriteFile(path, []byte(initial), 0644)

	mappings := map[string]string{"VAR": "model"}
	// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("original path should be valid: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(content, &root); err != nil {
		t.Fatalf("result should be valid JSON: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("stale temp file found: %s", e.Name())
		}
	}
}

func TestClaudeSettingsJSON_PreserveExtraKeys(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"verbose": true, "maxTokens": 8192, "proxy": {"host": "localhost", "port": 8080}}`
	os.WriteFile(path, []byte(initial), 0644)

	mappings := map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4"}
	// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)

	if root["verbose"] != true {
		t.Error("expected 'verbose' preserved")
	}
	if root["maxTokens"] != float64(8192) {
		t.Error("expected 'maxTokens' preserved")
	}
	proxy, ok := root["proxy"].(map[string]any)
	if !ok || proxy["host"] != "localhost" {
		t.Error("expected 'proxy' object preserved")
	}
}

func TestClaudeSettingsJSON_BackupPruning(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	os.WriteFile(path, []byte(`{"data": "test"}`), 0644)

	mappings := map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "m1"}
	for i := 0; i < 7; i++ {
		// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
		provider := defaultClaudeProvider()
		provider.BaseURL = ""
		if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
			t.Fatalf("iteration %d failed: %v", i, err)
		}
	}

	entries, _ := os.ReadDir(dir)
	backupCount := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "settings.json.aimux-backup-") {
			backupCount++
		}
	}

	if backupCount > 5 {
		t.Errorf("expected at most 5 backups, got %d", backupCount)
	}
}

func TestClaudeSettingsJSON_AuthTokenExclusion(t *testing.T) {
	// When API key is set, existing ANTHROPIC_AUTH_TOKEN must be removed to
	// prevent the "both tokens set" 401 error from the Anthropic API.
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"env": {"ANTHROPIC_AUTH_TOKEN": "existing-oauth-token", "SOME_OTHER_VAR": "keep-me"}}`
	os.WriteFile(path, []byte(initial), 0644)

	mappings := map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4"}
	// Use a provider without BaseURL to avoid ANTHROPIC_BASE_URL polluting the env count.
	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, defaultClaudeConfig()); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)

	if _, exists := env["ANTHROPIC_AUTH_TOKEN"]; exists {
		t.Error("ANTHROPIC_AUTH_TOKEN must be removed when ANTHROPIC_API_KEY is set")
	}
	if env["SOME_OTHER_VAR"] != "keep-me" {
		t.Error("existing env vars must be preserved")
	}
	if env["ANTHROPIC_API_KEY"] != "sk-test-key-12345" {
		t.Errorf("expected api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestClaudeSettingsJSON_NewFile(t *testing.T) {
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "new_settings.json")

	mappings := map[string]string{"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4"}
	result, err := m.Mutate(path, mappings, defaultClaudeProvider(), defaultClaudeConfig())
	if err != nil {
		t.Fatalf("Mutate on non-existent file failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected config file to be created")
	}

	if result != nil && result.BackupPath != "" {
		t.Error("expected no backup for new file")
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_API_KEY"] != "sk-test-key-12345" {
		t.Errorf("expected api key, got %v", env["ANTHROPIC_API_KEY"])
	}
}
