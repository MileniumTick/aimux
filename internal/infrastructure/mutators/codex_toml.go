package mutators

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// CodexConfigTOML mutates Codex's config.toml by building model_providers tables
// and writing the API key to a separate .env file.
// Registered as: "codex-config-toml"
type CodexConfigTOML struct{}

// Mutate reads the TOML config, builds model_providers section, and writes
// atomically with backup.
func (m *CodexConfigTOML) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	providerID, ok := mutatorConfig["provider_id"].(string)
	if !ok || providerID == "" {
		return nil, fmt.Errorf("codex mutator requires provider_id in mutator_config")
	}

	if err := config.PrepareDir(configPath); err != nil {
		return nil, err
	}

	// Create backup BEFORE reading — file existence on disk, not parse success
	var backupResult *domain.BackupResult
	if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(configPath)
		if err == nil {
			backupResult = &domain.BackupResult{BackupPath: bp}
		}
	}

	// Read existing TOML (or start empty)
	root := make(map[string]any)
	existingData, err := os.ReadFile(configPath)
	if err == nil {
		if _, err := toml.Decode(string(existingData), &root); err != nil {
			root = make(map[string]any)
		}
	}

	// Get first non-empty model name for top-level model
	var firstModel string
	for _, val := range modelMappings {
		if val != "" {
			firstModel = val
			break
		}
	}

	// Set top-level entries
	root["model"] = firstModel
	root["model_provider"] = providerID

	// Build model_providers section
	modelProviders, ok := root["model_providers"].(map[string]any)
	if !ok {
		modelProviders = make(map[string]any)
		root["model_providers"] = modelProviders
	}

	// Derive env key name from provider ID
	envKeyName := "CODEX_" + strings.ToUpper(providerID) + "_API_KEY"

	wireAPI, _ := mutatorConfig["wire_api"].(string)

	providerEntry := map[string]any{
		"name":    provider.Name,
		"base_url": provider.BaseURL,
		"env_key": envKeyName,
	}
	if wireAPI != "" {
		providerEntry["wire_api"] = wireAPI
	}

	modelProviders[providerID] = providerEntry

	// Marshal TOML
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(root); err != nil {
		return nil, fmt.Errorf("marshal toml: %w", err)
	}

	if err := config.AtomicWrite(buf.Bytes(), configPath); err != nil {
		return nil, err
	}

	// Write API key to separate .env file
	envDir := filepath.Dir(configPath)
	envPath := filepath.Join(envDir, ".env")
	envContent := envKeyName + "=" + provider.APIKey + "\n"
	if err := os.WriteFile(envPath, []byte(envContent), 0600); err != nil {
		return nil, fmt.Errorf("write env file: %w", err)
	}

	config.PruneBackups(configPath, 5)

	return backupResult, nil
}
