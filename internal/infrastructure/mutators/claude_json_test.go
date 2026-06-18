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

	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-test-key-12345" {
		t.Errorf("expected 'sk-test-key-12345' in env, got %v", env["ANTHROPIC_AUTH_TOKEN"])
	}

	if _, exists := root["ANTHROPIC_AUTH_TOKEN"]; exists {
		t.Error("root ANTHROPIC_AUTH_TOKEN should have been removed")
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
			t.Errorf("expected only ANTHROPIC_AUTH_TOKEN in env, got %d entries", len(env))
		}
		if env["ANTHROPIC_AUTH_TOKEN"] != "sk-test-key-12345" {
			t.Errorf("expected api key in env, got %v", env["ANTHROPIC_AUTH_TOKEN"])
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
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-test-key-12345" {
		t.Errorf("expected new api key in env, got %v", env["ANTHROPIC_AUTH_TOKEN"])
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
	// When API key is set (and AuthToken is empty), the API key should be written
	// as ANTHROPIC_AUTH_TOKEN, overwriting any existing value.
	// ANTHROPIC_API_KEY is never written because it causes login prompts in Claude Code.
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

	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-test-key-12345" {
		t.Errorf("expected ANTHROPIC_AUTH_TOKEN with api key value, got %v", env["ANTHROPIC_AUTH_TOKEN"])
	}
	if env["SOME_OTHER_VAR"] != "keep-me" {
		t.Error("existing env vars must be preserved")
	}
	if _, exists := env["ANTHROPIC_API_KEY"]; exists {
		t.Error("ANTHROPIC_API_KEY must never be written (causes Claude Code login prompts)")
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
	if env["ANTHROPIC_AUTH_TOKEN"] != "sk-test-key-12345" {
		t.Errorf("expected api key, got %v", env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestClaudeSettingsJSON_OneMillionContext(t *testing.T) {
	// Models with context_window >= 1M auto-get "[1m]" suffix from metadata.
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	cfg := map[string]any{
		"_model_metadata": map[string]any{
			"deepseek-v4-pro":   map[string]any{"context_window": float64(1_000_000)},
			"deepseek-v4-flash": map[string]any{"context_window": float64(1_000_000)},
		},
	}
	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-v4-pro",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "deepseek-v4-flash",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "",
	}

	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, cfg); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)

	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "deepseek-v4-pro[1m]" {
		t.Errorf("expected 'deepseek-v4-pro[1m]', got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "deepseek-v4-flash[1m]" {
		t.Errorf("expected 'deepseek-v4-flash[1m]', got %v", env["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
	if _, exists := env["ANTHROPIC_DEFAULT_OPUS_MODEL"]; exists {
		t.Error("empty mapping should not appear even with 1M context")
	}
}

func TestClaudeSettingsJSON_NoMillionContext(t *testing.T) {
	// Models with context_window < 1M get no suffix.
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	cfg := map[string]any{
		"_model_metadata": map[string]any{
			"gpt-4o": map[string]any{"context_window": float64(128_000)},
		},
	}
	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "gpt-4o",
	}

	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, cfg); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)

	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "gpt-4o" {
		t.Errorf("expected 'gpt-4o' without suffix, got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
}

func TestClaudeSettingsJSON_NoMetadataFallback(t *testing.T) {
	// Without _model_metadata, no suffix is applied (graceful degradation).
	m := &ClaudeSettingsJSON{}
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	cfg := map[string]any{} // no metadata
	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-v4-pro",
	}

	provider := defaultClaudeProvider()
	provider.BaseURL = ""
	if _, err := m.Mutate(path, mappings, provider, cfg); err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	content, _ := os.ReadFile(path)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)

	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "deepseek-v4-pro" {
		t.Errorf("expected 'deepseek-v4-pro' without suffix (no metadata), got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
}
