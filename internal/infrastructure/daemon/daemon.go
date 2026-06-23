package daemon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
)

// ResolveViaDB resolves env vars for a CLI by querying the database directly.
// Used as fallback when the daemon is not running.
func ResolveViaDB(db *sql.DB, cliName string) (*ResolveResult, error) {
	providerRepo := &sqlite.ProviderRepository{DB: db}
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}

	// Find CLI
	clis, err := cliRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list CLIs: %w", err)
	}
	cli := domain.FindCLIByName(clis, cliName)
	if cli == nil {
		return nil, fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Get active bindings
	bindings, err := multiplexRepo.ListForCLI(cli.ID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("no active binding for '%s'. Use the TUI to set one up first", cliName)
	}

	binding := bindings[0]

	provider, err := providerRepo.Get(binding.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(binding.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}

	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg); err != nil {
			return nil, fmt.Errorf("parse mutator config: %w", err)
		}
	}

	var envVarNames []string
	if cli.EnvVars != "" {
		json.Unmarshal([]byte(cli.EnvVars), &envVarNames)
	}
	mutatorCfg["_env_var_names"] = envVarNames
	mutatorCfg["_mutator_name"] = cli.Mutator

	return resolveEnvVars(cliName, provider, mappings, mutatorCfg)
}

// cliBinaries maps known CLI names to their default binary names.
var cliBinaries = map[string]string{
	"claude-code":    "claude",
	"opencode":       "opencode",
	"codex":          "codex",
	"github-copilot": "github-copilot",
	"pi-ai":          "pi",
}

// ResolveBinary resolves the binary path for a CLI name.
// First checks if the CLI has a custom binary_path in its mutator_config,
// then falls back to the known binary name map, then tries PATH lookup.
func ResolveBinary(db *sql.DB, cliName string) (string, error) {
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	clis, err := cliRepo.List()
	if err != nil {
		return "", fmt.Errorf("list CLIs: %w", err)
	}
	cli := domain.FindCLIByName(clis, cliName)
	if cli == nil {
		return "", fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Check custom binary path in mutator_config
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &cfg); err == nil {
			if bp, ok := cfg["binary_path"].(string); ok && bp != "" {
				if expanded, err := application.ExpandTilde(bp); err == nil {
					if _, err := os.Stat(expanded); err == nil {
						return expanded, nil
					}
				}
			}
		}
	}

	// Try known binary name
	binName, known := cliBinaries[cliName]
	if !known {
		binName = cliName
	}

	// Look up in PATH
	binary, err := exec.LookPath(binName)
	if err != nil {
		return "", fmt.Errorf("binary '%s' not found in PATH for '%s'. Install it or set binary_path in CLI config", binName, cliName)
	}
	return binary, nil
}

// RunCLI resolves credentials for a CLI and launches its binary.
// If providerName is set, uses that provider instead of the bound one.
// models is JSON: either {"ENV_VAR":"model",...} (env mapping) or ["model1","model2",...] (registered).
// reasoning is the reasoning level: "off", "low", "medium", "high", "max".
func RunCLI(db *sql.DB, cliName, providerName, models, reasoning string, mutatorRegistry map[string]domain.ConfigMutator) error {
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	providerRepo := &sqlite.ProviderRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}

	clis, err := cliRepo.List()
	if err != nil {
		return fmt.Errorf("list CLIs: %w", err)
	}
	cli := domain.FindCLIByName(clis, cliName)
	if cli == nil {
		return fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Resolve provider: by name (launch) or from binding (run with binding)
	var provider domain.Provider
	mappings := make(map[string]string)

	if providerName != "" {
		// Look up provider by name (from Launch flow)
		allProviders, err := providerRepo.List()
		if err != nil {
			return fmt.Errorf("list providers: %w", err)
		}
		var found bool
		for _, p := range allProviders {
			if strings.EqualFold(p.Name, providerName) {
				provider = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("provider '%s' not found", providerName)
		}
		// Parse models JSON: if it's an object ({"ENV":"model",...}), use as mappings.
		// If it's an array (["m1","m2",...]), set _registered_models later.
		if models != "" {
			if err := json.Unmarshal([]byte(models), &mappings); err != nil {
				// Not an object, might be an array — handled later via _registered_models
				mappings = make(map[string]string)
			}
		}
	} else {
		// Use binding from DB
		bindings, err := multiplexRepo.ListForCLI(cli.ID)
		if err != nil {
			return fmt.Errorf("list bindings: %w", err)
		}
		if len(bindings) == 0 {
			return fmt.Errorf("no active binding for '%s'. Use the TUI to set one up first", cliName)
		}

		binding := bindings[0]
		provider, err = providerRepo.Get(binding.ProviderID)
		if err != nil {
			return fmt.Errorf("get provider: %w", err)
		}

		if err := json.Unmarshal([]byte(binding.ModelMappings), &mappings); err != nil {
			mappings = make(map[string]string)
		}
	}

	configPath := cli.ConfigPath
	if strings.HasPrefix(configPath, "~") {
		if hd, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(hd, configPath[1:])
		}
	}

	// ── Backup config + write via mutator (non-env-var CLIs) ─────────
	// Claude Code supports env var auth — no file mutation at all.
	// For opencode/pi/codex/copilot, backup → write → launch → restore.
	var backupPath string
	var fileExisted bool
	if cli.Mutator != "" && configPath != "" {
		if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
			fileExisted = true
			if bp, err := config.CreateBackup(configPath); err == nil {
				backupPath = bp
				log.Printf("backed up %s", filepath.Base(bp))
			}
		}
		defer func() {
			if backupPath != "" {
				if err := config.RestoreBackup(backupPath, configPath); err != nil {
					log.Printf("warning: restore failed: %v", err)
				} else {
					log.Printf("restored config from backup")
				}
			} else if !fileExisted {
				if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
					log.Printf("warning: failed to remove created config: %v", err)
				} else {
					log.Printf("removed config file created by launch")
				}
			}
		}()

		if m, ok := mutatorRegistry[cli.Mutator]; ok {
			applyCfg := make(map[string]any)
			if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
				json.Unmarshal([]byte(cli.MutatorConfig), &applyCfg)
			}
			applyCfg["_clear_providers"] = true
			applyCfg["provider_id"] = strings.ToLower(strings.ReplaceAll(provider.Name, " ", "-"))
			// Parse models JSON for registered_models (array format)
			if models != "" && strings.HasPrefix(models, "[") {
				var regModels []string
				if err := json.Unmarshal([]byte(models), &regModels); err == nil && len(regModels) > 0 {
					applyCfg["_registered_models"] = regModels
				}
			}
			models, _ := providerRepo.ListModels(provider.ID)
			if len(models) > 0 {
				modelMeta := make(map[string]any)
				for _, model := range models {
					if len(model.Metadata) > 0 {
						modelMeta[model.ModelName] = map[string]any(model.Metadata)
					}
				}
				if len(modelMeta) > 0 {
					applyCfg["_model_metadata"] = modelMeta
				}
			}
			if _, err := m.Mutate(configPath, mappings, provider, applyCfg); err != nil {
				log.Printf("warning: config write failed: %v", err)
			}
		}
	}

	// ── Inject env vars from .env file (for codex) ──────────────────
	// Some mutators (codex) write a .env alongside the config. The agent
	// expects those env vars in its environment.
	dotEnv := make(map[string]string)
	if configPath != "" {
		envDir := filepath.Dir(configPath)
		if data, err := os.ReadFile(filepath.Join(envDir, ".env")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				eq := strings.IndexByte(line, '=')
				if eq > 0 {
					if k, v := line[:eq], line[eq+1:]; k != "" && v != "" {
						dotEnv[k] = v
					}
				}
			}
		}
	}

	// Resolve env vars
	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg)
	}
	var envVarNames []string
	if cli.EnvVars != "" {
		json.Unmarshal([]byte(cli.EnvVars), &envVarNames)
	}
	mutatorCfg["_env_var_names"] = envVarNames
	mutatorCfg["_mutator_name"] = cli.Mutator

	// Inject model metadata for context suffix resolution
	provModels, _ := providerRepo.ListModels(provider.ID)
	if len(provModels) > 0 {
		modelMeta := make(map[string]any)
		for _, m := range provModels {
			if len(m.Metadata) > 0 {
				modelMeta[m.ModelName] = map[string]any(m.Metadata)
			}
		}
		if len(modelMeta) > 0 {
			mutatorCfg["_model_metadata"] = modelMeta
		}
	}

	// ── Inject reasoning level ──────────────────────────────────────
	if reasoning != "" && cli.Mutator == "claude-settings-json" {
		mutatorCfg["_reasoning_level"] = reasoning
		mutatorCfg["extra_env_disabled"] = false
		extra := make(map[string]any)
		if e, ok := mutatorCfg["extra_env"].(map[string]any); ok {
			extra = e
		}
		extra["CLAUDE_CODE_EFFORT_LEVEL"] = reasoning
		mutatorCfg["extra_env"] = extra
	}

	result, err := resolveEnvVars(cliName, provider, mappings, mutatorCfg)
	if err != nil {
		return fmt.Errorf("resolve env: %w", err)
	}

	// ── Resolve binary ─────────────────────────────────────────────────
	binary, err := ResolveBinary(db, cliName)
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	log.Printf("run %s → %s (%s): %d env vars", cliName, result.ProviderName, binary, len(result.Env))

	// ── Build env (our vars first, then non-conflicting inherited) ──────
	env := make([]string, 0, len(result.Env)+len(os.Environ()))
	for k, v := range result.Env {
		env = append(env, k+"="+v)
	}
	managed := make(map[string]bool, len(result.Env))
	for k := range result.Env {
		managed[k] = true
	}
	for _, e := range os.Environ() {
		eq := strings.IndexByte(e, '=')
		if eq > 0 {
			key := e[:eq]
			if !managed[key] {
				env = append(env, e)
			}
		}
	}

	// ── Inject .env vars (from mutator-written .env files) ──────────
	for k, v := range dotEnv {
		if !managed[k] {
			env = append(env, k+"="+v)
			managed[k] = true
		}
	}

	// ── Launch agent as subprocess ──────────────────────────────────────
	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("run %s: %w", binary, err)
	}

	return nil
}


