package application

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// SwitchUseCases handles profile switching and config mutation business logic.
type SwitchUseCases struct {
	providerRepo  domain.ProviderRepository
	cliRepo       domain.TargetCLIRepository
	multiplexRepo domain.MultiplexRepository
	mutators      map[string]domain.ConfigMutator
}

// NewSwitchUseCases creates a new SwitchUseCases.
func NewSwitchUseCases(
	providerRepo domain.ProviderRepository,
	cliRepo domain.TargetCLIRepository,
	multiplexRepo domain.MultiplexRepository,
	mutators map[string]domain.ConfigMutator,
) *SwitchUseCases {
	return &SwitchUseCases{
		providerRepo:  providerRepo,
		cliRepo:       cliRepo,
		multiplexRepo: multiplexRepo,
		mutators:      mutators,
	}
}

// Apply activates a profile and mutates the target CLI's config file.
func (uc *SwitchUseCases) Apply(targetCLIID, providerID int64) (*domain.BackupResult, error) {
	// Get provider for API key and config
	provider, err := uc.providerRepo.Get(providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Get target CLI with mutator info
	cli, err := uc.cliRepo.Get(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("get target cli: %w", err)
	}

	// Get active multiplex for model mappings
	activeMX, err := uc.multiplexRepo.GetActive(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("get active multiplex: %w", err)
	}
	if activeMX.TargetCLIID == 0 {
		return nil, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}

	// Parse model mappings JSON
	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(activeMX.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}

	// Resolve config path
	resolvedPath, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	// Parse mutator_config JSON
	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg); err != nil {
			return nil, fmt.Errorf("parse mutator config for CLI '%s': %w", cli.Name, err)
		}
	}

	// Resolve mutator name (fallback for legacy rows)
	mutatorName := cli.Mutator
	if mutatorName == "" {
		mutatorName = "claude-settings-json"
	}

	// Look up mutator from registry
	mutator, ok := uc.mutators[mutatorName]
	if !ok {
		return nil, fmt.Errorf("mutator '%s' not registered for CLI '%s'", mutatorName, cli.Name)
	}

	// Extract _registered models from the mappings (comma-separated list) and remove
	// the key so it doesn't interfere with env-var-to-model mapping.
	if registeredStr, ok := mappings["_registered"]; ok && registeredStr != "" {
		delete(mappings, "_registered")
		parts := strings.Split(registeredStr, ",")
		registered := make([]string, 0, len(parts))
		for _, p := range parts {
			if p != "" {
				registered = append(registered, p)
			}
		}
		if len(registered) > 0 {
			mutatorCfg["_registered_models"] = registered
		}
	}

	// Inject model metadata for mutators that need it (pi, claude [1m] suffix, etc.)
	models, _ := uc.providerRepo.ListModels(providerID)
	if len(models) > 0 {
		modelMeta := make(map[string]any)
		for _, m := range models {
			if len(m.Metadata) > 0 {
				modelMeta[m.ModelName] = m.Metadata
			}
		}
		if len(modelMeta) > 0 {
			mutatorCfg["_model_metadata"] = modelMeta
		}
	}

	return mutator.Mutate(resolvedPath, mappings, provider, mutatorCfg)
}

// DryRunResult holds the information about what Apply would do without executing it.
type DryRunResult struct {
	CLIName    string
	ConfigPath string
	EnvVars    map[string]string
}

// DryRun simulates what Apply would write, returning the target info without mutating.
func (uc *SwitchUseCases) DryRun(targetCLIID, providerID int64) (*DryRunResult, error) {
	cli, err := uc.cliRepo.Get(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("get target cli: %w", err)
	}

	provider, err := uc.providerRepo.Get(providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	activeMX, err := uc.multiplexRepo.GetActive(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("get active multiplex: %w", err)
	}
	if activeMX.TargetCLIID == 0 {
		return nil, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(activeMX.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}

	resolvedPath, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	_ = provider // provider is used by the mutator, not needed for dry-run display
	return &DryRunResult{
		CLIName:    cli.Name,
		ConfigPath: resolvedPath,
		EnvVars:    mappings,
	}, nil
}

// ListTargetCLIs returns all registered target CLIs.
func (uc *SwitchUseCases) ListTargetCLIs() ([]domain.TargetCLI, error) {
	return uc.cliRepo.List()
}

// UpdateCLIConfigPath updates a target CLI's config path.
func (uc *SwitchUseCases) UpdateCLIConfigPath(id int64, configPath string) error {
	cli, err := uc.cliRepo.Get(id)
	if err != nil {
		return fmt.Errorf("get target cli %d: %w", id, err)
	}
	cli.ConfigPath = configPath
	return uc.cliRepo.Update(cli)
}

// GetActiveForCLI returns the active multiplex for a given target CLI.
func (uc *SwitchUseCases) GetActiveForCLI(targetCLIID int64) (*domain.ActiveMultiplex, error) {
	am, err := uc.multiplexRepo.GetActive(targetCLIID)
	if err != nil {
		return nil, err
	}
	if am.TargetCLIID == 0 {
		return nil, nil
	}
	return &am, nil
}

// BindProfile validates and stores a profile binding.
func (uc *SwitchUseCases) BindProfile(targetCLIID, providerID int64, mappings map[string]string) error {
	// Get target CLI to validate env vars
	clis, err := uc.cliRepo.List()
	if err != nil {
		return fmt.Errorf("list target clis: %w", err)
	}

	var targetCLI *domain.TargetCLI
	for i := range clis {
		if clis[i].ID == targetCLIID {
			targetCLI = &clis[i]
			break
		}
	}
	if targetCLI == nil {
		return fmt.Errorf("target CLI %d not found", targetCLIID)
	}

	// Parse known env vars
	var knownVars []string
	if err := json.Unmarshal([]byte(targetCLI.EnvVars), &knownVars); err != nil {
		return fmt.Errorf("parse env vars: %w", err)
	}

	// Build a set of known env vars
	knownSet := make(map[string]bool, len(knownVars))
	for _, v := range knownVars {
		knownSet[v] = true
	}

	// Validate all mapping keys are in the known set
	for key := range mappings {
		if !knownSet[key] {
			return fmt.Errorf("unknown env var '%s' for target CLI '%s'", key, targetCLI.Name)
		}
	}

	// Get provider and validate status
	provider, err := uc.providerRepo.Get(providerID)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}
	if provider.Status != "active" {
		return fmt.Errorf("provider '%s' is not active (status: %s)", provider.Name, provider.Status)
	}

	// Get provider models for validation
	models, err := uc.providerRepo.ListModels(providerID)
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	modelSet := make(map[string]bool, len(models))
	for _, m := range models {
		modelSet[m.ModelName] = true
	}

	// Validate each non-empty model ID exists in provider_models
	for key, modelName := range mappings {
		if modelName != "" && !modelSet[modelName] {
			return fmt.Errorf("model '%s' not found for this provider (env var: %s)", modelName, key)
		}
	}

	// Marshal mappings to JSON
	mappingsJSON, err := json.Marshal(mappings)
	if err != nil {
		return fmt.Errorf("marshal mappings: %w", err)
	}

	// Store active multiplex
	if err := uc.multiplexRepo.SetActive(targetCLIID, providerID, string(mappingsJSON)); err != nil {
		return fmt.Errorf("set active multiplex: %w", err)
	}

	return nil
}

// GetBoundModels returns the current model mappings for a given CLI.
func (uc *SwitchUseCases) GetBoundModels(targetCLIID int64) (map[string]string, error) {
	am, err := uc.multiplexRepo.GetActive(targetCLIID)
	if err != nil {
		return nil, err
	}
	if am.TargetCLIID == 0 {
		// No active multiplex — return empty map, no error
		return make(map[string]string), nil
	}

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(am.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}
	return mappings, nil
}

// GetProviderForCLI returns the provider ID bound to the given CLI.
func (uc *SwitchUseCases) GetProviderForCLI(targetCLIID int64) (int64, error) {
	am, err := uc.multiplexRepo.GetActive(targetCLIID)
	if err != nil {
		return 0, err
	}
	if am.TargetCLIID == 0 {
		return 0, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}
	return am.ProviderID, nil
}

// ListActiveMultiplexes returns all active multiplexes with joined data.
func (uc *SwitchUseCases) ListActiveMultiplexes() ([]domain.ActiveMultiplex, error) {
	return uc.multiplexRepo.ListActive()
}

// GetModelsForProvider returns all models for a given provider.
func (uc *SwitchUseCases) GetModelsForProvider(providerID int64) ([]domain.ProviderModel, error) {
	return uc.providerRepo.ListModels(providerID)
}

// ListAllModels returns all models across all providers.
func (uc *SwitchUseCases) ListAllModels() ([]domain.ProviderModel, error) {
	return uc.providerRepo.ListAllModels()
}

// FindCLIByName resolves a target CLI by (case-insensitive) name.
func (uc *SwitchUseCases) FindCLIByName(name string) (*domain.TargetCLI, error) {
	clis, err := uc.cliRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list CLIs: %w", err)
	}
	for i := range clis {
		if strings.EqualFold(clis[i].Name, name) {
			return &clis[i], nil
		}
	}
	return nil, fmt.Errorf("CLI '%s' not found", name)
}

// ListBackups returns the centralized backups for a target CLI's config file,
// newest first.
func (uc *SwitchUseCases) ListBackups(cliName string) ([]config.BackupEntry, error) {
	cli, err := uc.FindCLIByName(cliName)
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return nil, err
	}
	return config.ListBackups(resolved)
}

// RestoreLatest restores the most recent centralized backup for a target CLI's
// config file. Returns the backup path that was restored.
func (uc *SwitchUseCases) RestoreLatest(cliName string) (string, error) {
	cli, err := uc.FindCLIByName(cliName)
	if err != nil {
		return "", err
	}
	resolved, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return "", err
	}
	backups, err := config.ListBackups(resolved)
	if err != nil {
		return "", err
	}
	if len(backups) == 0 {
		return "", fmt.Errorf("no backups for '%s'", cliName)
	}
	latest := backups[0] // ListBackups is newest-first
	if err := config.RestoreBackup(latest.Path, resolved); err != nil {
		return "", fmt.Errorf("restore backup: %w", err)
	}
	return latest.Path, nil
}

// BackupOption is a presentation-friendly backup entry for the TUI.
type BackupOption struct {
	Label string // timestamp segment, e.g. "2026-06-18T03-21-00Z"
	Path  string // absolute path to the backup file
}

// BackupOptions returns centralized backups for a CLI as display options,
// newest first. Empty (no error) when there are none.
func (uc *SwitchUseCases) BackupOptions(cliName string) ([]BackupOption, error) {
	entries, err := uc.ListBackups(cliName)
	if err != nil {
		return nil, err
	}
	opts := make([]BackupOption, len(entries))
	for i, e := range entries {
		opts[i] = BackupOption{Label: e.When, Path: e.Path}
	}
	return opts, nil
}

// RestoreBackup restores a specific backup file for a target CLI's config.
func (uc *SwitchUseCases) RestoreBackup(cliName, backupPath string) error {
	cli, err := uc.FindCLIByName(cliName)
	if err != nil {
		return err
	}
	resolved, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return err
	}
	return config.RestoreBackup(backupPath, resolved)
}
