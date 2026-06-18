package mutators

import (
	"os"

	"github.com/jchavarriam/aimux/internal/domain"
	"github.com/jchavarriam/aimux/internal/infrastructure/config"
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

	// Write model mappings (skip empty values)
	for key, val := range modelMappings {
		if val != "" {
			env[key] = val
		}
	}

	// Mutually exclusive: API key vs OAuth token. Both set at once causes 401.
	if provider.APIKey != "" {
		env["ANTHROPIC_API_KEY"] = provider.APIKey
		delete(env, "ANTHROPIC_AUTH_TOKEN")
	} else if provider.AuthToken != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = provider.AuthToken
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
