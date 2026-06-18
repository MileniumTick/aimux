package mutators

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// PiDualJSON mutates Pi's dual JSON config files: models.json (provider definitions
// and models) and auth.json (credentials).
// Registered as: "pi-dual-json"
type PiDualJSON struct{}

// Mutate reads and writes both models.json and auth.json, creating backups for
// each file before mutation.
func (m *PiDualJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	providerID, ok := mutatorConfig["provider_id"].(string)
	if !ok || providerID == "" {
		return nil, fmt.Errorf("pi mutator requires provider_id in mutator_config")
	}

	providerType, ok := mutatorConfig["provider_type"].(string)
	if !ok || providerType == "" {
		return nil, fmt.Errorf("pi mutator requires provider_type in mutator_config")
	}

	// Resolve file paths
	piDir := configPath
	if filepath.Ext(configPath) != "" {
		piDir = filepath.Dir(configPath)
	}

	modelsPath := filepath.Join(piDir, "models.json")
	if mp, ok := mutatorConfig["models_path"].(string); ok && mp != "" {
		modelsPath = mp
	}

	authPath := filepath.Join(piDir, "auth.json")
	if ap, ok := mutatorConfig["auth_path"].(string); ok && ap != "" {
		authPath = ap
	}

	// Ensure directory exists
	if err := config.PrepareDir(modelsPath); err != nil {
		return nil, err
	}
	if err := config.PrepareDir(authPath); err != nil {
		return nil, err
	}

	// --- models.json ---
	var modelsBackupResult *domain.BackupResult
	if fi, err := os.Stat(modelsPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(modelsPath)
		if err != nil {
			return nil, err
		}
		modelsBackupResult = &domain.BackupResult{BackupPath: bp}
	}

	modelsRoot, err := config.ReadJSONWithLock(modelsPath)
	if err != nil {
		return nil, err
	}

	// Ensure providers key exists
	providers, ok := modelsRoot["providers"].(map[string]any)
	if !ok {
		providers = make(map[string]any)
		modelsRoot["providers"] = providers
	}

	// Build provider entry with models
	providerModels := make(map[string]any)
	for _, val := range modelMappings {
		if val != "" {
			providerModels[val] = map[string]any{"name": val}
		}
	}

	providerEntry := map[string]any{
		"type":    providerType,
		"base_url": provider.BaseURL,
		"models":   providerModels,
	}

	providers[providerID] = providerEntry

	if err := config.WriteAtomicJSON(modelsPath, modelsRoot); err != nil {
		return nil, err
	}
	config.PruneBackups(modelsPath, 5)

	// --- auth.json ---
	if fi, err := os.Stat(authPath); err == nil && fi.Mode().IsRegular() {
		if _, err := config.CreateBackup(authPath); err != nil {
			return nil, err
		}
	}

	authRoot, err := config.ReadJSONWithLock(authPath)
	if err != nil {
		return nil, err
	}

	// Set credentials for this provider (preserves other providers)
	authRoot[providerID] = map[string]any{
		"type": "api_key",
		"key":  provider.APIKey,
	}

	if err := config.WriteAtomicJSON(authPath, authRoot); err != nil {
		return nil, err
	}
	config.PruneBackups(authPath, 5)

	return modelsBackupResult, nil
}
