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
		BaseURL: "https://bifrost.example.com/v1",
		APIKey:  "sk-copilot-key",
		Status:  "active",
	}
}

func TestCopilotShellProfile_WritesToProfile(t *testing.T) {
	m := &CopilotShellProfile{}
	dir := t.TempDir()

	// Simulate a shell profile
	profilePath := filepath.Join(dir, ".zshrc")
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	// Override home to use temp dir
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Pre-write existing content
	os.WriteFile(profilePath, []byte("export EDITOR=vim\n"), 0644)

	provider := defaultCopilotProvider()
	mappings := map[string]string{"COPILOT_MODEL": "bifrost-sonnet"}
	cfg := map[string]any{"provider_type": "openai"}

	_, err := m.Mutate("", mappings, provider, cfg)
	if err != nil {
		t.Fatalf("Mutate failed: %v", err)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "COPILOT_PROVIDER_BASE_URL") {
		t.Error("expected COPILOT_PROVIDER_BASE_URL in profile")
	}
	if !strings.Contains(content, "COPILOT_PROVIDER_TYPE") {
		t.Error("expected COPILOT_PROVIDER_TYPE in profile")
	}
	if !strings.Contains(content, "COPILOT_PROVIDER_API_KEY") {
		t.Error("expected COPILOT_PROVIDER_API_KEY in profile")
	}
	if !strings.Contains(content, "COPILOT_MODEL") {
		t.Error("expected COPILOT_MODEL in profile")
	}
	if !strings.Contains(content, shellBlockStart) {
		t.Error("expected start marker")
	}
	if !strings.Contains(content, shellBlockEnd) {
		t.Error("expected end marker")
	}
	// Original content preserved
	if !strings.Contains(content, "export EDITOR=vim") {
		t.Error("original profile content should be preserved")
	}
}

func TestCopilotShellProfile_UsesRegisteredModels(t *testing.T) {
	m := &CopilotShellProfile{}
	dir := t.TempDir()

	profilePath := filepath.Join(dir, ".zshrc")
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	provider := defaultCopilotProvider()
	// Simulate multi-select flow: no COPILOT_MODEL in mappings
	cfg := map[string]any{
		"provider_type":     "openai",
		"_registered_models": []string{"deepseek-v4-flash", "deepseek-v4-pro"},
	}

	_, err := m.Mutate("", map[string]string{}, provider, cfg)
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	data, _ := os.ReadFile(profilePath)
	content := string(data)

	if !strings.Contains(content, "COPILOT_MODEL") || !strings.Contains(content, "deepseek-v4-flash") {
		t.Errorf("expected COPILOT_MODEL with deepseek-v4-flash, got:\n%s", content)
	}
}

func TestCopilotShellProfile_IdempotentReplace(t *testing.T) {
	m := &CopilotShellProfile{}
	dir := t.TempDir()

	profilePath := filepath.Join(dir, ".zshrc")
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	provider := defaultCopilotProvider()
	mappings := map[string]string{"COPILOT_MODEL": "bifrost-sonnet"}
	cfg := map[string]any{"provider_type": "openai"}

	// First write
	_, err := m.Mutate("", mappings, provider, cfg)
	if err != nil {
		t.Fatalf("first Mutate: %v", err)
	}

	// Second write with different model
	mappings2 := map[string]string{"COPILOT_MODEL": "bifrost-pro"}
	_, err = m.Mutate("", mappings2, provider, cfg)
	if err != nil {
		t.Fatalf("second Mutate: %v", err)
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	content := string(data)

	// Should only have ONE block
	if strings.Count(content, shellBlockStart) != 1 {
		t.Errorf("expected 1 start marker, got %d", strings.Count(content, shellBlockStart))
	}

	// Should have the new model
	if !strings.Contains(content, "bifrost-pro") {
		t.Error("expected updated model in profile")
	}
}

func TestCopilotShellProfile_NoAPIKeyForLocal(t *testing.T) {
	m := &CopilotShellProfile{}
	dir := t.TempDir()

	profilePath := filepath.Join(dir, ".zshrc")
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	provider := defaultCopilotProvider()
	cfg := map[string]any{"local": true}

	_, err := m.Mutate("", map[string]string{"COPILOT_MODEL": "local-model"}, provider, cfg)
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}

	data, _ := os.ReadFile(profilePath)
	if strings.Contains(string(data), "COPILOT_PROVIDER_API_KEY") {
		t.Error("should not write API key for local provider")
	}
}

func TestCopilotShellProfile_RemoveEnvBlock(t *testing.T) {
	dir := t.TempDir()

	profilePath := filepath.Join(dir, ".zshrc")
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// Write a profile with the block
	content := "export EDITOR=vim\n" +
		shellBlockStart + "\n" +
		"# Managed by aimux\n" +
		`export COPILOT_PROVIDER_BASE_URL="https://example.com"` + "\n" +
		shellBlockEnd + "\n"
	os.WriteFile(profilePath, []byte(content), 0644)

	if err := RemoveShellEnvBlock(); err != nil {
		t.Fatalf("RemoveShellEnvBlock: %v", err)
	}

	data, _ := os.ReadFile(profilePath)
	result := string(data)

	if strings.Contains(result, shellBlockStart) {
		t.Error("start marker should be removed")
	}
	if strings.Contains(result, "COPILOT_PROVIDER_BASE_URL") {
		t.Error("env vars should be removed")
	}
	if !strings.Contains(result, "export EDITOR=vim") {
		t.Error("original content should remain")
	}
}

func TestShellProfileCmds_DetectsFish(t *testing.T) {
	os.Setenv("SHELL", "/opt/homebrew/bin/fish")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	path, exportFmt, _, err := shellProfileCmds()
	if err != nil {
		t.Fatalf("shellProfileCmds: %v", err)
	}
	if !strings.HasSuffix(path, "config.fish") {
		t.Errorf("expected fish config, got %s", path)
	}
	if !strings.Contains(exportFmt, "set -gx") {
		t.Errorf("expected fish export format, got %s", exportFmt)
	}
}

func TestShellProfileCmds_DetectsZsh(t *testing.T) {
	os.Setenv("SHELL", "/bin/zsh")
	t.Cleanup(func() { os.Unsetenv("SHELL") })

	path, exportFmt, _, err := shellProfileCmds()
	if err != nil {
		t.Fatalf("shellProfileCmds: %v", err)
	}
	if !strings.HasSuffix(path, ".zshrc") {
		t.Errorf("expected .zshrc, got %s", path)
	}
	if !strings.Contains(exportFmt, "export") {
		t.Errorf("expected bash/zsh export format, got %s", exportFmt)
	}
}

func TestShellProfileCmds_DefaultsToBash(t *testing.T) {
	os.Unsetenv("SHELL")

	path, exportFmt, _, err := shellProfileCmds()
	if err != nil {
		t.Fatalf("shellProfileCmds: %v", err)
	}
	if !strings.HasSuffix(path, ".bashrc") {
		t.Errorf("expected .bashrc, got %s", path)
	}
	if !strings.Contains(exportFmt, "export") {
		t.Errorf("expected bash export format, got %s", exportFmt)
	}
}

func TestShellProfileCmds_FallbackWhenNoShell(t *testing.T) {
	os.Unsetenv("SHELL")
	t.Cleanup(func() { os.Setenv("SHELL", "/bin/zsh") })

	path, _, _, err := shellProfileCmds()
	if err != nil {
		t.Fatalf("shellProfileCmds: %v", err)
	}
	if !strings.HasSuffix(path, ".bashrc") {
		t.Errorf("expected .bashrc fallback, got %s", path)
	}
}
