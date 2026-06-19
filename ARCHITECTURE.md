# aimux Architecture

> Single-binary Go application. TUI-first. DDD layered architecture. SQLite-backed.
> Zero CGO dependencies. Idempotent config mutation. Centralized backup system.

---

## Table of Contents

1. [System Overview](#system-overview)
2. [Layers](#layers)
   - [Domain](#domain)
   - [Application](#application)
   - [Infrastructure](#infrastructure)
   - [TUI](#tui)
3. [Component Diagrams](#component-diagrams)
4. [Data Flow](#data-flow)
5. [Key Design Decisions](#key-design-decisions)
6. [Error Handling](#error-handling)
7. [Concurrency Model](#concurrency-model)
8. [Interfaces](#interfaces)
9. [Package Contracts](#package-contracts)

---

## System Overview

```
┌──────────────────────────────────────────────────────────────┐
│                        main.go                                │
│  Entrypoint · CLI routing · TUI launch · DB init · Mutator   │
│  registry · Migration chain                                  │
├──────────────────────────────────────────────────────────────┤
│  ┌─────────────────────────────────────┐  ┌───────────────┐ │
│  │            TUI (Bubbletea)          │  │  CLI (Args)   │ │
│  │  Model · Update · View · Forms      │  │  apply · list │ │
│  │  Menu · Table · Stepper · Theme     │  │  backups ·    │ │
│  │  Dashboard · Providers · SwitchFlow │  │  restore ·    │ │
│  └─────────────────────────────────────┘  │  version ·    │ │
│                                           │  update       │ │
│                                           └───────────────┘ │
├──────────────────────────────────────────────────────────────┤
│                 Application Layer                            │
│  ┌─────────────────────┐  ┌──────────────────────────────┐  │
│  │  ProviderUseCases   │  │     SwitchUseCases           │  │
│  │  · CRUD             │  │  · Apply (mutate config)     │  │
│  │  · FetchModels      │  │  · DryRun (preview diff)     │  │
│  │  · TestConnectivity │  │  · BindProfile (validate+save)│  │
│  │  · RetryFetch       │  │  · Backups / Restore         │  │
│  │  · saveModelMetadata│  │  · ClearCLIConfig            │  │
│  │                     │  │  · Multi-provider bindings   │  │
│  └─────────────────────┘  └──────────────────────────────┘  │
├──────────────────────────────────────────────────────────────┤
│                    Domain Layer                              │
│  Provider · ProviderModel · ModelMetadata · TargetCLI       │
│  ActiveMultiplex · BackupResult                             │
│  ProviderRepository · TargetCLIRepository · MultiplexRepo   │
│  ConfigMutator interface                                    │
├──────────────────────────────────────────────────────────────┤
│                Infrastructure Layer                          │
│  ┌────────────────┐ ┌───────────────┐ ┌──────────────────┐  │
│  │  SQLite Repos  │ │   Mutators    │ │  Config Utils    │  │
│  │  · providers   │ │ claude_json   │ │  AtomicWrite     │  │
│  │  · models      │ │ opencode_json │ │  ReadJSONWithLock│  │
│  │  · target_clis │ │ codex_toml    │ │  CreateBackup    │  │
│  │  · multiplex   │ │ copilot_shell │ │  PruneBackups    │  │
│  │  · update_cache│ │ pi_dual       │ │  RestoreBackup   │  │
│  └────────────────┘ └───────────────┘ └──────────────────┘  │
│  ┌────────────────┐ ┌───────────────┐                       │
│  │ Model Catalog  │ │ Self-Update   │                       │
│  │ Known models    │ │ Checker       │                       │
│  │ w/ metadata     │ │ Cache (24h)   │                       │
│  │ Cost, ctx, etc  │ │ Updater (tar) │                       │
│  └────────────────┘ └───────────────┘                       │
└──────────────────────────────────────────────────────────────┘
```

---

## Layers

### Domain

**Path**: `internal/domain/`

Pure data structures and interfaces. Zero dependencies on infrastructure or application layers.

| File | Contents |
|------|----------|
| `provider.go` | `Provider` struct, `ProviderModel`, `ModelMetadata` (typed map), `ProviderRepository` interface, metadata key constants |
| `targetcli.go` | `TargetCLI` struct, `TargetCLIRepository` interface, `ConfigMutator` interface, `BackupResult` |
| `multiplex.go` | `ActiveMultiplex` struct, `MultiplexRepository` interface |

**Key interfaces**:

```go
type ProviderRepository interface {
    Add(name, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) (int64, error)
    Get(id int64) (Provider, error)
    List() ([]Provider, error)
    Update(id int64, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) error
    UpdateStatus(id int64, status string) error
    Delete(id int64) error
    InsertModels(providerID int64, modelNames []string) error
    DeleteModelsByProvider(providerID int64) error
    ListModels(providerID int64) ([]ProviderModel, error)
    ListAllModels() ([]ProviderModel, error)
    UpdateModelMetadata(providerID int64, modelName string, metadata ModelMetadata) error
}

type ConfigMutator interface {
    Mutate(configPath string, modelMappings map[string]string, provider Provider, mutatorConfig map[string]any) (*BackupResult, error)
}
```

**ModelMetadata keys** (constant-defined, used across all layers):

| Key | Type | Use |
|-----|------|-----|
| `context_window` | int64 | Max input tokens |
| `max_tokens` | int64 | Max output tokens |
| `reasoning` | bool | Extended thinking support |
| `input_modalities` | []string | e.g. `["text","image"]` |
| `cost` | map (input, output, cacheRead, cacheWrite) | Per-million token pricing |
| `compat` | map | Provider/model compatibility flags |
| `thinking_level_map` | map | Pi-specific thinking level mappings |
| `headers` | map | Custom HTTP headers |
| `context_suffix` | string | Display suffix: `[1m]`, `[200k]` |
| `extra_env` | map | Extra env vars (Claude Code) |

### Application

**Path**: `internal/application/`

Use cases and orchestration logic. Depends on Domain interfaces, Infrastructure config utils, but NOT on SQLite or TUI.

| File | Contents |
|------|----------|
| `provider_svc.go` | `ProviderUseCases`: Add, List, Get, Update, Delete, FetchModels, TestConnectivity, RetryFetch, HTTP client, JSON response parsing |
| `multiplex_svc.go` | `SwitchUseCases`: Apply, DryRun, BindProfile, ListBindingsForCLI, RemoveBinding, ClearCLIConfig, ListBackups, RestoreLatest, RestoreBackup, multi-provider orchestration |
| `path.go` | `ExpandTilde`, `ResolveConfigDir`, `ResolveConfigPath`, `SetupLogFile`, `ResolveTargetConfigPath` — OS path resolution |
| `helpers_test.go` | Test utilities: in-memory SQLite setup, provider seeding, mutator registry |
| `provider_svc_test.go` | Provider use case tests |
| `multiplex_svc_test.go` | Switch use case tests |

**SwitchUseCases.Apply() flow**:

```
1. Get TargetCLI by ID
2. List all active multiplexes for this CLI
3. Filter by providerID (or apply all)
4. Resolve config path (tilde expansion)
5. Parse mutator_config JSON
6. Resolve mutator name (fallback to claude-settings-json)
7. Look up mutator from registry
8. For each binding:
   a. Get provider + models + metadata
   b. Parse model mappings JSON
   c. Build per-provider config (clear_providers on first iteration)
   d. Inject _registered_models, _model_metadata
   e. Call mutator.Mutate()
9. Return last BackupResult
```

**Multi-provider pattern**: CLIs with `opencode-provider-json`, `pi-dual-json`, or `copilot-shell-profile` mutators support multiple simultaneous providers. Each call to `Mutate()` adds/replaces its own entry. The first call clears the provider map so deleted bindings don't leave stale entries.

### Infrastructure

**Path**: `internal/infrastructure/`

Concrete implementations of domain interfaces plus supporting utilities.

#### SQLite (`internal/infrastructure/sqlite/`)

| File | Contents |
|------|----------|
| `db.go` | `Open()` with WAL mode, FK enforcement, busy timeout, `0600` permissions. Migration chain: `RunMigrations` → `MigrationAddMutatorColumns` → `MigrationAddDiscoveryURLColumn` → `MigrationDropApiTypeColumn` → `MigrationAddModelMetadataColumn` → `MigrationMultiProvider` → `MigrationRemoveOpenCodeNpm` → `MigrationAddDefaultContextWindow` → `CreateIndexes` → `SeedTargetCLIs`. All idempotent. |
| `provider_repo.go` | `ProviderRepository` implementation. Name uniqueness check. |
| `targetcli_repo.go` | `TargetCLIRepository` implementation. |
| `multiplex_repo.go` | `MultiplexRepository` with composite PK `(target_cli_id, provider_id)` for multi-provider support. `SetActive` uses `ON CONFLICT ... DO UPDATE` for upsert. |

**Database schema** (after all migrations):

```sql
CREATE TABLE providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    discovery_url TEXT NOT NULL DEFAULT '',
    default_context_window INTEGER NOT NULL DEFAULT 0,
    api_key TEXT NOT NULL DEFAULT '',
    auth_token TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','error')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    UNIQUE(provider_id, model_name)
);

CREATE TABLE target_clis (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    config_path TEXT NOT NULL,
    env_vars TEXT NOT NULL,
    mutator TEXT NOT NULL DEFAULT 'claude-settings-json',
    mutator_config TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE active_multiplex (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_cli_id INTEGER NOT NULL REFERENCES target_clis(id) ON DELETE CASCADE,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_mappings TEXT NOT NULL,
    activated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(target_cli_id, provider_id)
);

CREATE TABLE update_cache (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    checked_at TEXT NOT NULL
);
```

#### Mutators (`internal/infrastructure/mutators/`)

| File | Registry Key | Target CLI | Output Format |
|------|-------------|------------|---------------|
| `claude_json.go` | `claude-settings-json` | Claude Code | `settings.json` with `env` block |
| `opencode_json.go` | `opencode-provider-json` | OpenCode | `config.json` with `provider.<id>` entry |
| `codex_toml.go` | `codex-config-toml` | Codex | `config.toml` + `.env` file |
| `copilot_shell.go` | `copilot-shell-profile` | GitHub Copilot | Shell profile (`~/.zshrc` etc.) with idempotent markers |
| `pi_dual.go` | `pi-dual-json` | pi | `models.json` with `providers.<id>` entry |

**Shared helpers** (in `pi_dual.go`, used by `opencode_json.go` and `codex_toml.go`):

- `buildModelList()` — extracts model names from `_registered_models` or `modelMappings`
- `extractProviderHeaders()` — extracts custom HTTP headers from mutator config
- `extractProviderCompat()` — promotes shared compat fields from model-level to provider-level
- `fillPiModelEntry()` / `fillOpenCodeModelEntry()` — populates model entries from metadata
- `copyField()` — copies a field from source map to dest map

**Mutator contract**:

1. Create backup BEFORE reading (file existence on disk, not parse success)
2. Read existing config with flock (ReadJSONWithLock)
3. Merge mutations into existing structure (never replace root)
4. Write atomically (WriteAtomicJSON / AtomicWrite)
5. Prune old backups (keep 5)

#### Config Utilities (`internal/infrastructure/config/`)

| File | Contents |
|------|----------|
| `utils.go` | `ReadJSONWithLock` (trailing comma tolerance), `AtomicWrite`, `WriteAtomicJSON`, `CreateBackup`, `ListBackups`, `PruneBackups`, `RestoreBackup`, `PrepareDir`, flock with timeout |
| `utils_test.go` | 12 tests: read/write/empty/invalid/atomic/backup/prune/restore |
| `model_catalog.go` | `KnownModelCatalog` (40+ known models), `LookupModelMetadata`, `StripProviderPrefix`, `ContextSuffixForWindow`, `LookupContextSuffix`, `FormatCost`, `ApplyModelOverrides`, `cloneMetadata` |

**Sentinel errors**:

```go
var (
    ErrConfigNotFound = errors.New("config file not found")
    ErrConfigParse    = errors.New("could not parse config file: invalid JSON")
    ErrFlockTimeout   = errors.New("could not acquire file lock: timeout")
    ErrTempFileCreate = errors.New("could not create temporary file")
    ErrTempFileWrite  = errors.New("could not write to temporary file")
    ErrAtomicRename   = errors.New("could not rename temp file atomically")
    ErrSyncFile       = errors.New("could not sync config file to disk")
)
```

#### Self-Update (`internal/infrastructure/update/`)

| File | Contents |
|------|----------|
| `models.go` | `UpdateInfo` struct |
| `checker.go` | `CheckForUpdate()` — cached GitHub Releases API check (24h TTL), semver comparison |
| `cache.go` | `CacheGet` / `CacheSet` with 24h expiry via SQL `datetime('now', '-24 hours')` |
| `updater.go` | `SelfUpdate()` — download tar.gz, SHA256 checksum validation, atomic replace, Homebrew detection+delegation, `isHomebrewInstall` / `homebrewUpdate` |

**Self-update flow**:

```
1. Check write permission in binary directory
2. Fetch latest release from GitHub API
3. Compare versions (semver)
4. Build archive name: aimux_<version>_<os>_<arch>.tar.gz
5. Download archive to temp file
6. Extract binary from tar.gz
7. Download checksums.txt, validate SHA256
8. Atomic replace: temp rename → chmod 0755 → rename over target
```

### TUI

**Path**: `internal/tui/`

Bubbletea model with Elm Architecture (Model/Init/Update/View).

| File | Contents |
|------|----------|
| `model.go` | Core `model` struct, `Init()`, `Update()`, `View()`, message types, keymap, form completion handling, view transitions, loading state, notification system, diff viewport |
| `forms.go` | `huh.Form` builders: Add/Edit/Delete Provider, Select CLI, Select Provider, Map Models, Register Models, Edit Models, Select Single Model, Edit CLI Path, Restore Backup |
| `table.go` | Dashboard table renderer — provider list with status badges, model counts |
| `menu.go` | Side menu rendering with selection indicators |
| `theme.go` | Synthwave Outrun palette (`#0F051D` base, `#FF007F` accent, `#00F0FF` cyan), `HuhTheme()`, all style constants |
| `stepper.go` | Switch flow step indicator (`● ○ ◉` dots with labels) |
| `truncate.go` | Inline text truncation utilities |
| `logo.go` | ASCII art logo |
| `tui_test.go` | TUI tests |

**View types** (state machine):

```
dashboardView → providerListView → addProviderView
                                  → editProviderView
                                  → deleteProviderView
                                  → switchTargetCLIView → switchProviderView
                                                        → switchMapModelsView
                                                        → switchSelectModelsView
                                                        → switchAdvancedConfigView
                                                        → switchConfirmationView
              → manageCLIView → editCLIPathView
              → restoreCLIView → restoreBackupView
              → switchManageBindingsView → deleteBindingConfirmView
```

**Message types**:

| Message | Direction | Purpose |
|---------|-----------|---------|
| `DashboardRefreshMsg` | async → model | Trigger data refresh after background operation |
| `SwitchToViewMsg` | sync → model | Navigate to a different view |
| `FormResultMsg` | form complete → model | Form submission result |
| `notificationMsg` | any → model | Show toast notification with severity and TTL |
| `applyResultMsg` | async → model | Config apply completed |
| `retryFetchResultMsg` | async → model | Model fetch retry completed |
| `testConnectivityResultMsg` | async → model | Connectivity test completed |

**Loading guard**: During async operations (model fetch, apply, test), all navigation input is blocked except `Quit`. Spinner is rendered with contextual message.

**Hybrid adaptive layout**: Centered mode for most views (forms, tables, menus). Fluid full-width mode for diff views (side-by-side panels).

---

## Data Flow

### Provider Creation + Model Discovery

```
TUI Form → ProviderUseCases.Add()
     │
     ├─ providerRepo.Add() → INSERT INTO providers
     │
     ├─ ProviderUseCases.FetchModels()
     │     │
     │     ├─ resolveBaseURL() + /v1/models
     │     ├─ HTTP GET with Bearer auth, timeout 5s
     │     ├─ parseModelsResponse() → []string
     │     ├─ providerRepo.InsertModels() → DELETE + INSERT in transaction
     │     ├─ saveModelMetadata() → LookupModelMetadata + UpdateModelMetadata
     │     └─ providerRepo.UpdateStatus("active" | "error")
     │
     └─ Return provider ID (or error)
```

### Switch Flow → Config Mutation

```
TUI (5-step form) → SwitchUseCases.BindProfile()
     │                    │
     │                    ├─ Validate env vars against target_clis.env_vars
     │                    ├─ Validate models exist in provider_models
     │                    └─ multiplexRepo.SetActive()
     │
     └─ SwitchUseCases.Apply()
              │
              ├─ Get CLI + multiplexes
              ├─ Resolve config path (tilde expansion)
              ├─ Parse mutator_config JSON
              ├─ For each binding:
              │    ├─ Get provider + models + metadata
              │    ├─ Build per-provider config with _registered_models, _model_metadata
              │    └─ mutator.Mutate()
              │         │
              │         ├─ CreateBackup() → centralized backup
              │         ├─ ReadJSONWithLock() → existing config
              │         ├─ Merge env vars / provider entry
              │         ├─ WriteAtomicJSON() → atomic write with flock
              │         └─ PruneBackups() → keep 5
              │
              └─ Return BackupResult
```

### Self-Update Flow

```
aimux version → CheckForUpdate()
     │              │
     │              ├─ CacheGet("latest_version") → 24h cached
     │              └─ fetchLatestRelease() → GitHub API
     │
aimux update → SelfUpdate()
     │              │
     │              ├─ checkWritePermission()
     │              ├─ fetchLatestRelease()
     │              ├─ semver comparison → skip if current ≥ latest
     │              ├─ isHomebrewInstall() → brew upgrade (delegate)
     │              ├─ download tar.gz → temp file
     │              ├─ extractBinary() → tar.gz reader
     │              ├─ validateChecksum() → SHA256
     │              ├─ atomicReplace() → rename chain
     │              └─ print result
```

---

## Key Design Decisions

### 1. Single binary, zero CGO

**Decision**: Use `modernc.org/sqlite` (pure Go SQLite) instead of `mattn/go-sqlite3` (CGO wrapper).

**Rationale**: Single static binary. No C toolchain. Cross-compilation without `CGO_ENABLED=1`. Smaller surface area for build issues.

**Trade-off**: Slightly slower than C-based SQLite. Negligible for aimux's data volume (~100 rows).

### 2. DDD layered architecture

**Decision**: `internal/domain/` (interfaces + value objects) → `internal/application/` (use cases) → `internal/infrastructure/` (repos, mutators, config) → `internal/tui/` (presentation).

**Rationale**: Testable at each layer. Domain interfaces mockable. Infrastructure swappable (e.g., PostgreSQL instead of SQLite). Mutation logic isolated from both storage and UI.

### 3. ConfigMutator interface per CLI

**Decision**: Each CLI gets its own mutator implementation behind a common `ConfigMutator` interface, registered by string key at startup.

**Rationale**: Adding a new CLI requires only:

1. New mutator file implementing `ConfigMutator`
2. Registry entry in `main.go`
3. Seed row in `SeedTargetCLIs`
4. OpenSpec specs

No changes to application or domain layers.

### 4. Centralized backups

**Decision**: Backups live in `~/.config/aimux/backups/<basename>-<hash>/` instead of alongside config files.

**Rationale**: Don't pollute CLI config directories. Files with same basename (e.g., multiple `settings.json`) don't collide. Hash of absolute path ensures uniqueness.

### 5. ANTHROPIC_AUTH_TOKEN over ANTHROPIC_API_KEY

**Decision**: Use `ANTHROPIC_AUTH_TOKEN` environment variable for Claude Code.

**Rationale**: Claude Code's OAuth login flow interferes with `ANTHROPIC_API_KEY`. Setting `AUTH_TOKEN` bypasses the login prompt. Key is removed from root level at every mutation (security invariant).

### 6. Trailing comma tolerance in JSON

**Decision**: When JSON parsing fails, retry after stripping trailing commas with regex.

**Rationale**: Users hand-edit `settings.json`. Trailing commas are the #1 syntax error. Lenient parsing prevents silent loss of configuration.

### 7. Flock for concurrent safety

**Decision**: Acquire shared lock before reading, exclusive lock before writing via `gofrs/flock`.

**Rationale**: Multiple processes (e.g., Claude Code itself + aimux) may access the same config file concurrently. Flock prevents TOCTOU races.

### 8. Atomic writes with temp file + rename

**Decision**: Write to `*.tmp`, sync, rename. Clean up temp on failure.

**Rationale**: Crash-safe. Never leaves a partially-written config file. POSIX `rename` is atomic on the same filesystem.

### 9. Multi-provider via composite PK

**Decision**: Schema migration from single `target_cli_id` PK to composite `(target_cli_id, provider_id)` with `ON CONFLICT DO UPDATE`.

**Rationale**: OpenCode, pi, and Copilot support multiple simultaneous providers. Single-row model forced switch-then-lose-previous. Composite PK allows N bindings per CLI.

### 10. Metadata catalog as Go constants

**Decision**: Hardcoded `KnownModelCatalog` map with 40+ model entries.

**Rationale**: Provider APIs often don't return full metadata (context window, costs, reasoning support). Catalog provides fallback. Pre-validated, zero-runtime-overhead. Add new entries as Go constants — no external file format.

**When to replace**: If catalog exceeds ~100 entries or users need to add custom models, replace with a JSON/YAML file loaded at startup.

### 11. Self-update with SHA256 validation

**Decision**: Download release tar.gz, extract binary, validate SHA256 against `checksums.txt`, atomic replace.

**Rationale**: Integrity guarantee. Attackers can't serve a tampered binary. `checksums.txt` is a standard goreleaser artifact.

### 12. Copilot via shell profile

**Decision**: Write `COPILOT_PROVIDER_*` env vars to shell profile files (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/config.fish`) wrapped in idempotent markers.

**Rationale**: Copilot reads process environment variables, not `.env` files. Shell profile injection is the only portable method. Markers enable safe removal.

---

## Error Handling

### Conventions

- **Sentinel errors**: `infrastructure/config/utils.go` defines 6 sentinel errors for common I/O failures.
- **Wrapping**: Use `fmt.Errorf("context: %w", err)` throughout. Never lose the root cause.
- **Non-fatal errors**: Model fetch failures are non-fatal — provider is saved with status `error` and user can retry later.
- **Loading guard**: TUI blocks navigation during async operations except `Ctrl+C`. No concurrent mutations.
- **Logging**: `aimux.log` at `~/.config/aimux/` captures all `log.Printf` calls with timestamps. TUI notifications surface errors visually.
- **HTTP errors**: `doModelFetch()` returns structured errors for 401/403, 429 (with Retry-After), 5xx, HTML responses (wrong URL).

### TUI Notification Severities

| Severity | Icon | TTL | Persists on Esc? |
|----------|------|-----|------------------|
| `info` | ✓ | 3s | No |
| `warn` | ⚠ | 5s | No |
| `error` | ✗ | ∞ | Yes (until any key press) |

---

## Concurrency Model

**Single-threaded by design**. Bubbletea's Elm architecture processes messages sequentially. No goroutines for business logic.

**Async operations** use Bubbletea commands (`tea.Cmd`):

- Model fetch (HTTP GET)
- Config apply (file I/O)
- Connectivity test (HTTP GET)
- Self-update (download + extract)

During async operations, the TUI shows a spinner and blocks all navigation input except `Ctrl+C` / `q`.

**Filesystem safety**: `gofrs/flock` with 2s timeout prevents concurrent mutation by external processes.

---

## Interfaces

### ProviderRepository

```go
Add(name, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) (int64, error)
Get(id int64) (Provider, error)
List() ([]Provider, error)
Update(id int64, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) error
UpdateStatus(id int64, status string) error
Delete(id int64) error
InsertModels(providerID int64, modelNames []string) error
DeleteModelsByProvider(providerID int64) error
ListModels(providerID int64) ([]ProviderModel, error)
ListAllModels() ([]ProviderModel, error)
UpdateModelMetadata(providerID int64, modelName string, metadata ModelMetadata) error
```

### TargetCLIRepository

```go
List() ([]TargetCLI, error)
Get(id int64) (TargetCLI, error)
Update(TargetCLI) error
```

### MultiplexRepository

```go
GetActive(targetCLIID int64) (*ActiveMultiplex, error)
SetActive(targetCLIID, providerID int64, modelMappingsJSON string) error
ClearActive(targetCLIID int64) error
ClearBinding(targetCLIID, providerID int64) error
ListActive() ([]ActiveMultiplex, error)
ListForCLI(targetCLIID int64) ([]ActiveMultiplex, error)
```

### ConfigMutator

```go
Mutate(configPath string, modelMappings map[string]string, provider Provider, mutatorConfig map[string]any) (*BackupResult, error)
```

---

## Package Contracts

### Adding a new CLI mutator

1. Create `internal/infrastructure/mutators/new_cli.go` implementing `domain.ConfigMutator`
2. Register in `main.go`'s `mutatorRegistry`
3. Add seed row in `sqlite.SeedTargetCLIs` with name, config_path, env_vars, mutator key, mutator_config
4. Write tests in `new_cli_test.go`
5. Add OpenSpec specs under `openspec/specs/`

### Adding a new provider type

1. Update `ProviderUseCases.FetchModels()` URL construction
2. Update `parseModelsResponse()` to handle new response format
3. Update `doModelFetch()` HTTP headers/params
4. Add test fixtures

### Adding model metadata

Add entry to `KnownModelCatalog` in `internal/infrastructure/config/model_catalog.go`. Use existing metadata key constants. Prefix match supported (e.g., `deepseek-v4-pro-20250601` matches `deepseek-v4-pro`).

### Database migrations

Add new migration function in `internal/infrastructure/sqlite/db.go`. Must be idempotent (check column/table exists before altering). Register in `main.go`'s migration chain (order matters). Old migrations never removed — they become no-ops for existing databases.
