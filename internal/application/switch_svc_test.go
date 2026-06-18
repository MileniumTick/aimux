package application

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBindProfile_FullMapping(t *testing.T) {
	uc := setupSwitchTest(t)
	pid := addTestProvider(t, uc, "BindFull", "http://test.full")
	insertTestModels(t, uc, pid, []string{"claude-haiku-3", "claude-sonnet-4", "claude-opus-4"})

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "claude-haiku-3",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "claude-opus-4",
		"CLAUDE_CODE_SUBAGENT_MODEL":     "claude-sonnet-4",
	}

	if err := uc.BindProfile(1, pid, mappings); err != nil {
		t.Fatalf("BindProfile failed: %v", err)
	}

	bound, err := uc.GetBoundModels(1)
	if err != nil {
		t.Fatalf("GetBoundModels failed: %v", err)
	}
	if len(bound) != 4 {
		t.Fatalf("expected 4 bound models, got %d", len(bound))
	}
	if bound["ANTHROPIC_DEFAULT_HAIKU_MODEL"] != "claude-haiku-3" {
		t.Errorf("expected haiku model, got %q", bound["ANTHROPIC_DEFAULT_HAIKU_MODEL"])
	}
}

func TestBindProfile_PartialBinding(t *testing.T) {
	uc := setupSwitchTest(t)
	pid := addTestProvider(t, uc, "BindPartial", "http://test.partial")
	insertTestModels(t, uc, pid, []string{"claude-sonnet-4", "claude-opus-4"})

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "",
	}

	if err := uc.BindProfile(1, pid, mappings); err != nil {
		t.Fatalf("BindProfile failed: %v", err)
	}

	bound, _ := uc.GetBoundModels(1)
	if len(bound) != 2 {
		t.Fatalf("expected 2 entries in bound models (including empty), got %d", len(bound))
	}
	if bound["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "" {
		t.Errorf("expected empty string for opus model, got %q", bound["ANTHROPIC_DEFAULT_OPUS_MODEL"])
	}
}

func TestBindProfile_UnknownModelID(t *testing.T) {
	uc := setupSwitchTest(t)
	pid := addTestProvider(t, uc, "BindUnknown", "http://test.unknown")
	insertTestModels(t, uc, pid, []string{"claude-sonnet-4"})

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL": "non-existent-model",
	}

	err := uc.BindProfile(1, pid, mappings)
	if err == nil {
		t.Fatal("expected error for unknown model, got nil")
	}

	bound, _ := uc.GetBoundModels(1)
	if len(bound) != 0 {
		t.Error("active multiplex should not have been created after failed binding")
	}
}

func TestBindProfile_UnknownEnvVar(t *testing.T) {
	uc := setupSwitchTest(t)
	pid := addTestProvider(t, uc, "BindEnvVar", "http://test.envvar")
	insertTestModels(t, uc, pid, []string{"claude-sonnet-4"})

	mappings := map[string]string{
		"UNKNOWN_VAR": "claude-sonnet-4",
	}

	err := uc.BindProfile(1, pid, mappings)
	if err == nil {
		t.Fatal("expected error for unknown env var, got nil")
	}
}

func TestGetBoundModels_NoActiveProfile(t *testing.T) {
	uc := setupSwitchTest(t)

	bound, err := uc.GetBoundModels(1)
	if err != nil {
		t.Fatalf("GetBoundModels with no profile should not error: %v", err)
	}
	if len(bound) != 0 {
		t.Errorf("expected empty map, got %v", bound)
	}
}

func TestApply_ExistingSettingsJSON(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "ApplyTest", "http://test.apply")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	initialContent := `{"allowWrites": true}`
	os.WriteFile(configPath, []byte(initialContent), 0644)

	// Point target CLI to temp config path
	h.uc.providerRepo.UpdateStatus(pid, "active")
	h.db.Exec("UPDATE target_clis SET config_path = ? WHERE id = 1", configPath)

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	if err := h.uc.BindProfile(1, pid, mappings); err != nil {
		t.Fatalf("BindProfile failed: %v", err)
	}

	result, err := h.uc.Apply(1, pid)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result == nil || result.BackupPath == "" {
		t.Error("expected backup path after apply")
	}

	content, _ := os.ReadFile(configPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	if root["allowWrites"] != true {
		t.Error("expected 'allowWrites' preserved")
	}
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "claude-sonnet-4" {
		t.Errorf("expected model in env, got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["ANTHROPIC_API_KEY"] != "test-api-key" {
		t.Errorf("expected api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestApply_NonExistentConfigFile(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "ApplyNew", "http://test.newfile")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "new_settings.json")

	h.uc.providerRepo.UpdateStatus(pid, "active")
	h.db.Exec("UPDATE target_clis SET config_path = ? WHERE id = 1", configPath)

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	h.uc.BindProfile(1, pid, mappings)

	result, err := h.uc.Apply(1, pid)
	if err != nil {
		t.Fatalf("Apply on non-existent config file failed: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("expected config file to be created")
	}

	if result != nil && result.BackupPath != "" {
		t.Error("expected no backup for new file")
	}

	content, _ := os.ReadFile(configPath)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_API_KEY"] != "test-api-key" {
		t.Errorf("expected api key, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestApply_APIKeySecurityCleanup(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "ApplySecurity", "http://test.security")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	initialContent := `{"ANTHROPIC_API_KEY": "old-leaked-key", "theme": "dark"}`
	os.WriteFile(configPath, []byte(initialContent), 0644)

	h.uc.providerRepo.UpdateStatus(pid, "active")
	h.db.Exec("UPDATE target_clis SET config_path = ? WHERE id = 1", configPath)

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	h.uc.BindProfile(1, pid, mappings)
	h.uc.Apply(1, pid)

	content, _ := os.ReadFile(configPath)
	var root map[string]any
	json.Unmarshal(content, &root)

	if _, exists := root["ANTHROPIC_API_KEY"]; exists {
		t.Error("root ANTHROPIC_API_KEY must be removed (security invariant)")
	}
	if root["theme"] != "dark" {
		t.Error("expected 'theme' preserved")
	}
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_API_KEY"] != "test-api-key" {
		t.Errorf("expected new api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestApply_PartialMappingsEmptyValues(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "ApplyPartial", "http://test.partial")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	os.WriteFile(configPath, []byte(`{"key": "val"}`), 0644)

	h.uc.providerRepo.UpdateStatus(pid, "active")
	h.db.Exec("UPDATE target_clis SET config_path = ? WHERE id = 1", configPath)

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	h.uc.BindProfile(1, pid, mappings)
	h.uc.Apply(1, pid)

	content, _ := os.ReadFile(configPath)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)

	if _, exists := env["ANTHROPIC_DEFAULT_HAIKU_MODEL"]; exists {
		t.Error("empty haiku model should be excluded from env block")
	}
	if env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "claude-sonnet-4" {
		t.Errorf("expected sonnet model in env, got %v", env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
	if env["ANTHROPIC_API_KEY"] != "test-api-key" {
		t.Errorf("expected api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}

func TestApply_MissingMutatorError(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "MissingMutator", "http://test.missing")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	os.WriteFile(configPath, []byte(`{"key": "val"}`), 0644)

	h.uc.providerRepo.UpdateStatus(pid, "active")
	h.db.Exec("UPDATE target_clis SET config_path = ?, mutator = ? WHERE id = 1", configPath, "unknown-mutator")

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	h.uc.BindProfile(1, pid, mappings)

	_, err := h.uc.Apply(1, pid)
	if err == nil {
		t.Fatal("expected error for unknown mutator, got nil")
	}
	if err.Error() != "mutator 'unknown-mutator' not registered for CLI 'claude-code'" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestApply_FallbackEmptyMutator(t *testing.T) {
	h := setupSwitchHarness(t)
	pid := addTestProvider(t, h.uc, "FallbackEmpty", "http://test.fallback")
	insertTestModels(t, h.uc, pid, []string{"claude-sonnet-4"})

	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	os.WriteFile(configPath, []byte(`{"key": "val"}`), 0644)

	h.uc.providerRepo.UpdateStatus(pid, "active")
	// Set mutator to empty string to test fallback to claude-settings-json
	h.db.Exec("UPDATE target_clis SET config_path = ?, mutator = '' WHERE id = 1", configPath)

	mappings := map[string]string{
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4",
	}
	h.uc.BindProfile(1, pid, mappings)

	result, err := h.uc.Apply(1, pid)
	if err != nil {
		t.Fatalf("Apply with empty mutator should fall back to claude-settings-json: %v", err)
	}
	if result == nil || result.BackupPath == "" {
		t.Error("expected backup path after apply")
	}

	content, _ := os.ReadFile(configPath)
	var root map[string]any
	json.Unmarshal(content, &root)
	env := root["env"].(map[string]any)
	if env["ANTHROPIC_API_KEY"] != "test-api-key" {
		t.Errorf("expected api key in env, got %v", env["ANTHROPIC_API_KEY"])
	}
}
