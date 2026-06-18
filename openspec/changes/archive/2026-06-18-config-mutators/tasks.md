# Config Mutators — Implementation Tasks

## Dependency Graph

```
Phase 1: Foundation
  T1: Extract shared utils from config/settings.go → config/utils.go
  T2: Add BackupResult + ConfigMutator interface to domain/targetcli.go
  └─> (no deps)

Phase 2: Schema
  T3: Add ALTER TABLE migration for mutator + mutator_config columns
  T4: Update TargetCLI struct + TargetCLIRepository (scan, Get method)
  T5: Update SeedTargetCLIs with mutator + mutator_config
  └─> depends on: T2

Phase 3: Mutators (parallel)
  T6: ClaudeSettingsJSON mutator (refactor from settings.go)
  T7: OpenCodeProviderJSON mutator
  T8: CodexConfigTOML mutator
  T9: CopilotEnvFile mutator
  T10: PiDualJSON mutator
  └─> depends on: T1, T2

Phase 4: Wiring
  T11: Refactor SwitchUseCases.Apply (registry lookup)
  T12: Update SwitchUseCases constructor (inject mutators map)
  T13: Wire mutator registry in main.go
  └─> depends on: T6-T10

Phase 5: TUI
  T14: Update confirmation view to show backup path
  └─> depends on: T11

Phase 6: Cleanup
  T15: Remove old config.MutateAndWrite, keep only shared utils
  T16: go mod tidy (add BurntSushi/toml)
  └─> depends on: T11

Phase 7: Tests (parallel)
  T17: Shared utils tests
  T18: Claude mutator tests
  T19: OpenCode mutator tests
  T20: Codex mutator tests
  T21: Copilot mutator tests
  T22: Pi mutator tests
  T23: SwitchUseCases.Apply with registry tests
  └─> depends on: T6-T10, T11
```

## Tasks

### Phase 1: Foundation

- [x] **T1**: Extract shared utilities from `internal/infrastructure/config/settings.go` into `internal/infrastructure/config/utils.go`
  - Move: `acquireFlock`, `ReadJSONWithLock`, `WriteAtomicJSON`, `CreateBackup`, `PruneBackups`
  - `WriteAtomicJSON` is new: marshals map to temp file, renames atomically
  - Keep `settings.go` for now (T15 removes old code)
  - Estimated: 80 LOC

- [x] **T2**: Add `BackupResult` struct and `ConfigMutator` interface to `internal/domain/targetcli.go`
  - Add `Mutator` and `MutatorConfig` fields to `TargetCLI` struct
  - Estimated: 15 LOC

### Phase 2: Schema

- [x] **T3**: Add migration function `MigrationAddMutatorColumns()` to `internal/infrastructure/sqlite/db.go`
  - `ALTER TABLE target_clis ADD COLUMN mutator TEXT NOT NULL DEFAULT 'claude-settings-json'`
  - `ALTER TABLE target_clis ADD COLUMN mutator_config TEXT NOT NULL DEFAULT '{}'`
  - Call migration in `main.go` after existing migrations
  - Estimated: 10 LOC

- [x] **T4**: Update `internal/infrastructure/sqlite/targetcli_repo.go`
  - Add `Get(id int64)` method
  - Update `List()` scan to include `Mutator` and `MutatorConfig`
  - Add `Get` to `domain.TargetCLIRepository` interface
  - Estimated: 20 LOC

- [x] **T5**: Update `SeedTargetCLIs()` in `internal/infrastructure/sqlite/db.go`
  - Include `mutator` and `mutator_config` in INSERT
  - Estimated: 5 LOC

### Phase 3: Mutators

- [x] **T6**: Create `internal/infrastructure/mutators/claude_json.go` — `"claude-settings-json"`
  - Struct: `ClaudeSettingsJSON{Utils *config.Utils}` implementing `domain.ConfigMutator`
  - Logic: read JSON → delete root ANTHROPIC_API_KEY → build env map → set env + API key → backup + write
  - Estimated: 40 LOC

- [x] **T7**: Create `internal/infrastructure/mutators/opencode_json.go` — `"opencode-provider-json"`
  - Requires `provider_id` and `npm` in mutatorConfig
  - Logic: read JSON → ensure `provider` key → build provider entry with npm, name, baseURL, apiKey → build models map → set + write
  - Estimated: 45 LOC

- [x] **T8**: Create `internal/infrastructure/mutators/codex_toml.go` — `"codex-config-toml"`
  - Add `github.com/BurntSushi/toml` dependency
  - Requires `provider_id` in mutatorConfig
  - Logic: read TOML → set model/model_provider → build [model_providers] table → marshal → backup + write
  - API key written to env file, not TOML
  - Estimated: 50 LOC

- [x] **T9**: Create `internal/infrastructure/mutators/copilot_env.go` — `"copilot-env-file"`
  - No config file — writes key=value to `.env` file
  - Requires `provider_type` in mutatorConfig (defaults to `"openai"`)
  - Optional `local` flag skips API key
  - Logic: build env lines → create dir → backup if exists → write .env
  - Estimated: 40 LOC

- [x] **T10**: Create `internal/infrastructure/mutators/pi_dual.go` — `"pi-dual-json"`
  - Requires `provider_id` and `provider_type` in mutatorConfig
  - Logic: read models.json → build provider entry → write models.json → read auth.json → set credentials → write auth.json
  - Both files backed up
  - Estimated: 55 LOC

### Phase 4: Wiring

- [x] **T11**: Refactor `internal/application/switch_svc.go` — `Apply()` method
  - Parse `mutator_config` JSON from `cli.MutatorConfig`
  - Lookup mutator from injected map
  - Call `mutator.Mutate()` instead of `config.MutateAndWrite`
  - Add `cliRepo.Get()` call (new method from T4)
  - Estimated: 25 LOC

- [x] **T12**: Update `SwitchUseCases` constructor
  - Add `mutators map[string]domain.ConfigMutator` parameter
  - Update `NewSwitchUseCases` signature
  - Estimated: 5 LOC

- [x] **T13**: Wire mutator registry in `main.go`
  - Create shared utils instance
  - Create each mutator with utils
  - Build `map[string]domain.ConfigMutator`
  - Pass to `NewSwitchUseCases`
  - Estimated: 20 LOC

### Phase 5: TUI

- [x] **T14**: Update switch confirmation view in `internal/tui/model.go`
  - `Apply()` now returns `*domain.BackupResult`
  - Show backup path in confirmation message
  - Estimated: 10 LOC

### Phase 6: Cleanup

- [x] **T15**: Remove `config.MutateAndWrite` from `internal/infrastructure/config/settings.go`
  - Keep only shared utils
  - Remove Claude Code-specific logic
  - Estimated: -80 LOC (deletion)

- [x] **T16**: Run `go mod tidy` to add BurntSushi/toml, clean unused deps
  - Estimated: 0 LOC (tooling)

### Phase 7: Tests

- [x] **T17**: Add tests for shared utils in `internal/infrastructure/config/utils_test.go`
  - Test WriteAtomicJSON with temp dirs
  - Test backup creation and pruning
  - Test flock acquisition
  - Estimated: 60 LOC

- [x] **T18**: Add Claude mutator tests in `internal/infrastructure/mutators/claude_json_test.go`
  - Test with existing settings.json, verify env block, preserved keys
  - Test with missing file
  - Estimated: 40 LOC

- [x] **T19**: Add OpenCode mutator tests in `internal/infrastructure/mutators/opencode_json_test.go`
  - Test provider entry creation, model mapping, preserved providers
  - Test missing provider_id error
  - Estimated: 40 LOC

- [x] **T20**: Add Codex mutator tests in `internal/infrastructure/mutators/codex_toml_test.go`
  - Test TOML generation, model_provider key, env_key
  - Test with existing TOML sections
  - Estimated: 40 LOC

- [x] **T21**: Add Copilot mutator tests in `internal/infrastructure/mutators/copilot_env_test.go`
  - Test .env file generation
  - Test local provider (no API key)
  - Test azure/anthropic types
  - Estimated: 40 LOC

- [x] **T22**: Add Pi mutator tests in `internal/infrastructure/mutators/pi_dual_test.go`
  - Test models.json + auth.json writes
  - Test preserved providers
  - Estimated: 50 LOC

- [x] **T23**: Update SwitchUseCases tests for registry-based Apply
  - Test mutator resolution
  - Test fallback to claude-settings-json
  - Test missing mutator error
  - Estimated: 40 LOC

---

## Review Workload Forecast

| Metric | Value |
|--------|-------|
| Estimated changed lines | ~630 new + ~80 deleted = ~710 net |
| Files touched | 16 |
| Chained PRs recommended | No (single coherent refactor) |
| 400-line budget risk | Low (spread across 16 files) |
| Decision needed before apply | No |
