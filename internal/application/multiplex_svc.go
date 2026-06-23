package application

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/MileniumTick/aimux/internal/infrastructure/mutators"
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

// Apply activates all bound providers and mutates the target CLI's config file.
// For multi-provider CLIs (pi, OpenCode), each provider is applied separately —
// the mutator reads the existing config and adds/replaces its provider entry.
func (uc *SwitchUseCases) Apply(targetCLIID, providerID int64) (*domain.BackupResult, error) {
	cli, err := uc.cliRepo.Get(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("get target cli: %w", err)
	}

	// Collect all active multiplexes for this CLI (supports multi-provider)
	allMux, err := uc.multiplexRepo.ListForCLI(targetCLIID)
	if err != nil {
		return nil, fmt.Errorf("list multiplexes for CLI: %w", err)
	}
	if len(allMux) == 0 {
		return nil, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}

	// If a specific providerID was requested, filter to that one;
	// otherwise apply all bound providers.
	if providerID != 0 {
		filtered := make([]domain.ActiveMultiplex, 0, 1)
		for _, m := range allMux {
			if m.ProviderID == providerID {
				filtered = append(filtered, m)
				break
			}
		}
		if len(filtered) == 0 {
			return nil, fmt.Errorf("provider %d not bound to CLI %s", providerID, cli.Name)
		}
		allMux = filtered
	}

	// Resolve config path once (shared across all providers)
	resolvedPath, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}

	// Parse mutator_config JSON once
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

	// Apply each binding — mutators are idempotent, so multiple calls are safe.
	// pi/OpenCode mutators read-modify-write, each call adds/replaces its own provider entry.
	// The first call clears the provider map so deleted bindings don't leave stale entries.
	var lastResult *domain.BackupResult
	for i, mux := range allMux {
		// Get provider
		provider, err := uc.providerRepo.Get(mux.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("get provider %d: %w", mux.ProviderID, err)
		}

		// Parse model mappings JSON
		mappings := make(map[string]string)
		if err := json.Unmarshal([]byte(mux.ModelMappings), &mappings); err != nil {
			return nil, fmt.Errorf("parse model mappings: %w", err)
		}

		// Per-provider mutator config (clone to avoid cross-contamination)
		cfg := make(map[string]any, len(mutatorCfg)+2)
		for k, v := range mutatorCfg {
			cfg[k] = v
		}

		// For pi/OpenCode mutators, use provider name as the entry key so each
		// binding gets its own entry. For env-mapping CLIs (Claude, Codex), keep
		// the original provider_id since they use it for env key names.
		if cli.Mutator == "pi-dual-json" || cli.Mutator == "opencode-provider-json" {
			cfg["provider_id"] = strings.ToLower(strings.ReplaceAll(provider.Name, " ", "-"))
		}

		// First binding clears the provider map so deleted bindings don't leave
		// stale entries in the config file.
		if i == 0 {
			cfg["_clear_providers"] = true
		}

		// Extract _registered models from mappings
		if registeredStr, ok := mappings["_registered"]; ok && registeredStr != "" {
			parts := strings.Split(registeredStr, ",")
			registered := make([]string, 0, len(parts))
			for _, p := range parts {
				if p != "" {
					registered = append(registered, p)
				}
			}
			if len(registered) > 0 {
				cfg["_registered_models"] = registered
			}
			delete(mappings, "_registered")
		}

		// Inject per-provider model metadata
		models, _ := uc.providerRepo.ListModels(mux.ProviderID)
		if len(models) > 0 {
			modelMeta := make(map[string]any)
			for _, m := range models {
				if len(m.Metadata) > 0 {
					modelMeta[m.ModelName] = map[string]any(m.Metadata)
				} else if provider.DefaultContextWindow > 0 {
					// Fallback: model sin metadata pero provider tiene default
					modelMeta[m.ModelName] = map[string]any(domain.ModelMetadata{
						domain.MetaContextWindow: provider.DefaultContextWindow,
						domain.MetaContextSuffix: config.ContextSuffixForWindow(provider.DefaultContextWindow),
					})
				}
			}
			if len(modelMeta) > 0 {
				cfg["_model_metadata"] = modelMeta
			}
		}

		result, err := mutator.Mutate(resolvedPath, mappings, provider, cfg)
		if err != nil {
			return nil, fmt.Errorf("apply provider %s: %w", provider.Name, err)
		}
		if result != nil {
			lastResult = result
		}
	}

	return lastResult, nil
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

	allMux, err := uc.multiplexRepo.ListForCLI(targetCLIID)
	if err != nil || len(allMux) == 0 {
		return nil, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}

	// Filter by providerID when specified (same as Apply does)
	muxes := allMux
	if providerID != 0 {
		muxes = nil
		for _, m := range allMux {
			if m.ProviderID == providerID {
				muxes = append(muxes, m)
				break
			}
		}
		if len(muxes) == 0 {
			return nil, fmt.Errorf("provider %d not bound to CLI %s", providerID, cli.Name)
		}
	}

	// Collect all env vars from the filtered bindings
	allMappings := make(map[string]string)
	for _, mux := range muxes {
		m := make(map[string]string)
		if err := json.Unmarshal([]byte(mux.ModelMappings), &m); err != nil {
			return nil, fmt.Errorf("parse model mappings: %w", err)
		}
		for k, v := range m {
			allMappings[k] = v
		}
	}

	// For CLIs with auto-detected paths (copilot-shell-profile), show the
	// detected shell profile path instead of an empty config path.
	resolvedPath := cli.ConfigPath
	if resolvedPath == "" {
		resolvedPath = mutators.ShellProfilePath()
		if resolvedPath == "" {
			resolvedPath = "(shell profile — auto-detected)"
		}
	} else {
		var rErr error
		resolvedPath, rErr = ResolveTargetConfigPath(cli.ConfigPath)
		if rErr != nil {
			return nil, fmt.Errorf("resolve config path: %w", rErr)
		}
	}

	return &DryRunResult{
		CLIName:    cli.Name,
		ConfigPath: resolvedPath,
		EnvVars:    allMappings,
	}, nil
}

// ListTargetCLIs returns all registered target CLIs.
func (uc *SwitchUseCases) ListTargetCLIs() ([]domain.TargetCLI, error) {
	return uc.cliRepo.List()
}

// UpdateCLIConfig updates a CLI's config path, mutator_config, and optionally
// sets binary_path in mutator_config.
func (uc *SwitchUseCases) UpdateCLIConfig(id int64, configPath, mutatorConfig, binaryPath string) error {
	cli, err := uc.cliRepo.Get(id)
	if err != nil {
		return fmt.Errorf("get target cli %d: %w", id, err)
	}
	cli.ConfigPath = configPath

	// Merge binary_path into mutator_config
	if mutatorConfig != "" && mutatorConfig != "{}" {
		cli.MutatorConfig = mutatorConfig
	}
	if binaryPath != "" {
		var mc map[string]any
		if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
			json.Unmarshal([]byte(cli.MutatorConfig), &mc)
		}
		if mc == nil {
			mc = make(map[string]any)
		}
		mc["binary_path"] = binaryPath
		if data, err := json.Marshal(mc); err == nil {
			cli.MutatorConfig = string(data)
		}
	}
	return uc.cliRepo.Update(cli)
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

	// Validate all mapping keys are in the known set.
	// Keys starting with _ are metadata (like _registered) and bypass env var validation.
	for key := range mappings {
		if strings.HasPrefix(key, "_") {
			continue
		}
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
		if strings.HasPrefix(key, "_") {
			continue // metadata keys like _registered store lists, not model names
		}
		if modelName != "" && !modelSet[modelName] {
			return fmt.Errorf("model '%s' not found for this provider (env var: %s)", modelName, key)
		}
	}

	// Marshal mappings to JSON
	mappingsJSON, err := json.Marshal(mappings)
	if err != nil {
		return fmt.Errorf("marshal mappings: %w", err)
	}

	// For single-provider CLIs, clear all existing bindings before setting the
	// new one so stale bindings don't contaminate Apply or DryRun results.
	isMultiProvider := targetCLI.Mutator == "pi-dual-json" || targetCLI.Mutator == "opencode-provider-json"
	if !isMultiProvider {
		if err := uc.multiplexRepo.ClearActive(targetCLIID); err != nil {
			return fmt.Errorf("clear stale bindings: %w", err)
		}
	}

	// Store active multiplex
	if err := uc.multiplexRepo.SetActive(targetCLIID, providerID, string(mappingsJSON)); err != nil {
		return fmt.Errorf("set active multiplex: %w", err)
	}

	return nil
}

// ListBindingsForCLI returns all active multiplex rows for a given CLI.
func (uc *SwitchUseCases) ListBindingsForCLI(targetCLIID int64) ([]domain.ActiveMultiplex, error) {
	return uc.multiplexRepo.ListForCLI(targetCLIID)
}

// RemoveBinding removes a specific provider binding for a CLI.
func (uc *SwitchUseCases) RemoveBinding(targetCLIID, providerID int64) error {
	all, err := uc.multiplexRepo.ListForCLI(targetCLIID)
	if err != nil {
		return err
	}
	for _, b := range all {
		if b.ProviderID == providerID {
			return uc.multiplexRepo.ClearBinding(targetCLIID, providerID)
		}
	}
	return fmt.Errorf("binding for provider %d not found", providerID)
}

// ClearCLIConfig removes all custom provider entries from the config file
// without wiping other settings. For copilot-shell-profile, removes the
// managed env var block from the user's shell profile.
func (uc *SwitchUseCases) ClearCLIConfig(targetCLIID int64) error {
	cli, err := uc.cliRepo.Get(targetCLIID)
	if err != nil {
		return fmt.Errorf("get cli: %w", err)
	}

	resolvedPath, err := ResolveTargetConfigPath(cli.ConfigPath)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Read existing config, clear only the provider map, write back
	root, err := config.ReadJSONWithLock(resolvedPath)
	if err != nil {
		root = make(map[string]any)
	}

	switch cli.Mutator {
	case "pi-dual-json":
		root["providers"] = map[string]any{}
	case "opencode-provider-json":
		root["provider"] = map[string]any{}
	case "copilot-shell-profile":
		// Remove the aimux-managed block from the user's shell profile
		if err := mutators.RemoveShellEnvBlock(); err != nil {
			return fmt.Errorf("remove shell env block: %w", err)
		}
		return nil
	default:
		return nil
	}

	return config.WriteAtomicJSON(resolvedPath, root)
}

// GetProviderForCLI returns the first provider ID bound to the given CLI.
// For multi-provider setups, use ListBindingsForCLI for all providers.
func (uc *SwitchUseCases) GetProviderForCLI(targetCLIID int64) (int64, error) {
	all, err := uc.multiplexRepo.ListForCLI(targetCLIID)
	if err != nil {
		return 0, err
	}
	if len(all) == 0 {
		return 0, fmt.Errorf("no active multiplex for target CLI %d", targetCLIID)
	}
	return all[0].ProviderID, nil
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
	cli := domain.FindCLIByName(clis, name)
	if cli == nil {
		return nil, fmt.Errorf("CLI '%s' not found", name)
	}
	return cli, nil
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
