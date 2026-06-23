package domain

import "strings"

// FindCLIByName finds a TargetCLI by case-insensitive name match.
// Returns nil if not found.
func FindCLIByName(clis []TargetCLI, name string) *TargetCLI {
	for i := range clis {
		if strings.EqualFold(clis[i].Name, name) {
			return &clis[i]
		}
	}
	return nil
}

// TargetCLI represents a target CLI row in the database.
type TargetCLI struct {
	ID            int64
	Name          string
	ConfigPath    string
	EnvVars       string
	Mutator       string // mutator registry key, e.g. "claude-settings-json"
	MutatorConfig string // JSON object for mutator-specific configuration
}

// TargetCLIRepository defines the interface for target CLI persistence.
type TargetCLIRepository interface {
	List() ([]TargetCLI, error)
	Get(id int64) (TargetCLI, error)
	Update(TargetCLI) error
}

// BackupResult contains information about the backup performed during mutation.
type BackupResult struct {
	BackupPath string
}

// ConfigMutator defines the interface for mutating a target CLI's config file(s).
// Each target CLI has a corresponding mutator implementation registered by name.
type ConfigMutator interface {
	// Mutate writes model mappings and provider config to the target CLI's config file(s).
	// configPath is the resolved path from target_clis.config_path.
	// modelMappings is a map of env var names to model names (may be empty).
	// provider contains the full Provider record (base_url, api_key, auth_token).
	// mutatorConfig is the parsed JSON from target_clis.mutator_config.
	Mutate(
		configPath string,
		modelMappings map[string]string,
		provider Provider,
		mutatorConfig map[string]any,
	) (*BackupResult, error)
}
