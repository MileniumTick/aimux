# Config Mutators — Technical Design

## Architecture Overview

Refactor the monolithic `config.MutateAndWrite` into a `domain.ConfigMutator` interface with a registry pattern. Each target CLI gets its own mutator implementation. The application layer (`SwitchUseCases`) resolves mutators by name — zero format-specific logic.

```
main.go
  └─ registers mutators in map[string]ConfigMutator
     ├─ "claude-settings-json"   → ClaudeSettingsJSON{shared: utils}
     ├─ "opencode-provider-json" → OpenCodeProviderJSON{shared: utils}
     ├─ "codex-config-toml"      → CodexConfigTOML{shared: utils}
     ├─ "copilot-env-file"       → CopilotEnvFile{shared: utils}
     └─ "pi-dual-json"           → PiDualJSON{shared: utils}
           │
           └─ SwitchUseCases.Apply()
                ├─ cliRepo.Get(targetCLIID) → TargetCLI{mutator, mutator_config}
                ├─ parse mutator_config JSON → map[string]any
                ├─ mutator, ok := mutators[targetCLI.Mutator]
                └─ mutator.Mutate(configPath, mappings, provider, mutatorConfig)
```

## Package Structure

```
internal/
  domain/
    provider.go          — Provider, ProviderModel, ProviderRepository (unchanged)
    targetcli.go         — +Mutator, +MutatorConfig fields, +ConfigMutator interface, +BackupResult
    multiplex.go         — ActiveMultiplex, MultiplexRepository (unchanged)
  application/
    switch_svc.go        — +mutators map[string]ConfigMutator in constructor, updated Apply()
    provider_svc.go      — (unchanged)
    path.go              — (unchanged)
  infrastructure/
    config/
      utils.go           — Shared: flock, atomic write, backup, prune (extracted from settings.go)
    mutators/
      claude_json.go     — "claude-settings-json" mutator (refactored from settings.go)
      opencode_json.go   — "opencode-provider-json" mutator
      codex_toml.go      — "codex-config-toml" mutator
      copilot_env.go     — "copilot-env-file" mutator
      pi_dual.go         — "pi-dual-json" mutator
    sqlite/
      db.go              — +MigrationAddMutatorColumns()
      targetcli_repo.go  — +Mutator, +MutatorConfig in scan
      (others unchanged)
  tui/
    model.go             — Updated confirmation view to show backup path + mutator-specific info
    (others unchanged)
```

## ConfigMutator Interface

```go
// domain/targetcli.go

type BackupResult struct {
    BackupPath string
}

type ConfigMutator interface {
    Mutate(
        configPath     string,            // resolved path from target_clis.config_path
        modelMappings  map[string]string,  // env_var → model_name
        provider       Provider,          // full provider record (name, base_url, api_key, auth_token)
        mutatorConfig  map[string]any,     // parsed from target_clis.mutator_config JSON
    ) (*BackupResult, error)
}
```

## Schema Migration

```sql
-- Add to RunMigrations() or as separate migration step
ALTER TABLE target_clis ADD COLUMN mutator TEXT NOT NULL DEFAULT 'claude-settings-json';
ALTER TABLE target_clis ADD COLUMN mutator_config TEXT NOT NULL DEFAULT '{}';
```

## Updated TargetCLI Struct

```go
type TargetCLI struct {
    ID            int64
    Name          string
    ConfigPath    string
    EnvVars       string
    Mutator       string  // registry key
    MutatorConfig string  // JSON object
}
```

## Updated SeedTargetCLIs

```sql
INSERT OR IGNORE INTO target_clis (name, config_path, env_vars, mutator, mutator_config)
VALUES ('claude-code',
  '~/.config/claude/settings.json',
  '["ANTHROPIC_DEFAULT_HAIKU_MODEL","ANTHROPIC_DEFAULT_SONNET_MODEL","ANTHROPIC_DEFAULT_OPUS_MODEL","CLAUDE_CODE_SUBAGENT_MODEL"]',
  'claude-settings-json',
  '{}');
```

## SwitchUseCases.Apply — Refactored Flow

```go
func (uc *SwitchUseCases) Apply(targetCLIID, providerID int64) (*domain.BackupResult, error) {
    provider, err := uc.providerRepo.Get(providerID)
    cli, err := uc.cliRepo.Get(targetCLIID)        // new: Get method
    activeMX, err := uc.multiplexRepo.GetActive(targetCLIID)
    
    mappings := parseJSON(activeMX.ModelMappings)
    
    resolvedPath, _ := ResolveTargetConfigPath(cli.ConfigPath)
    
    // Parse mutator_config
    var mutatorCfg map[string]any
    if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
        json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg)
    }
    if mutatorCfg == nil {
        mutatorCfg = make(map[string]any)
    }
    
    // Mutator fallback for legacy rows
    mutatorName := cli.Mutator
    if mutatorName == "" {
        mutatorName = "claude-settings-json"
    }
    
    mutator, ok := uc.mutators[mutatorName]
    if !ok {
        return nil, fmt.Errorf("mutator '%s' not registered for CLI '%s'", mutatorName, cli.Name)
    }
    
    return mutator.Mutate(resolvedPath, mappings, provider, mutatorCfg)
}
```

## Shared Utilities (config/utils.go)

Extracted from `config/settings.go` — all mutators use these:

```go
func ReadJSONWithLock(path string) (map[string]any, error)
func WriteAtomicJSON(path string, data map[string]any) error
func CreateBackup(path string) (string, error)
func PruneBackups(dir, prefix string, max int)
func AcquireFlock(fd uintptr, lockType int, timeout time.Duration) error
```

## Mutator Implementations

### 1. ClaudeSettingsJSON (`infrastructure/mutators/claude_json.go`)

**Registered as**: `"claude-settings-json"`

**Logic**:
1. Read JSON with shared lock
2. Delete `ANTHROPIC_API_KEY` from root
3. Build `env` map from modelMappings (skip empty values)
4. Add `env["ANTHROPIC_API_KEY"]` = provider.APIKey
5. Set `root["env"]` = env
6. Backup + atomic write

**~20 lines of unique logic** (rest is shared utils).

### 2. OpenCodeProviderJSON (`infrastructure/mutators/opencode_json.go`)

**Registered as**: `"opencode-provider-json"`

**mutator_config fields**:
- `provider_id` (required): key under `provider`
- `npm` (required): AI SDK package name

**Logic**:
1. Read JSON with shared lock
2. Ensure `provider` key exists in root
3. Build provider entry: npm, name (from provider.Name), options.baseURL, options.apiKey
4. Build models map: each model mapping → `{name: <modelName>}`
5. Set `root["provider"][providerID]` = provider entry
6. Backup + atomic write

**~30 lines of unique logic**.

### 3. CodexConfigTOML (`infrastructure/mutators/codex_toml.go`)

**Registered as**: `"codex-config-toml"`

**Dependency**: `github.com/BurntSushi/toml`

**mutator_config fields**:
- `provider_id` (required): key under `[model_providers]`
- `wire_api` (optional): wire protocol

**Logic**:
1. Read existing TOML (or empty struct)
2. Set top-level `model` = first non-empty model mapping value
3. Set `model_provider` = providerID
4. Build `[model_providers.<id>]` section with name, base_url, env_key
5. Write API key to `~/.codex/.env` file (not in TOML)
6. Marshal TOML back, backup + atomic write

**~35 lines of unique logic**.

### 4. CopilotEnvFile (`infrastructure/mutators/copilot_env.go`)

**Registered as**: `"copilot-env-file"`

**mutator_config fields**:
- `provider_type` (optional): `"openai"` (default), `"azure"`, `"anthropic"`
- `local` (optional): if true, skip API key

**Logic**:
1. Build env var lines from modelMappings + provider
2. Write to `.env` file (dir from config_path or `~/.config/copilot/`)
3. Backup existing .env if present
4. No JSON/TOML parsing needed — just key=value lines

**~25 lines of unique logic**.

### 5. PiDualJSON (`infrastructure/mutators/pi_dual.go`)

**Registered as**: `"pi-dual-json"`

**mutator_config fields**:
- `provider_id` (required): key under `providers`
- `provider_type` (required): e.g. `"openai-compatible"`
- `models_path` (optional): defaults to `~/.config/pi/models.json`
- `auth_path` (optional): defaults to `~/.config/pi/auth.json`

**Logic**:
1. Read models.json with shared lock
2. Build provider entry under `providers[providerID]` with type, base_url, models
3. Backup + atomic write models.json
4. Read auth.json with shared lock
5. Set `auth[providerID]` = `{type: "api_key", key: provider.APIKey}`
6. Backup + atomic write auth.json

**~40 lines of unique logic** (two files).

## Error Handling

| Error | Handling |
|-------|----------|
| Mutator not in registry | `""mutator X not registered for CLI Y""` — returned to TUI |
| mutator_config invalid JSON | `"parse mutator config: ..."` — returned to TUI |
| File locked (flock timeout) | `"could not acquire file lock"` — retryable, shown to user |
| Config file not found | Create new (all mutators handle missing files) |
| Invalid config JSON/TOML | `"parse config file: ..."` — backup original, create fresh |
| Backup fails | Non-fatal warning, mutation proceeds |

## Dependency: TOML

Add to `go.mod`:
```
require github.com/BurntSushi/toml v1.4.0
```

## Data Flow

```
User selects Switch in TUI
  → TUI: SelectTargetCLI → SelectProvider → MapModels
  → handleFormCompletion switchMapModelsView
     → switchSvc.BindProfile(targetCLIID, providerID, mappings)
     → switchSvc.Apply(targetCLIID, providerID)
        → cliRepo.Get(targetCLIID) → TargetCLI{mutator, mutator_config}
        → parse mutator_config JSON
        → mutators[cli.Mutator].Mutate(path, mappings, provider, cfg)
           → [mutator-specific logic]
           → CreateBackup(originalPath)
           → WriteAtomicJSON(newPath, mutatedData)
        → return BackupResult{BackupPath}
  → TUI: show confirmation with backup path
```

## Estimated Effort

| Component | Files | LOC |
|-----------|-------|-----|
| domain/targetcli.go changes | 1 | +15 |
| config/utils.go (extract shared) | 1 | +80 |
| mutators/claude_json.go | 1 | +40 |
| mutators/opencode_json.go | 1 | +45 |
| mutators/codex_toml.go | 1 | +50 |
| mutators/copilot_env.go | 1 | +40 |
| mutators/pi_dual.go | 1 | +55 |
| application/switch_svc.go changes | 1 | +25 |
| sqlite/db.go (migration) | 1 | +10 |
| sqlite/targetcli_repo.go (Get + fields) | 1 | +20 |
| main.go (registry wiring) | 1 | +20 |
| sqlite/targetcli_repo_test.go | 1 | +30 |
| mutators/*_test.go | 5 | +200 |
| **Total** | **16** | **~630** |
