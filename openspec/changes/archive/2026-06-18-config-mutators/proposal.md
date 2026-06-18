# Config Mutators — Pluggable Config Mutation Architecture

## Intent

### Business Problem

The current `MutateAndWrite` in `internal/infrastructure/config/settings.go` is hardcoded to Claude Code's JSON config structure. It:

- Assumes the config is a flat JSON object with an `env` block
- Hardcodes `delete(root, "ANTHROPIC_API_KEY")` as the security cleanup
- Builds the `env` block with Claude Code-specific env var names
- Has pruning logic that hardcodes the `settings.json` backup prefix
- Accepts `modelMappings` and `apiKey` but ignores the target CLI identity entirely

Research has identified 5 target CLIs (Claude Code, OpenCode, Codex, Copilot CLI, Pi) with radically different config formats: JSON with env blocks, deeply nested JSON provider trees, TOML tables, environment-variable-only profiles, and multi-file JSON pairs. Each new CLI would require code changes to `MutateAndWrite`, breaking the principle that adding a CLI should be purely data-driven.

### Target Users and Situations

- **aimux end users** who want to switch providers for any of the 5+ supported CLIs
- **aimux operators/maintainers** who want to add support for a new CLI by writing one struct and one INSERT
- **Power users** who use multiple AI CLIs concurrently and want profile switching to work for all of them

### Success Criteria

1. `SwitchUseCases.Apply` resolves the mutator by name (from `target_clis.mutator`) and calls it — zero format-specific logic in the application layer
2. Adding a new CLI requires exactly: INSERT into `target_clis` + 1 struct (~30 lines) implementing `domain.ConfigMutator`
3. Existing Claude Code behavior is fully preserved (backwards compatible)
4. All 5 CLIs identified in research have a clear mapping to a mutator implementation path

---

## Scope

### In-Scope

1. **`domain.ConfigMutator` interface** — a new interface in `internal/domain/` that encapsulates config mutation per CLI:

   ```go
   type ConfigMutator interface {
       // Mutate configures the target CLI's config file at the given path
       // with the given model mappings and provider information.
       Mutate(path string, modelMappings map[string]string, provider Provider) (*BackupResult, error)
   }
   ```

2. **Schema migration** — add two new columns to `target_clis`:
   - `mutator TEXT NOT NULL DEFAULT 'claude-code'` — identifies which mutator to use for this CLI
   - `api_key_env_var TEXT NOT NULL DEFAULT ''` — mutator-specific API key injection info (interpretation differs per mutator)

3. **Mutator registry** — a `map[string]domain.ConfigMutator` injected into `SwitchUseCases` at composition time, so `Apply()` does `mutator := uc.mutatorRegistry[targetCLI.Mutator]`

4. **Shared utilities** — extract from current `settings.go` into reusable helpers:
   - `config.PrepareDir(path string) error` — `os.MkdirAll(dir, 0755)`
   - `config.AtomicWrite(data []byte, path string) error` — temp file + sync + rename (currently inline in `MutateAndWrite`)
   - `config.CreateBackup(path, dir, prefix string) (string, error)` — currently `createBackup` but hardcodes backup name prefix
   - `config.PruneBackups(dir, prefix string, max int)` — currently hardcodes `settings.json` prefix and `maxBackups`
   - `config.ReadJSONWithLock` — already exported, keep as-is
   - `config.AcquireFlock` — exported if useful for non-JSON mutators

5. **Refactored Claude Code mutator** — extract the current JSON `env`-block logic into its own `ClaudeCodeMutator` struct, using shared utilities. Must preserve all existing behavior:
   - `delete(root, "ANTHROPIC_API_KEY")` from root (security invariant)
   - Build `env` block from model mappings + API key (skip empty vals)
   - Overwrite `root["env"]` with new block
   - Atomic write with flock, backup, and pruning

6. **`SwitchUseCases.Apply` refactoring** — wire mutator resolution:
   - Read `targetCLI.Mutator` from the repository (new column)
   - Look up the mutator in the registry
   - Call `mutator.Mutate(resolvedPath, mappings, provider)`
   - Return `BackupResult` from the mutator

7. **Insert logic for new CLIs** — `SeedTargetCLIs` must accept additional rows. For the MVP scope, only Claude Code is seeded; new CLIs are added later via separate changes (each mutator struct + seed row is its own batch).

### Out-of-Scope (Deferred)

- **Writing the 4 remaining mutator implementations** (OpenCode, Codex, Copilot, Pi). This proposal covers the architecture only; each mutator is a separate implementation batch.
- **TOML parsing** — adding a TOML library as a dependency is deferred to the Codex batch.
- **Shell profile mutation** — writing to `~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish` is deferred to the Copilot batch.
- **Multi-file mutations** — the Pi mutator writing to both `models.json` and `auth.json` simultaneously is deferred to the Pi batch.
- **Mutator unit tests** — each mutator batch includes its own tests; the proposal batch covers tests for the refactored Claude Code mutator only.
- **Changing the mapping spec** — the mapping/binding workflow (BindProfile) remains unchanged; model mappings are still a flat `map[string]string` per profile. Only the config WRITE side changes.
- **Changing the storage spec** — the schema change to `target_clis` is in-scope for this proposal but the full storage spec update is deferred to the design/specs phase.

---

## Approach

### Architecture Diagram

```
Apply(targetCLIID, providerID)
    |
    v
SwitchUseCases.Apply
    |-- targetCLI = cliRepo.Get(targetCLIID)
    |-- provider = providerRepo.Get(providerID)
    |-- activeMX = multiplexRepo.GetActive(targetCLIID)
    |-- mappings = parse(activeMX.ModelMappings)
    |-- resolvedPath = ResolveTargetConfigPath(targetCLI.ConfigPath)
    |-- mutator = registry[targetCLI.Mutator]     <-- NEW: lookup by name
    |-- backup, err = mutator.Mutate(resolvedPath, mappings, provider)  <-- NEW: delegate
    |-- return backup

domain.ConfigMutator (interface)
    |
    |-- ClaudeCodeMutator   (existing logic, refactored)
    |-- OpenCodeMutator     (future)
    |-- CodexMutator        (future)
    |-- CopilotMutator      (future)
    |-- PiMutator           (future)

Shared utilities (config package):
    PrepareDir, AtomicWrite, CreateBackup, PruneBackups, ReadJSONWithLock
```

### Key Decisions

#### Decision 1: Mutator owns all file I/O

Each mutator struct receives the file path, model mappings, and provider — and is responsible for all file operations. This is necessary because:

- TOML vs JSON vs shell profiles require different parsers
- Pi writes to TWO files (models.json + auth.json), not one
- Copilot writes to shell profiles with different atomicity guarantees
- Backup naming and pruning policies differ per CLI

The shared utilities (`PrepareDir`, `AtomicWrite`, `CreateBackup`, `PruneBackups`, `ReadJSONWithLock`) are **helpers** that mutators MAY call. They are not enforced by the interface. This keeps the interface simple and each mutator self-contained.

**Tradeoff**: Slight duplication of flock/backup/atomic-write boilerplate across JSON-based mutators (Claude Code, OpenCode, Pi's models.json). Mitigated by extracting these into named helpers that make each mutator body ~30 lines.

#### Decision 2: Registry injection, not global state

`SwitchUseCases` gains a `mutatorRegistry map[string]domain.ConfigMutator` field set via constructor or a `WithMutators()` builder. Composition happens at the main/application wiring layer, not via global `init()` functions or package-level maps.

**Rationale**: Testability. Each test can inject its own registry or mock mutators without package-level side effects. No init-order hazards.

#### Decision 3: Schema addition, not a separate mutator table

Adding `mutator` and `api_key_env_var` columns to `target_clis` is simpler than a separate `mutators` junction table. There is a one-to-one relationship between target CLI and mutator. A separate table is only justified if mutators need per-CLI configuration that doesn't fit in two columns — that's deferred and can be added later as a JSON config column if needed.

#### Decision 4: `api_key_env_var` semantics are mutator-specific

For Claude Code: the env var name to inject into `env` block (default `ANTHROPIC_API_KEY`).
For Copilot: the env var name to export (e.g. `COPILOT_PROVIDER_API_KEY`).
For OpenCode: the key path in the JSON config (e.g. `options.apiKey`).
For Pi: which field in auth.json to set.
For Codex: the env key name referenced by `env_key` in the TOML.

Each mutator documents what it expects in this column. An empty string means the mutator derives the key name from its own defaults.

### Impact Analysis

**Specs that need updating**:

| Spec | Change |
|------|--------|
| `switch/spec.md` | Mutation phase is no longer hardcoded to JSON `env` block. Must describe the mutator dispatch. |
| `storage/spec.md` | `target_clis` table gains `mutator` and `api_key_env_var` columns. `SeedTargetCLIs` includes these. |
| `mapping/spec.md` | No change — mapping is still flat `map[string]string` per profile. Only the write side changes. |

**Files that change**:

| File | Change Type |
|------|-------------|
| `internal/domain/targetcli.go` | + `ConfigMutator` interface, + `TargetCLI.Mutator` and `TargetCLI.APIKeyEnvVar` fields |
| `internal/domain/config_mutator.go` | NEW file — interface + BackupResult |
| `internal/infrastructure/config/settings.go` | Refactor `MutateAndWrite` into `ClaudeCodeMutator`, extract shared utilities |
| `internal/infrastructure/config/settings_test.go` | Update tests for refactored mutator + new shared utils |
| `internal/infrastructure/config/claude_code_mutator.go` | NEW file — refactored Claude Code mutator |
| `internal/infrastructure/config/utils.go` | NEW file — extracted shared utilities (AtomicWrite, CreateBackup, PruneBackups, PrepareDir) |
| `internal/application/switch_svc.go` | + registry lookup in `Apply()`, + constructor changes |
| `internal/infrastructure/sqlite/db.go` | + new columns in `target_clis` CREATE, update `SeedTargetCLIs` |
| `openspec/specs/switch/spec.md` | Update mutation phase to describe mutator dispatch |
| `openspec/specs/storage/spec.md` | Update `target_clis` table schema |

**Files that do NOT change**:

- `internal/domain/provider.go` — Provider struct stays the same
- `internal/domain/multiplex.go` — ActiveMultiplex struct stays the same
- `internal/infrastructure/config/settings.go` renamed/kept — current `settings.go` may be split into `claude_code_mutator.go` + `utils.go`
- Path resolution (`internal/application/path.go` or similar) — unchanged
- TUI layer — unchanged (switch commands still call `Apply(targetCLIID, providerID)`)

### Edge Cases

1. **Missing mutator in registry**: If `target_clis` references a mutator name that is not registered, `Apply` MUST return a clear error like `"mutator '{name}' not registered — add it to the application wiring"`.

2. **Empty `mutator` column**: Old rows (before migration) have empty or NULL mutator. `SeedTargetCLIs` MUST set a default of `"claude-code"` for the existing row. The schema default SHOULD be `'claude-code'` for backwards compatibility.

3. **Mutator receives empty `modelMappings`**: The contract is the same as today — empty values are excluded, the `env` block (for Claude Code) is an empty object with just the API key. Each mutator documents how it handles empty mappings.

4. **Provider with empty API key**: Mutators MUST handle empty API keys gracefully — write the key as empty string or skip the auth injection entirely. The behavior is mutator-specific.

5. **Migration safety**: Adding columns with defaults is safe for existing databases. The migration does not break existing rows.

### Rollback Plan

1. Revert the schema migration (ALTER TABLE is not reversible in SQLite without a table rebuild — instead, keep the old columns and leave new ones unused).
2. Revert `SwitchUseCases.Apply` to call `config.MutateAndWrite` directly.
3. Keep the new mutator code but wire it as the Claude Code path only.
4. The most conservative rollback: keep the old `MutateAndWrite` function signature intact, have the new Claude Code mutator delegate to it internally.

---

## Open Questions

These should be resolved during the specs/design phase:

1. **`BackupResult` in domain or infrastructure?** Currently `config.BackupResult` is in the infrastructure layer. Should the interface return it, or should it be moved to domain? Moving to domain would make the interface truly domain-layer, but `config.BackupResult` is simple enough. Decision: keep it in domain, `domain.BackupResult`.

2. **Error type hierarchy**: Should each mutator define its own error types, or should we have domain-level error wrappers? For MVP, each mutator can define its own sentinel errors (as `settings.go` does today). A unified error taxonomy can be added later.

3. **Prefix-based backup pruning**: The current `pruneBackups` hardcodes `"settings.json.aimux-backup-"`. The extracted utility needs the prefix as a parameter. Should the prefix be derived from the filename (e.g., `filepath.Base(path) + ".aimux-backup-"`) or passed explicitly? Decision: derive from `filepath.Base(path)` automatically, override only if needed.

4. **TOML dependency**: Adding `BurntSushi/toml` or `pelletier/go-toml` is deferred to the Codex batch, but the design should ensure the shared utilities don't assume JSON.

5. **Seed data expansion**: Should existing databases be migrated to add default mutator values for existing rows? Or should the application default to `"claude-code"` when the column is empty/NULL? Decision: the app layer defaults, migration just adds the column.

---

## Summary

This proposal refactors the single monolithic `MutateAndWrite` into a `domain.ConfigMutator` interface with one concrete implementation (Claude Code, extracted from existing code) and a registry pattern for dispatch. The schema gains two columns to identify which mutator a CLI uses and how API keys are injected. The total new code is approximately 100 lines (interface + registry wiring + refactored Claude Code mutator + shared utilities). The design unlocks adding OpenCode, Codex, Copilot, and Pi as separate, self-contained batches with no changes to the core switch flow.
