# Tasks: aimux — AI Multiplexer CLI (Initial MVP)

## Dependency Graph

```
Phase 1 (Scaffold)
  │
  ├──> Phase 2a: business/path.go (no deps)
  ├──> Phase 2b: data/db.go (no deps beyond sqlite driver)
  ├──> Phase 2c: data/queries.go [needs 2b]
  ├──> Phase 2d: data/config.go (no deps beyond stdlib)
  │
  ├──> Phase 3a: business/provider.go [needs 2a, 2c]
  ├──> Phase 3b: business/switch.go [needs 2a, 2c, 2d]
  │
  ├──> Phase 4a: tui/model.go [needs 3a, 3b]
  ├──> Phase 4b: tui/table.go [needs business structs — can start after 3a]
  ├──> Phase 4c: tui/menu.go [needs business layer — can start after 3a]
  ├──> Phase 4d: tui/forms.go [needs business layer — can start after 3a]
  │
  ├──> Phase 5: main.go wiring [needs all Phase 4]
  │
  └──> Phase 6: Tests [parallel groups can start after their respective phases]
```

**Parallel groups** (no cross-dependency):
- T1, T2a, T2b, T2d can all run in parallel.
- T2c depends on T2b only — can run in parallel with T2a, T2d.
- T3a and T3b can run in parallel (once their data deps are ready).
- T4a–T4d can run in parallel (once business layer is ready).

---

## Phase 1: Project Scaffold

### [x] T1 — Initialize Go module and directory structure

**Specs satisfied**: All (foundational)
**Depends on**: Nothing
**Parallel with**: Nothing (all other tasks depend on directory structure)

- Run `go mod init github.com/jchavarriam/aimux`
- Create directory structure:
  ```
  aimux/
  ├── main.go                 (skeleton — just package main + empty main())
  ├── internal/tui/           (empty dir)
  ├── internal/business/      (empty dir)
  └── internal/data/          (empty dir)
  ```
- Add `go.mod` with explicit dependency pinning for:
  - `modernc.org/sqlite` (pure Go SQLite)
  - `github.com/charmbracelet/bubbletea`
  - `github.com/charmbracelet/huh`
  - `github.com/charmbracelet/lipgloss`
- Run `go mod tidy` to resolve transitive deps.
- Verify `go build ./...` succeeds (empty main is fine).

---

## Phase 2a: Path Resolution (business/path.go)

### [x] T2a — Implement PathResolver

**Specs satisfied**: path/spec.md (all acceptance scenarios)
**Depends on**: T1 (directory structure)
**Parallel with**: T2b, T2d

- Implement `business/path.go`:
  - `ResolvePath(path string) (string, error)`: tilde expansion via `os.UserHomeDir()` exclusively. No shell invocation. Cache `os.UserHomeDir()` once.
  - `ResolveConfigPath() string`: returns `ResolvePath("~/.config/aimux/matrix.db")`, calls `os.MkdirAll` with 0700 on first use.
  - `ResolveTargetConfigPath(targetCLIConfigPath string) string`: resolves a stored config path using `ResolvePath`.
- Constraints: NO `os.Getenv("HOME")`, NO `os/exec.Command`, NO shell-based path expansion.

---

## Phase 2b: SQLite Schema + Migration (data/db.go)

### [x] T2b — Implement database open, migration, and seed

**Specs satisfied**: storage/spec.md (Schema Creation, File Permissions)
**Depends on**: T1 (directory structure)
**Parallel with**: T2a, T2d

- Implement `data/db.go`:
  - `Open(path string) (*sql.DB, error)`: open SQLite, enable WAL journal mode, set `PRAGMA journal_mode=WAL`.
  - `RunMigrations(db *sql.DB) error`: execute all four `CREATE TABLE IF NOT EXISTS` DDL statements exactly as specified in the design:
    - `providers` (with CHECK on status IN 'active','error')
    - `provider_models` (with FK CASCADE, UNIQUE(provider_id, model_name))
    - `target_clis` (env_vars as TEXT storing JSON array)
    - `active_multiplex` (target_cli_id as PK FK, model_mappings as TEXT storing JSON object)
  - `CreateIndexes(db *sql.DB) error`: create `idx_provider_models_provider_id`.
  - `SeedTargetCLIs(db *sql.DB) error`: INSERT OR IGNORE the `claude-code` row with `config_path = "~/.config/claude/settings.json"` and `env_vars = '["ANTHROPIC_DEFAULT_HAIKU_MODEL","ANTHROPIC_DEFAULT_SONNET_MODEL","ANTHROPIC_DEFAULT_OPUS_MODEL","CLAUDE_CODE_SUBAGENT_MODEL"]'`.
  - Set DB file permissions to 0600, parent directory to 0700.

---

## Phase 2c: CRUD Queries (data/queries.go)

### [x] T2c — Implement all CRUD query functions

**Specs satisfied**: storage/spec.md (all Operations, all Acceptance Scenarios)
**Depends on**: T2b (needs DB schema for correct query writing)
**Parallel with**: T2d

- Implement `data/queries.go` with all 12 query functions:
  1. `AddProvider(db, name, baseURL, apiKey, authToken) -> (int64, error)` — INSERT, return last insert ID
  2. `GetProvider(db, id) -> (Provider, error)` — SELECT by id
  3. `ListProviders(db) -> ([]Provider, error)` — SELECT all ORDER BY name ASC
  4. `UpdateProviderStatus(db, id, status) -> error` — UPDATE status + updated_at
  5. `DeleteProvider(db, id) -> error` — DELETE (CASCADE deletes models + active_multiplex)
  6. `InsertModels(db, providerID, modelNames []string) -> error` — BEGIN; DELETE WHERE provider_id; INSERT; COMMIT
  7. `ListModels(db, providerID) -> ([]Model, error)` — SELECT by provider_id ORDER BY model_name ASC
  8. `ListAllModels(db) -> ([]Model, error)` — SELECT with join on providers for provider_name
  9. `GetActiveMultiplex(db, targetCLIID) -> (ActiveMultiplex, error)` — SELECT single row, return nil/empty struct if not found (not error)
  10. `SetActiveMultiplex(db, targetCLIID, providerID, modelMappingsJSON) -> error` — INSERT OR REPLACE
  11. `ClearActiveMultiplex(db, targetCLIID) -> error` — DELETE
  12. `ListActiveMultiplexes(db) -> ([]ActiveMultiplex, error)` — SELECT with JOIN on providers + target_clis for names
- Define Go structs: `Provider`, `Model`, `TargetCLI`, `ActiveMultiplex` in this package (used by business layer).

---

## Phase 2d: JSON Config File Operations (data/config.go)

### [x] T2d — Implement flock-based file read/write with atomic rename

**Specs satisfied**: storage/spec.md (File Locking section), switch/spec.md (Read Phase, Write Phase, Error Recovery)
**Depends on**: T1 (directory structure)
**Parallel with**: T2a, T2b

- Implement `data/config.go`:
  - `ReadJSONWithLock(path string) (map[string]any, error)`: open file, acquire `LOCK_SH` with 2s timeout via goroutine+timer, read + JSON decode into `map[string]any`, unlock. If file not found, return empty `map[string]any{}` and nil error. If file is empty or invalid JSON, return empty `map[string]any{}` (no error — per switch spec).
  - `MutateAndWrite(path string, modelMappings map[string]string, apiKey string) error`:
    1. Open file, acquire `LOCK_EX` with 2s timeout
    2. Read + JSON decode into `map[string]any`
    3. Delete key `"ANTHROPIC_API_KEY"` from root
    4. Build `"env"` map: iterate `modelMappings`, skip empty values, set non-empty; add `"ANTHROPIC_API_KEY": apiKey`
    5. Set root `"env"` = built map (overwrite, not merge)
    6. Marshal to `json.MarshalIndent(root, "", "  ")` + trailing newline
    7. Write to temp file in same directory (`os.CreateTemp`)
    8. `file.Sync()` on temp file
    9. `os.Rename(tempFile, originalPath)`
    10. Release `LOCK_UN` (deferred)
  - `acquireFlock(fd, lockType, timeout) error`: helper with goroutine + timer pattern
  - Error handling per the switch spec's Error Recovery table (typed errors for lock timeout, temp file failure, write failure, rename failure, sync failure).

---

## Phase 3a: Provider Business Logic (business/provider.go)

### [x] T3a — Implement ProviderService

**Specs satisfied**: provider/spec.md (all sections), part of tui/spec.md (Manage Providers flow requirements)
**Depends on**: T2a (path), T2c (queries)
**Parallel with**: T3b

- Implement `business/provider.go`:
  - `ProviderService` struct that holds `*sql.DB`.
  - `type Provider struct` — mirrors data layer struct (or use data layer struct directly via import).
  - `Add(name, baseURL, apiKey, authToken string) (int64, error)` — calls data.AddProvider, then triggers fetch.
  - `List() ([]Provider, error)` — calls data.ListProviders.
  - `Get(id int64) (Provider, error)` — calls data.GetProvider.
  - `Delete(id int64) error` — calls data.DeleteProvider.
  - `FetchModels(providerID int64, baseURL, authToken string) error` — HTTP GET `{baseURL}/v1/models`:
    - 5s timeout via `http.Client.Timeout`
    - Auth header: `Authorization: Bearer {auth_token}`
    - Accept: `application/json`, User-Agent: `aimux/0.1.0`
    - Parse response: try `data[].id` (OpenAI format), fallback to `models[].id` (Anthropic format)
    - Store models via data.InsertModels
    - On success: data.UpdateProviderStatus(id, "active")
    - On failure: data.UpdateProviderStatus(id, "error"), return typed error
  - `RetryFetch(providerID int64) error` — re-fetch using stored baseURL + authToken from DB.
- Error messages per provider/spec.md Error Handling table.

---

## Phase 3b: Switch Business Logic (business/switch.go)

### [x] T3b — Implement SwitchService

**Specs satisfied**: mapping/spec.md (all sections), switch/spec.md (all sections)
**Depends on**: T2a (path), T2c (queries), T2d (config ops)
**Parallel with**: T3a

- Implement `business/switch.go`:
  - `SwitchService` struct that holds `*sql.DB`.
  - `Apply(targetCLIID, providerID int64) error`:
    1. Read provider from DB (get api_key)
    2. Read target CLI from DB (get config_path)
    3. Get active multiplex for targetCLIID (get model_mappings JSON)
    4. Resolve config path via path resolver
    5. Call `data/config.MutateAndWrite(resolvedPath, parsedMappings, providerAPIKey)`
  - `ListTargetCLIs() ([]TargetCLI, error)` — calls data for target_clis list
  - `GetActiveForCLI(targetCLIID int64) (*ActiveMultiplex, error)` — calls data.GetActiveMultiplex
- Implement mapping/business logic functions:
  - `BindProfile(targetCLIID, providerID int64, mappings map[string]string) error`:
    - Validate keys are subset of target CLI's env_vars
    - Validate provider status = 'active'
    - Validate each non-empty model ID exists in provider_models
    - Call data.SetActiveMultiplex
  - `GetBoundModels(targetCLIID int64) (map[string]string, error)` — calls data.GetActiveMultiplex, parses model_mappings JSON, returns map
  - `GetProviderForCLI(targetCLIID int64) (int64, error)` — calls data.GetActiveMultiplex, returns provider_id

---

## Phase 4a: TUI Main Model (tui/model.go)

### [x] T4a — Implement Bubble Tea main model with message routing

**Specs satisfied**: tui/spec.md (Main Loop, Keyboard Model, Empty State)
**Depends on**: T3a, T3b (needs business service types)
**Parallel with**: T4b, T4c, T4d

- Implement `tui/model.go`:
  - `type model struct` — composes:
    - `providers []business.Provider`
    - `activeMultiplexes []business.ActiveMultiplex`
    - `menuSelected int` — current menu selection index
    - `currentView viewType` — enum: `dashboardView`, `providerListView`, `addProviderView`, `switchView`
  - `Init() tea.Cmd` — query DB for providers + active multiplexes, set initial state
  - `Update(msg tea.Msg) (tea.Model, tea.Cmd)` — message routing:
    - `tea.WindowSizeMsg` → update layout dimensions
    - `tea.KeyMsg{Enter}` → handle menu selection / form confirmation
    - `tea.KeyMsg{Esc}` → return to dashboard from sub-views
    - `tea.KeyMsg{q}` → tea.Quit (at dashboard level only)
    - `DashboardRefreshMsg` → re-query DB, re-render table
    - `SwitchToViewMsg{viewType}` → change current view
    - Form result messages → call business layer, dispatch refresh
  - `View() string` — render current view: delegates to table + menu for dashboard, or form views
- Define message types: `DashboardRefreshMsg`, `SwitchToViewMsg`, `FormResultMsg`
- Refresh from DB before every render cycle (no caching per spec).

---

## Phase 4b: Status Table (tui/table.go)

### [x] T4b — Implement lipgloss status table

**Specs satisfied**: tui/spec.md (Status Table, Empty State)
**Depends on**: T4a (model struct types), T2a+T2c (business structs)
**Parallel with**: T4c, T4d

- Implement `tui/table.go`:
  - `func RenderTable(providers []business.Provider, activeMultiplexes []business.ActiveMultiplex) string`
  - Columns: **CLI** | **Provider** | **Models** | **Status**
  - **CLI**: name from target_clis (hardcoded row for MVP: `claude-code`)
  - **Provider**: provider name from active multiplex, or `---`
  - **Models**: comma-separated model IDs from model_mappings JSON, or `---`
  - **Status**: `ACTIVE` (green) or `INACTIVE` (dim)
  - lipgloss styling: alternating row colors, bold header, border around table
  - Empty state: show single row with `---` / `INACTIVE`, plus hint line "No providers configured. Use 'Manage Providers' to add one."
  - Handle terminal resize via lipgloss width-aware rendering.

---

## Phase 4c: Action Menu (tui/menu.go)

### [x] T4c — Implement action menu with keyboard navigation

**Specs satisfied**: tui/spec.md (Action Menu, Keyboard Model)
**Depends on**: T4a (model struct types)
**Parallel with**: T4b, T4d

- Implement `tui/menu.go`:
  - `func RenderMenu(selectedIndex int, hasProviders bool) string`
  - Three items: **Switch**, **Manage Providers**, **Exit**
  - Vertical list with lipgloss styling: selected item highlighted, others dimmed
  - **Switch** disabled (greyed out, no selection via keyboard skip) when `hasProviders` is false
  - Keyboard: Up/Down or j/k to navigate, Enter to select
  - `func MenuItemCount() int` returns 3
  - Help text footer: "j/k - navigate | Enter - select | q - quit"

---

## Phase 4d: Form Factories (tui/forms.go)

### [x] T4d — Implement all huh form factories

**Specs satisfied**: tui/spec.md (Add Provider Form, Model Mapping Form, Delete Provider, Switch Profile flow)
**Depends on**: T3a, T3b (needs business types and service functions)
**Parallel with**: T4b, T4c

- Implement `tui/forms.go` with form factory functions:
  1. `AddProviderForm() *huh.Form` — 4 fields:
     - Name (text, required, validated non-empty)
     - Base URL (text, required, validated as parseable URL)
     - API Key (text, masked via `EchoModePassword`, required, validated non-empty)
     - Auth Token (text, masked, required, validated non-empty)
  2. `ConfirmDeleteProviderForm(name string) *huh.Confirm` — "Delete [name]? This will remove all associated models and active mappings."
  3. `SelectTargetCLIForm(clis []business.TargetCLI) *huh.Select` — list of registered CLIs
  4. `SelectProviderForm(providers []business.Provider) *huh.Select` — list of active providers only
  5. `MapModelsForm(envVars []string, models []business.Model) *huh.Form` — dynamic Select chain, one per env var:
     - Each Select offers all model IDs plus "Not Selected" (empty string) option
     - Returns `map[string]string` of envVar->modelID
  - Each form returns its result via a closure or callback that sends a `FormResultMsg` to the main model.

---

## Phase 5: Wiring (main.go)

### [x] T5 — Wire main.go with full initialization

**Specs satisfied**: All (runtime integration)
**Depends on**: T4a, T4b, T4c, T4d (complete TUI)
**Sequential after**: Phase 4 (last phase before testing)

- Implement `main.go`:
  1. Call `path.ResolveConfigPath()` to get DB path
  2. Ensure config directory exists (`os.MkdirAll` with 0700)
  3. `data.Open(dbPath)` — open SQLite with WAL mode
  4. `data.RunMigrations(db)` — create tables
  5. `data.SeedTargetCLIs(db)` — seed claude-code
  6. Create `business.ProviderService{DB: db}`
  7. Create `business.SwitchService{DB: db}`
  8. Create `tui.NewModel(providerSvc, switchSvc)` — initial TUI model
  9. `bubbletea.NewProgram(model, opts...)` — start program
  10. Handle panic/error: print to stderr if DB init fails, exit non-zero
- Do NOT bundle provider services or switch services — pass them to the model via dependency injection.
- Set `os.Exit(0)` on clean shutdown.

---

## Phase 6: Tests

### [x] T6a — Data layer query tests

**Specs satisfied**: All Acceptance Scenarios in storage/spec.md
**Depends on**: T2b, T2c (code must exist)
**Parallel with**: T6b, T6c, T6d

- `internal/data/queries_test.go`:
  - Use in-memory SQLite (`:memory:`) for each test.
  - Run migrations on each test's DB.
  - Test all CRUD functions:
    - AddProvider success / duplicate name error
    - ListProviders returns sorted providers
    - UpdateProviderStatus
    - DeleteProvider (verify CASCADE deletes models + active_multiplex)
    - InsertModels (clear + re-insert semantics)
    - ListModels / ListAllModels
    - SetActiveMultiplex (INSERT OR REPLACE)
    - GetActiveMultiplex (found / not found — empty struct, not error)
    - ClearActiveMultiplex
    - ListActiveMultiplexes with JOIN data
    - SeedTargetCLIs idempotency

### [x] T6b — Data layer config file tests

**Specs satisfied**: storage/spec.md (File Locking), switch/spec.md (Error Recovery)
**Depends on**: T2d (code must exist)
**Parallel with**: T6a, T6c, T6d

- `internal/data/config_test.go`:
  - Temp dir test fixtures.
  - Test `ReadJSONWithLock`: existing file, empty file, non-existent file, invalid JSON.
  - Test `MutateAndWrite`: full config injection, partial mappings (empty values omitted), API key cleanup from root, preservation of existing root keys.
  - Test atomic write: verify temp file created and renamed, original path is valid during write.
  - Test flock contention: simulate another process holding LOCK_EX, verify 2s timeout error.
  - Test with various settings.json structures: with env block, without env block, with extra keys.

### [x] T6c — Business layer provider tests

**Specs satisfied**: provider/spec.md (all Acceptance Scenarios)
**Depends on**: T3a (code must exist)
**Parallel with**: T6a, T6b, T6d

- `internal/business/provider_test.go`:
  - Use `httptest.NewServer` to mock `/v1/models` endpoint.
  - Test successful fetch — OpenAI format (`{"data": [...]}`)
  - Test successful fetch — Anthropic fallback format (`{"models": [...]}`)
  - Test HTTP 401 → error with "Authentication failed" message
  - Test HTTP 429 → error with "Rate limited" message
  - Test HTTP 5xx → error
  - Test timeout → error with "timed out" message
  - Test unparseable response → error
  - Test retry fetch after error: success clears error status, failure preserves it

### [x] T6d — Business layer switch tests

**Specs satisfied**: mapping/spec.md (all Acceptance Scenarios), switch/spec.md (all Acceptance Scenarios)
**Depends on**: T3b (code must exist)
**Parallel with**: T6a, T6b, T6c

- `internal/business/switch_test.go`:
  - Test `BindProfile` with all four mappings → SetActiveMultiplex called correctly
  - Test partial binding (empty values stored) → empty mappings stored in JSON
  - Test unknown model ID → error, no active multiplex created
  - Test unknown env var key → error, no active multiplex created
  - Test `GetBoundModels` with no active profile → empty map, no error
  - Test `Apply` flow end-to-end with in-memory JSON fixtures:
    - Existing settings.json with global keys + env block
    - Non-existent settings.json (created)
    - Partial mappings (empty values omitted from env block)
    - API key cleanup: root `ANTHROPIC_API_KEY` removed

### [x] T6e — TUI model tests (optional, time-permitting)

**Specs satisfied**: tui/spec.md (keyboard model, empty state)
**Depends on**: T4a, T4b, T4c, T4d
**Can start after**: Phase 4 complete

- `internal/tui/tui_test.go`:
  - Using `tea.NewProgram(model)` with `tea.WithInput`/`tea.WithOutput`.
  - Test Init state: verify providers and active multiplexes queried.
  - Test empty state: no providers → status table shows INACTIVE, menu defaults to "Manage Providers".
  - Test keyboard: Up/Down navigation in menu, Enter selection, Esc back.
  - Test q key exits at dashboard level.

---

## Review Workload Forecast

| Metric | Value |
|--------|-------|
| **Estimated source files** | 11 Go files (main.go + 10 internal) |
| **Estimated production LOC** | ~1090 lines (per design estimate) |
| **Estimated test files** | 5 Go test files |
| **Estimated test LOC** | ~400-500 lines |
| **Total estimated changed lines** | **~1490-1590 lines** |
| **Chained PRs recommended** | **No** — overridden by `delivery_strategy: exception-ok`. Single PR approved. |
| **Decision needed before apply** | **No** — exception-ok granted. |
| **Risk level** | Medium — high line count for a single PR but all code is new (no regression risk on existing code). Primary risk is TUI-business integration issues surfacing late (Phase 5). Mitigation: Phase 6 tests for each layer independently before wiring. |
| **Bottleneck tasks** | T2c (queries) is a gate for T3a and T3b → running T2c in parallel with T2a/T2d minimizes the wait. T3a/T3b gate the entire Phase 4 → prioritize business layer completion. |
