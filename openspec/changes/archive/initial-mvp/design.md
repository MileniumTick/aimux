# Design: aimux — AI Multiplexer CLI (Initial MVP)

## Technical Approach

Single-binary Go CLI with a Bubble Tea TUI, local SQLite state, and direct JSON config file mutation. Three-layer architecture (TUI, business logic, data access) with pure-Go dependencies only (no CGo). The core operation is atomic config mutation: read a CLI's `settings.json`, inject an `"env"` block with mapped model variables, write atomically via temp-file + rename, and lock via `syscall.Flock`.

The spec defines the schema. **No `profiles` table** — model mappings are stored inline as JSON in `active_multiplex.model_mappings`. The "profile" concept is a TUI/business-layer abstraction: the user picks a provider and maps models to env vars in one Switch flow, and the result is stored directly as the active state.

## Architecture Decisions

### AD-01: No separate Profiles table — inline JSON mappings in active_multiplex

**Status**: Accepted

**Choice**: Store model-to-env-var bindings as a JSON blob in `active_multiplex.model_mappings` instead of a normalized `profiles` + `profile_models` schema.

**Alternatives considered**:
- Normalized `profiles` + `profile_models` tables with FK relationships — more expressive, supports saving multiple named mappings per CLI.

**Rationale**: Spec is authoritative. For MVP, the user always configures the mapping during a Switch, and there is no "reuse saved profile" scenario. Inline JSON eliminates join complexity, simplifies the code, and still supports all MVP user stories. A normalized profiles table can be added as a non-breaking migration in a future iteration.

**Consequence**: Model strings are denormalized into the JSON blob (not FK-referenced). If a model is removed from `provider_models` on re-fetch, the active mapping still works. Switching always involves rebuilding the mapping from scratch.

### AD-02: Business logic returns typed structs, not raw SQL rows

**Status**: Accepted

**Choice**: The business layer defines Go structs (Provider, Model, ActiveMultiplex, TargetCLI) and the data layer maps SQL rows into these structs.

**Alternatives considered**: Return raw `sql.Rows` or `map[string]any` from data layer — flexible but leaks SQL types into TUI code.

**Rationale**: Clear separation of concerns. The TUI calls `business/provider.ListProviders()` and gets `[]business.Provider` — it never touches `database/sql`. The data layer is the only package that imports `modernc.org/sqlite`.

**Consequence**: An explicit mapping layer between SQL rows and business structs. ~5-10 lines of mapping code per query. Acceptable for clarity.

### AD-03: Env vars configuration is data-driven, not hardcoded

**Status**: Accepted

**Choice**: The `target_clis.env_vars` column stores a JSON array of env var names. The TUI reads this at switch time to know which variables to prompt for.

**Alternatives considered**:
- Hardcode the 4 Claude Code env vars in Go source — simpler but prevents adding future CLIs without a code change.

**Rationale**: The spec mandates `env_vars` as a JSON array. Data-driven means adding a new CLI (future) only requires a DB row, not a recompilation. For MVP, only `claude-code` is seeded.

**Consequence**: The mapping form must dynamically generate Select prompts from the `env_vars` array. More dynamic UI logic, but the business layer is generic.

### AD-04: WAL mode for SQLite

**Status**: Accepted

**Choice**: Enable WAL journal mode on database open.

**Alternatives considered**: Default DELETE journal mode — simpler but slower for concurrent read/write.

**Rationale**: Spec mandates WAL mode. Single-user CLI means concurrency is minimal, but WAL avoids the "database is locked" error if the TUI renders a status table while a write is in progress from another call site.

**Consequence**: A `matrix.db-wal` and `matrix.db-shm` file coexist alongside `matrix.db`. These are cleaned up on clean shutdown. The `.gitignore` should exclude `*.db-wal` and `*.db-shm` patterns.

### AD-05: File locking via syscall.Flock with 2s timeout

**Status**: Accepted

**Choice**: Use `syscall.Flock` (exclusive lock for writes, shared lock for reads) with a 2-second timeout via a goroutine + timer.

**Alternatives considered**:
- `os.Rename` atomicity alone (no file-level lock) — sufficient for single-writer but unsafe if Claude Code itself writes to settings.json concurrently.
- `github.com/nightlyone/lockfile` — a third-party library for file locking; adds dependency.

**Rationale**: spec mandates flock. `syscall.Flock` is standard library, zero dependencies. The 2s timeout prevents the TUI from hanging if another process holds the lock.

**Consequence**: Requires an `*os.File` descriptor to lock. The file must be opened before locking. Unlock in deferred call.

## Package Structure

```
aimux/
├── main.go                        # Entry: DB init, seed, launch Bubble Tea
├── go.mod / go.sum                # Go module
├── internal/
│   ├── tui/
│   │   ├── model.go               # Main Bubble Tea model (composes sub-models)
│   │   ├── table.go               # Status table view (lipgloss table)
│   │   ├── menu.go                # Action menu (Switch / Manage Providers / Exit)
│   │   └── forms.go               # huh form factories (AddProvider, ModelMapping)
│   ├── business/
│   │   ├── provider.go            # ProviderService: Add, List, Delete, FetchModels
│   │   ├── switch.go              # SwitchService: SelectMapping, LoadConfig, MutateConfig
│   │   └── path.go                # PathResolver: Resolve relative to os.UserHomeDir
│   └── data/
│       ├── db.go                  # Open/Create DB, run migrations, seed target_clis
│       ├── queries.go             # All SQL query functions (CRUD for each table)
│       └── config.go              # JSON config file operations with flock + atomic write
```

## Component Diagram

```
    main.go: main()
       │
       ├── data/db.Open() ─────────► matrix.db (SQLite, WAL mode)
       │     └── SeedTargetCLIs() ──► target_clis row: claude-code
       │
       └── bubbletea.NewProgram(model) ──► TUI starts
                │
        ┌───────┴────────┐
        │  tui/model.go   │
        │  (Main Model)   │
        └───────┬────────┘
                │  Init() ──► business/provider.ListProviders()
                │             └──► data/queries.ListAll()
                │
                │  Update() handles messages:
                │   ├── MenuMsg{Switch} ──► tui/forms.go ──► business/switch.go
                │   ├── MenuMsg{Manage} ──► tui/forms.go ──► business/provider.go
                │   └── MenuMsg{Exit}  ──► tea.Quit
                │
                │  View() renders:
                │   ├── tui/table.go (lipgloss table from data/queries.ListActiveMultiplexes)
                │   └── tui/menu.go (action options + help text)
                │
                └── Message flow:
                    TUI Form ──► business.Service ──► data Layer ──► SQLite/JSON
                                   │                       │
                                   ▼                       ▼
                              business structs         matrix.db
                                                     settings.json
```

## Data Flows

### Flow 1: Dashboard Render (Init + Refresh)

```
tui/model.Init()
  └─► business/provider.ListProviders()
        └─► data/queries.ListProviders() ──► SQL: SELECT * FROM providers ORDER BY name
  └─► data/queries.ListActiveMultiplexes()
        └─► SQL: SELECT am.*, p.name AS provider_name, tc.name AS cli_name
                 FROM active_multiplex am
                 JOIN providers p ON am.provider_id = p.id
                 JOIN target_clis tc ON am.target_cli_id = tc.id
  └─► tui/table.Render(providers, activeMultiplexes) ──► lipgloss table
```

### Flow 2: Add Provider

```
User: Manage Providers → Add Provider
  └─► tui/forms.AddProviderForm() ──► huh form (name, baseURL, apiKey, authToken)
        └─► business/provider.Add(name, baseURL, apiKey, authToken)
              ├─► data/queries.AddProvider() ──► INSERT INTO providers
              └─► HTTP GET {baseURL}/v1/models (5s timeout, auth header)
                    ├─ Success ──► data/queries.InsertModels(providerID, modelNames)
                    │               └─► BEGIN; DELETE WHERE provider_id = ?; INSERT ...; COMMIT
                    └─ Failure ──► data/queries.UpdateProviderStatus(id, "error")
  └─► tui/model sends DashboardRefreshMsg
```

### Flow 3: Switch (Profile Activate + Config Mutate)

```
User: Switch
  └─► tui/forms.SelectTargetCLI() ──► huh.Select from business/cli.ListAll()
  └─► tui/forms.SelectProvider() ──► huh.Select from providers with status=active
  └─► tui/forms.MapModels(envVars, providerModels) ──► huh.Select chain, one per env var
        └─► Returns map[string]string: {"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4", ...}
  └─► business/switch.Apply(targetCLI, providerID, modelMappings)
        ├─ 1. data/queries.SetActiveMultiplex(cliID, providerID, jsonMappings)
        ├─ 2. data/config.ResolvePath(targetCLI.config_path) ──► os.UserHomeDir() + relative
        ├─ 3. data/config.ReadWithLock(path)
        │      ├─ syscall.Flock(fd, LOCK_EX) with 2s timeout
        │      ├─ json.Decode into map[string]any
        │      └─ syscall.Flock(fd, LOCK_UN)
        ├─ 4. Mutate map:
        │      ├─ Delete key "ANTHROPIC_API_KEY" (root)
        │      ├─ Ensure map["env"] is map[string]any
        │      └─ For each (envVar, modelName): map["env"][envVar] = modelName
        ├─ 5. data/config.WriteAtomic(path, mutatedMap)
        │      ├─ json.MarshalIndent(map, "", "  ")
        │      ├─ Write to temp file in same directory (os.CreateTemp)
        │      └─ os.Rename(tempFile, originalPath)
        └─ 6. Return success
  └─► tui/model sends DashboardRefreshMsg
```

### Flow 4: First Run (Empty State)

```
tui/model.Init()
  └─► data/queries.ListProviders() ──► empty result (no providers)
  └─► data/queries.ListActiveMultiplexes() ──► empty result
  └─► tui/table.Render() ──► empty table with hint: "No providers configured"
  └─► tui/menu.Render() ──► highlights: "Manage Providers → Add Provider"
```

## SQLite Schema (Exact DDL)

```sql
CREATE TABLE IF NOT EXISTS providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    api_key TEXT NOT NULL DEFAULT '',
    auth_token TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'error')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    UNIQUE(provider_id, model_name)
);

CREATE TABLE IF NOT EXISTS target_clis (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    config_path TEXT NOT NULL,
    env_vars TEXT NOT NULL  -- JSON array of env var names
);

CREATE TABLE IF NOT EXISTS active_multiplex (
    target_cli_id INTEGER PRIMARY KEY REFERENCES target_clis(id) ON DELETE CASCADE,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_mappings TEXT NOT NULL,  -- JSON object: {"ENV_VAR": "model-string", ...}
    activated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON provider_models(provider_id);
```

### Seed Data

```sql
INSERT OR IGNORE INTO target_clis (name, config_path, env_vars)
VALUES (
    'claude-code',
    'Library/Application Support/Claude/claude_desktop_config.json',
    '["ANTHROPIC_DEFAULT_HAIKU_MODEL","ANTHROPIC_DEFAULT_SONNET_MODEL","ANTHROPIC_DEFAULT_OPUS_MODEL","CLAUDE_CODE_SUBAGENT_MODEL"]'
);
```

## Config Mutation Algorithm

```go
// internal/data/config.go

// MutateAndWrite atomically injects an "env" block into a JSON config file.
// 1. Open file, acquire LOCK_EX (2s timeout)
// 2. Read and parse JSON into map[string]any
// 3. Remove "ANTHROPIC_API_KEY" from root (security)
// 4. Ensure "env" is a map[string]any (create if absent)
// 5. Set each env var from modelMappings
// 6. Remove "ANTHROPIC_API_KEY" from env block if present
// 7. Marshal to indented JSON
// 8. Write to tempfile, os.Rename over original
// 9. Unlock and close
func MutateAndWrite(path string, modelMappings map[string]string) error
```

Key invariants:
- **Atomicity**: `os.Rename()` on POSIX is atomic within the same filesystem. Temp file is created in the same directory as the target.
- **Idempotency**: Calling Switch with the same provider+models twice produces the same result. The `"env"` block is overwritten, not merged.
- **Preservation**: All root-level non-env keys are preserved verbatim. Global settings (e.g. `"allowWrites"`) survive the mutation.
- **Security**: `ANTHROPIC_API_KEY` is always removed from both root and env blocks on every switch.

## Error Handling Strategy

| Layer | Error Source | Handling | TUI Presentation |
|-------|-------------|----------|------------------|
| Data | SQLite constraint violation (duplicate name) | Return typed error | Hub form: "Provider 'X' already exists" |
| Data | SQLite connection failure | Return error, main() exits | No TUI start — print to stderr |
| Data | File not found | Return ErrConfigNotFound | TUI alert: "Config file not found at PATH" |
| Data | JSON parse error | Return ErrConfigParse | TUI alert: "Could not parse PATH — invalid JSON" |
| Data | Flock timeout (>2s) | Return ErrFlockTimeout | TUI alert: "Could not lock PATH — another process is writing" |
| Business | HTTP fetch timeout (5s) | Store provider with status=error | Status table shows "Fetch Failed" badge |
| Business | HTTP fetch 401/403 | Store provider with status=error + error_message | Status table shows error message |
| TUI | Empty provider list during Switch | Pre-check, show "Add a provider first" | Menu disables Switch, shows hint |
| TUI | Empty model list for a provider | Pre-check, show "No models fetched" | Select form shows "No models — retry fetch" |

## File Organization

| File | Lines (est.) | What it contains |
|------|-------------|------------------|
| `main.go` | 40 | `main()`: create data dir, open DB, seed, create TUI model, `bubbletea.NewProgram()`, handle Fatal errors |
| `internal/tui/model.go` | 120 | Main `Model` struct, `Init()`, `Update()` with message routing, `View()` composing table+menu, refresh on DB change |
| `internal/tui/table.go` | 80 | `RenderTable(providers, activeMultiplexes)`: lipgloss table with columns: CLI, Provider, Model Mappings, Status. Empty-state hint. |
| `internal/tui/menu.go` | 60 | `RenderMenu()`: action list (Switch, Manage Providers, Exit). Disable Switch when no providers. Keyboard shortcuts. |
| `internal/tui/forms.go` | 180 | `AddProviderForm()`, `SelectTargetCLIForm()`, `SelectProviderForm()`, `MapModelsForm(envVars, models)` — returns form results via callbacks/messages |
| `internal/business/provider.go` | 100 | `ProviderService` interface + impl. `Add()`, `List()`, `Get()`, `Delete()`, `FetchModels(baseURL, apiKey, authToken)` |
| `internal/business/switch.go` | 80 | `SwitchService.Apply(targetCLI, providerID, modelMappings)` — orchestrates SetActiveMultiplex + config mutation |
| `internal/business/path.go` | 30 | `ResolvePath(relativePath string) string` — `filepath.Join(os.UserHomeDir(), relativePath)` |
| `internal/data/db.go` | 80 | `Open(path string) (*sql.DB, error)` — open/create, set WAL mode, run CREATE TABLEs, `SeedTargetCLIs()` |
| `internal/data/queries.go` | 200 | All CRUD functions: `AddProvider`, `ListProviders`, `GetProvider`, `DeleteProvider`, `InsertModels`, `ListModels`, `ListAllModels`, `SetActiveMultiplex`, `GetActiveMultiplex`, `ListActiveMultiplexes`, `ListTargetCLIs` |
| `internal/data/config.go` | 120 | `ReadWithLock`, `MutateAndWrite`, `writeAtomic`, `acquireFlock` helper with timeout |
| **Total** | **~1090** | |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Data (queries) | All CRUD operations | In-memory SQLite (`:memory:`) test helpers. Verify INSERT, SELECT, UPDATE, DELETE, CASCADE, UNIQUE violations. |
| Data (config) | Flock acquire/release, timeout, atomic write | Temp dir test fixtures. Test concurrent flock contention. Test atomic write by reading before/after. |
| Business (provider) | FetchModels with mock HTTP | `httptest.NewServer` for `/v1/models` endpoint. Test success, timeout, 401, malformed response. |
| Business (switch) | Config mutation logic | In-memory JSON fixtures. Test with various structures: with/without `env` block, with `ANTHROPIC_API_KEY`, with extra keys. |
| TUI | Model message routing | Bubble Tea testing via `tea.NewProgram` with `WithInput`/`WithOutput`. Test Init state, test update message sequences. |

## Migration / Rollout

No migration required. This is the initial schema — the `CREATE TABLE IF NOT EXISTS` statements run on every startup. The DB file is created at `~/.config/aimux/matrix.db` on first launch.

## Open Questions

- [ ] Linux vs macOS config path for Claude Code: spec says `~/.config/claude/settings.json` for Linux. The seeded path should be platform-aware or configurable.
- [ ] `auth_token` vs `api_key` for /v1/models requests — which header format? OpenAI uses `Authorization: Bearer <key>`, Anthropic uses `x-api-key`. Should the fetch function try both or let the user choose?
