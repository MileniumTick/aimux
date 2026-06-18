package mutators

import (
	"os"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// ClaudeSettingsJSON mutates Claude Code's settings.json by building an env
// block from model mappings and provider API key.
// Registered as: "claude-settings-json"
type ClaudeSettingsJSON struct{}

// Mutate reads the JSON config, builds an env block from model mappings, sets
// the API key, and writes atomically with backup.
func (m *ClaudeSettingsJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	if err := config.PrepareDir(configPath); err != nil {
		return nil, err
	}

	// Create backup BEFORE reading — file existence on disk, not parse success
	var backupResult *domain.BackupResult
	if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(configPath)
		if err != nil {
			return nil, err
		}
		backupResult = &domain.BackupResult{BackupPath: bp}
	}

	root, err := config.ReadJSONWithLock(configPath)
	if err != nil {
		return nil, err
	}

	// Delete ANTHROPIC_API_KEY from root (security invariant)
	delete(root, "ANTHROPIC_API_KEY")

	// Merge into existing env instead of replacing — Claude Code stores
	// ANTHROPIC_AUTH_TOKEN (OAuth) here and replacing wipes it.
	env := make(map[string]any)
	if existing, ok := root["env"].(map[string]any); ok {
		for k, v := range existing {
			env[k] = v
		}
	}

	// Write model mappings (skip empty values).
	// Auto-detect 1M context window from model metadata and append "[1m]"
	// suffix — Claude Code uses this to enable full context window.
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	for key, val := range modelMappings {
		if val != "" {
			if md, ok := modelMeta[val].(map[string]any); ok {
				if cw, ok := md["context_window"].(float64); ok && cw >= 1_000_000 {
					val = val + "[1m]"
				}
			}
			env[key] = val
		}
	}

	// Always use ANTHROPIC_AUTH_TOKEN — ANTHROPIC_API_KEY causes login prompts
	// in Claude Code instead of using the key directly.
	token := provider.AuthToken
	if token == "" {
		token = provider.APIKey
	}
	if token != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = token
		delete(env, "ANTHROPIC_API_KEY")
	}

	if provider.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = provider.BaseURL
	}

	root["env"] = env

	if err := config.WriteAtomicJSON(configPath, root); err != nil {
		return nil, err
	}

	// Clean up old backups
	config.PruneBackups(configPath, 5)

	return backupResult, nil
}
