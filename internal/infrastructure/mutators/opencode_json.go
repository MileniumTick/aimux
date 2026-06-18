package mutators

import (
	"fmt"
	"os"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// OpenCodeProviderJSON mutates OpenCode's opencode.json by building a provider
// entry under the provider key with npm package name, base URL, API key, and models.
// Registered as: "opencode-provider-json"
type OpenCodeProviderJSON struct{}

// Mutate reads the JSON config, builds a provider entry with models, and
// writes atomically with backup.
func (m *OpenCodeProviderJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	providerID, ok := mutatorConfig["provider_id"].(string)
	if !ok || providerID == "" {
		return nil, fmt.Errorf("opencode mutator requires provider_id in mutator_config")
	}

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

	// Ensure provider key exists
	providers, ok := root["provider"].(map[string]any)
	if !ok {
		providers = make(map[string]any)
		root["provider"] = providers
	}

	// Build provider entry
	providerEntry := map[string]any{
		"name": provider.Name,
		"options": map[string]any{
			"baseURL": provider.BaseURL,
			"apiKey":  provider.APIKey,
		},
	}

	// Build models map from model mappings.
	// If _registered_models is provided, use that list (free selection);
	// otherwise fall back to mapped model values (backward compatible).
	modelList := make([]string, 0)
	if registered, ok := mutatorConfig["_registered_models"].([]any); ok && len(registered) > 0 {
		for _, r := range registered {
			if name, ok := r.(string); ok && name != "" {
				modelList = append(modelList, name)
			}
		}
	}
	if len(modelList) == 0 {
		for _, val := range modelMappings {
			if val != "" {
				modelList = append(modelList, val)
			}
		}
	}

	models := make(map[string]any, len(modelList))
	for _, name := range modelList {
		models[name] = map[string]any{"name": name}
	}
	providerEntry["models"] = models

	// Set the provider entry (preserves other providers)
	providers[providerID] = providerEntry

	if err := config.WriteAtomicJSON(configPath, root); err != nil {
		return nil, err
	}

	// Clean up old backups
	config.PruneBackups(configPath, 5)

	return backupResult, nil
}
