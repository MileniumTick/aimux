package mutators

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jchavarriam/aimux/internal/domain"
	"github.com/jchavarriam/aimux/internal/infrastructure/config"
)

// CopilotEnvFile mutates Copilot CLI's environment configuration by writing
// a .env file with provider settings.
// Registered as: "copilot-env-file"
type CopilotEnvFile struct{}

// Mutate writes a .env file with provider settings, creating a backup of any
// existing .env file first.
func (m *CopilotEnvFile) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	// Resolve the directory: configPath may point to a directory or file
	envDir := configPath
	if !strings.HasSuffix(configPath, "/") && filepath.Ext(configPath) == "" {
		// Bare path, treat as directory
	} else if filepath.Ext(configPath) != "" {
		// File path — use its directory
		envDir = filepath.Dir(configPath)
	}

	if err := os.MkdirAll(envDir, 0700); err != nil {
		return nil, fmt.Errorf("create env directory: %w", err)
	}

	envPath := filepath.Join(envDir, ".env")

	// Backup existing .env if present
	var backupResult *domain.BackupResult
	if _, err := os.Stat(envPath); err == nil {
		bp, err := config.CreateBackup(envPath)
		if err == nil {
			backupResult = &domain.BackupResult{BackupPath: bp}
		}
	}

	// Determine provider type
	providerType := "openai"
	if pt, ok := mutatorConfig["provider_type"].(string); ok && pt != "" {
		providerType = pt
	}

	isLocal := false
	if l, ok := mutatorConfig["local"].(bool); ok {
		isLocal = l
	}

	// Build .env content
	var lines []string
	lines = append(lines, "COPILOT_PROVIDER_BASE_URL="+provider.BaseURL)
	lines = append(lines, "COPILOT_PROVIDER_TYPE="+providerType)

	if !isLocal && provider.APIKey != "" {
		lines = append(lines, "COPILOT_PROVIDER_API_KEY="+provider.APIKey)
	}

	for _, val := range modelMappings {
		if val != "" {
			lines = append(lines, "COPILOT_MODEL="+val)
			break
		}
	}

	content := strings.Join(lines, "\n") + "\n"

	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil {
		return nil, fmt.Errorf("write .env file: %w", err)
	}

	return backupResult, nil
}
