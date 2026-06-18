# Storage — Data Access Layer Spec

## Scope

All persistent state management for aimux: SQLite schema, CRUD operations, and file-level locking for JSON config mutation. This is the single source of truth for runtime state and the target for all profile-switch JSON writes.

## Schema

### SQLite Tables

#### `providers`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL UNIQUE | Human label, e.g. "My OpenAI" |
| base_url | TEXT | NOT NULL | e.g. `https://api.openai.com/v1` |
| api_key | TEXT | NOT NULL | Plaintext (MVP) |
| auth_token | TEXT | NOT NULL | Plaintext (MVP) |
| status | TEXT | NOT NULL DEFAULT 'active' | `active` or `error` |
| created_at | TEXT | NOT NULL DEFAULT CURRENT_TIMESTAMP | ISO 8601 |
| updated_at | TEXT | NOT NULL DEFAULT CURRENT_TIMESTAMP | ISO 8601 |

- `status` MUST be set to `error` when `/v1/models` fetch fails.
- `status` MUST be set to `active` when the provider is first added or when a retry fetch succeeds.

#### `provider_models`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INTEGER | PK AUTOINCREMENT | |
| provider_id | INTEGER | NOT NULL FK -> providers(id) ON DELETE CASCADE | |
| model_name | TEXT | NOT NULL | Raw string from API, e.g. `"claude-sonnet-4-20250514"` |
| UNIQUE(provider_id, model_name) | | | No duplicate model strings per provider |

- `model_name` MUST be stored exactly as returned by the provider API. No normalization, no classification.

#### `target_clis`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| id | INTEGER | PK AUTOINCREMENT | |
| name | TEXT | NOT NULL UNIQUE | e.g. `"claude-code"` |
| config_path | TEXT | NOT NULL | e.g. `"~/.config/claude/settings.json"` |
| env_vars | TEXT | NOT NULL | JSON array of env var names, e.g. `["ANTHROPIC_DEFAULT_HAIKU_MODEL", ...]` |
| mutator | TEXT | NOT NULL DEFAULT 'claude-settings-json' | registry key for config mutator |
| mutator_config | TEXT | NOT NULL DEFAULT '{}' | JSON object for mutator-specific config |

- `env_vars` MUST be a JSON-encoded array of strings identifying the environment variables this CLI reads.
- `mutator` identifies which `ConfigMutator` implementation to use for this CLI. Defaults to `"claude-settings-json"` for backwards compatibility.
- `mutator_config` is a JSON object whose structure is interpreted by each mutator implementation.
- For MVP, only `"claude-code"` SHALL be seeded with the four Claude Code variables.

#### `active_multiplex`

| Column | Type | Constraints | Notes |
|--------|------|-------------|-------|
| target_cli_id | INTEGER | PK FK -> target_clis(id) | One active profile per CLI |
| provider_id | INTEGER | NOT NULL FK -> providers(id) | |
| model_mappings | TEXT | NOT NULL | JSON object: `{"ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-xxx", ...}` |
| activated_at | TEXT | NOT NULL DEFAULT CURRENT_TIMESTAMP | |

- INSERT OR REPLACE semantics: switching replaces the existing row for the same `target_cli_id`.

### Indexes

- `idx_provider_models_provider_id` ON `provider_models(provider_id)`
- `idx_active_multiplex_target_cli_id` ON `active_multiplex(target_cli_id)` (PK, implicit)

### File Permissions

- The SQLite database file MUST be created at `~/.config/aimux/matrix.db`.
- The database file MUST have permissions `0600` (owner read/write only).
- The parent directory `~/.config/aimux/` MUST have permissions `0700`.
- WAL journal mode MUST be enabled on database open.

## Operations

### `AddProvider(name, baseURL, apiKey, authToken) -> (providerID, error)`

- INSERT into `providers` with `status = 'active'`.
- If name already exists, MUST return an error (unique constraint).
- Returns the new provider ID.

### `GetProvider(id) -> (Provider, error)`

- Returns all columns for the given provider ID.
- MUST return an error if not found.

### `ListProviders() -> ([]Provider, error)`

- Returns all providers ordered by `name` ASC.

### `UpdateProviderStatus(id, status) -> error`

- Sets `status` and `updated_at`.

### `DeleteProvider(id) -> error`

- Deletes the provider. CASCADE deletes its models and active_multiplex entries.

### `InsertModels(providerID, modelNames []string) -> error`

- INSERT each model name into `provider_models` (ignore duplicates via UNIQUE constraint or INSERT OR IGNORE).
- MUST clear existing models for the provider before inserting new ones, to reflect current API state.

### `ListModels(providerID) -> ([]Model, error)`

- Returns all models for the given provider ordered by `model_name` ASC.

### `ListAllModels() -> ([]Model, error)`

- Returns all models across all providers, with provider name joined.
- Used by the mapping forms to present all available models.

### `GetActiveMultiplex(targetCLIID) -> (ActiveMultiplex, error)`

- Returns the active multiplex row for the given CLI.
- MUST return no error if no row exists (empty state).

### `SetActiveMultiplex(targetCLIID, providerID, modelMappings JSON) -> error`

- INSERT OR REPLACE into `active_multiplex`.

### `ClearActiveMultiplex(targetCLIID) -> error`

- DELETE the row for the given CLI.

### `ListActiveMultiplexes() -> ([]ActiveMultiplex, error)`

- Returns all active multiplex rows with provider name and CLI name joined.
- Used by the TUI dashboard to render the status table.

### `SeedTargetCLIs() -> error`

- MUST insert `claude-code` target CLI on first run if not present.
- `config_path`: `~/.config/claude/settings.json`
- `env_vars`: `["ANTHROPIC_DEFAULT_HAIKU_MODEL", "ANTHROPIC_DEFAULT_SONNET_MODEL", "ANTHROPIC_DEFAULT_OPUS_MODEL", "CLAUDE_CODE_SUBAGENT_MODEL"]`
- `mutator`: `"claude-settings-json"`
- `mutator_config`: `"{}"`
- MUST be idempotent (INSERT OR IGNORE on UNIQUE constraint).

## File Locking (JSON Config Mutation)

- Before reading or writing any JSON config file (specifically `settings.json`), a `syscall.Flock` SHALL be acquired on the file.
- The lock MUST be an exclusive lock (`LOCK_EX`) for writes, and a shared lock (`LOCK_SH`) for reads.
- The lock MUST be released after the read or write completes, using a deferred unlock.
- If the lock cannot be acquired within 2 seconds, the operation MUST fail with a timeout error.
- The flock operates on the file descriptor, not the path. The path-to-FD mapping is the caller's responsibility.

## Acceptance Scenarios

### Schema Creation

Given the application is started for the first time  
When `SeedTargetCLIs` is called  
Then the `target_clis` table contains exactly one row: `name = "claude-code"`  
And the four Claude Code env vars are stored as a JSON array in `env_vars`

### Add Provider — Success

Given no provider named "My Anthropic" exists  
When `AddProvider` is called with name "My Anthropic", valid URL, api key, and auth token  
And `InsertModels` is called with model IDs from a successful `/v1/models` response  
Then a provider row exists with `status = 'active'`  
And a corresponding `provider_models` row exists for each API-returned model ID

### Add Provider — Duplicate Name

Given a provider named "My Anthropic" exists  
When `AddProvider` is called with name "My Anthropic" again  
Then the call MUST return an error indicating the name already exists

### Active Multiplex — Switch

Given an active multiplex does not exist for CLI `claude-code`  
When `SetActiveMultiplex` is called with targetCLIID = 1, providerID = 2, and a valid model mappings JSON  
Then a row exists in `active_multiplex` with the given values  
And calling `SetActiveMultiplex` again for the same targetCLIID replaces the row (no duplicate)

### File Lock — Exclusive Write

Given a JSON config file exists  
When a write operation acquires `LOCK_EX` on the file descriptor  
Then a concurrent read attempt MUST block until the lock is released  
And the write succeeds after lock acquisition

### File Lock — Timeout

Given the file is held by another process with `LOCK_EX`  
When a write attempt tries to acquire `LOCK_EX` with 2s timeout  
Then the operation MUST fail with a timeout error after 2 seconds

## Schema Migration for Config Mutators

### Requirement: New Columns on target_clis

The `target_clis` table must support mutator selection and configuration.

#### Scenario: Schema migration adds columns

Given the existing `target_clis` table
When the migration runs
Then columns `mutator TEXT NOT NULL DEFAULT 'claude-settings-json'` and `mutator_config TEXT NOT NULL DEFAULT '{}'` are added.

### Migration DDL

```sql
-- Migration: add mutator support to target_clis
-- Safe: uses IF NOT EXISTS pattern via ALTER TABLE ADD COLUMN with defaults

-- Step 1: Add mutator column (backwards compatible: defaults to claude-settings-json)
ALTER TABLE target_clis ADD COLUMN mutator TEXT NOT NULL DEFAULT 'claude-settings-json';

-- Step 2: Add mutator_config column (JSON, each mutator interprets its own config)
ALTER TABLE target_clis ADD COLUMN mutator_config TEXT NOT NULL DEFAULT '{}';
```

#### Scenario: Existing Claude Code row preserved

Given the `claude-code` row was seeded in a previous migration
When the new migration runs
Then the `claude-code` row now has `mutator = 'claude-settings-json'` and `mutator_config = '{}'`.

#### Scenario: Seed includes new columns

Given `SeedTargetCLIs` runs
When the claude-code row is inserted
Then it includes `mutator` and `mutator_config` values.

### Requirement: Updated TargetCLI Domain Struct

```go
type TargetCLI struct {
    ID            int64
    Name          string
    ConfigPath    string
    EnvVars       string  // JSON array of env var names
    Mutator       string  // mutator registry key
    MutatorConfig string  // JSON object for mutator-specific config
}
```

#### Scenario: Repository returns new fields

Given a `TargetCLIRepository.List()` call
When rows are scanned
Then `Mutator` and `MutatorConfig` fields are populated from the database.
