# aimux — Agent Guide

> **What**: Single-binary TUI + CLI that centralizes AI provider credentials and routes them to dev CLIs (Claude Code, OpenCode, Codex, Copilot, pi) via config file mutation.
>
> **Stack**: Go 1.25, Bubbletea TUI, modernc.org/sqlite (pure Go, no CGO), Synthwave Outrun theme.
>
> **Architecture**: DDD with 4 layers: Domain → Application → Infrastructure → TUI (inward dependency).

---

## Essential Commands

```bash
# Build (dev)
go build -o aimux .

# Build with version
go build -ldflags "-X main.version=0.3.0" -o aimux .

# Run
./aimux                          # TUI mode (default)
./aimux apply claude-code        # CLI mode
./aimux list                     # list active bindings

# Test (use -count=1 to bypass Go cache)
go test ./... -v -count=1
go test ./internal/application/ -v -count=1

# Test with race detector
go test ./... -race -count=1

# Coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Lint
# No linter is configured — rely on `go vet`:
go vet ./...

# Cross-platform build
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o aimux .

# Goreleaser snapshot
goreleaser build --snapshot --clean
```

---

## Project Structure

```
.
├── main.go                           Entrypoint, DB setup, migration chain, mutator registry, CLI routing
├── internal/
│   ├── domain/                       Pure types + interfaces (no infra imports)
│   │   ├── provider.go               Provider, ProviderModel, ModelMetadata, ProviderRepository interface
│   │   ├── targetcli.go              TargetCLI, ConfigMutator interface, BackupResult
│   │   └── multiplex.go              ActiveMultiplex, MultiplexRepository interface
│   ├── application/                  Business logic / use cases
│   │   ├── path.go                   Tilde expansion, config dir resolution, log file setup
│   │   ├── provider_svc.go           ProviderUseCases (CRUD, FetchModels, TestConnectivity)
│   │   ├── multiplex_svc.go          SwitchUseCases (Apply, DryRun, BindProfile, Backups/Restore)
│   │   ├── helpers_test.go           In-memory DB setup, seed data, test harnesses
│   │   ├── provider_svc_test.go
│   │   └── multiplex_svc_test.go
│   ├── infrastructure/
│   │   ├── config/
│   │   │   ├── utils.go              AtomicWrite, ReadJSONWithLock, CreateBackup, RestoreBackup, flock
│   │   │   ├── model_catalog.go      Hardcoded known-model metadata (40+ entries, prefix-matched)
│   │   │   └── utils_test.go
│   │   ├── mutators/                 Filesystem config mutators (one per target CLI)
│   │   │   ├── claude_json.go        claude-settings-json
│   │   │   ├── opencode_json.go      opencode-provider-json
│   │   │   ├── codex_toml.go         codex-config-toml
│   │   │   ├── copilot_shell.go      copilot-shell-profile
│   │   │   ├── pi_dual.go            pi-dual-json
│   │   │   └── *_test.go             Per-mutator tests
│   │   ├── sqlite/
│   │   │   ├── db.go                 Open(), RunMigrations, migration functions, seed data, indexes
│   │   │   ├── provider_repo.go      ProviderRepository SQLite impl
│   │   │   ├── targetcli_repo.go     TargetCLIRepository SQLite impl
│   │   │   ├── multiplex_repo.go     MultiplexRepository SQLite impl
│   │   │   └── queries_test.go       Integration tests for repos
│   │   └── update/
│   │       ├── checker.go            GitHub Releases API semver check
│   │       ├── cache.go              24h cache via SQL expiry
│   │       ├── updater.go            Download, SHA256 validate, atomic replace, Homebrew fallback
│   │       └── models.go             UpdateInfo struct
│   └── tui/
│       ├── model.go                  Core Bubbletea model (state machine with ~20 view types)
│       ├── forms.go                  huh form builders (AddProvider, EditProvider, Switch flows)
│       ├── table.go                  Dashboard table renderer
│       ├── menu.go                   Menu rendering
│       ├── theme.go                  Synthwave palette constants + lipgloss styles
│       ├── stepper.go                Switch flow 5-step indicator
│       ├── logo.go                   ASCII art logo
│       ├── truncate.go               Unicode-aware text truncation
│       └── tui_test.go
├── docs/
│   ├── DEVELOPMENT.md                Build, test, release, add-mutator guide
│   ├── DATABASE.md                   Schema, migrations, seed data
│   ├── SECURITY.md                   Threat model
│   └── manual-de-usuario.md          Spanish user manual
├── openspec/                         SDD (Spec-Driven Development) artifacts
│   ├── config.yaml
│   ├── specs/                        Domain, infra, TUI specs
│   └── changes/                      Change archives
├── .github/workflows/release.yml     semantic-release + goreleaser pipeline
├── .goreleaser.yml                   Cross-platform builds, Homebrew tap
└── .releaserc.json                   Conventional Commits → semver bump config
```

---

## Architecture & Data Flow

### Layer Rules

**Domain** (`internal/domain/`) — Pure Go structs + interfaces. Imports nothing from other project packages. `ModelMetadata` is `map[string]any` — mutators access it via well-known string keys.

**Application** (`internal/application/`) — Use case orchestration. Depends on domain interfaces + infrastructure/config utilities. No direct SQLite imports (uses repo interfaces).

**Infrastructure** (`internal/infrastructure/`) — SQLite repos, filesystem mutators, config utilities, self-update. Mutators import `config` package for backup/atomic-write utilities.

**TUI** (`internal/tui/`) — Bubbletea model/view/update. Depends on application use cases. Uses `huh` for forms, `bubbles` for viewport/help/spinner components.

### Initialization Flow (main.go)

1. Setup log file → `~/.config/aimux/aimux.log` (also writes to stderr)
2. Open SQLite DB at `~/.config/aimux/matrix.db` with WAL mode, 0600 perms
3. Run migrations in order (each is a standalone func, NOT an automated migration tool)
4. Seed target CLIs (idempotent)
5. Build mutator registry: `map[string]domain.ConfigMutator` keyed by mutator name
6. If `len(os.Args) > 1` → CLI mode; else → TUI mode

### Data Flow: Switch/Apply

```
User selects CLI + Provider + Model mappings in TUI
  → SwitchUseCases.BindProfile() (validates + saves to active_multiplex table)
  → SwitchUseCases.Apply()
    → ResolveTargetConfigPath (tilde expansion)
    → Parse mutator_config JSON
    → For each bound provider:
        → Lookup mutator by cli.Mutator name
        → Mutator: PrepareDir → CreateBackup → ReadJSONWithLock → Merge → AtomicWrite → PruneBackups
    → Return BackupResult
```

---

## Testing Patterns

### In-Memory SQLite

**ALL tests use `:memory:` SQLite** — never a real database. Every test setup opens a fresh in-memory DB and runs the full migration chain.

The `setupTestDB` pattern is **duplicated** across packages (`internal/application/helpers_test.go`, `internal/infrastructure/sqlite/queries_test.go`, `internal/infrastructure/config/utils_test.go`) — each package independently creates its own `setupTestDB`. Don't extract to a shared package; the duplication is deliberate.

### Test Harness Patterns

- `setupTestDB(t)` — opens `:memory:` DB, enables foreign keys, runs all migrations + seed
- `setupProviderTest(t)` — returns `*ProviderUseCases` with in-memory repos
- `setupSwitchTest(t)` — returns `*SwitchUseCases` with in-memory repos + Claude mutator
- `setupSwitchHarness(t)` — returns harness struct with both `uc` and `db` for tests needing direct DB access
- `addTestProvider(t, uc, name, baseURL)` — helper to seed a provider
- `insertTestModels(t, uc, providerID, models)` — helper to seed models

### What's Tested

| Package | Test approach |
|---------|--------------|
| `config` | Real temp dirs via `t.TempDir()`, backup isolation via `backupRootFn` override |
| `sqlite` | In-memory DB, tests for CRUD, cascade deletes, uniqueness constraints |
| `application` | In-memory repos + real mutator instances, tests for use case orchestration |
| `mutators` | Temp config files, verify JSON structure and env injection |
| `tui` | Unit tests for rendering helpers (not Bubbletea model integration) |

### Key: `-count=1`

Always use `-count=1` to bypass Go's test cache. Without it, repeated `go test ./...` may not re-run passing tests.

---

## Non-Obvious Patterns & Gotchas

### Model Metadata (the biggest gotcha)

`ModelMetadata` is `map[string]any` — **NOT** a typed struct. Mutators access it via string constants defined in `domain/provider.go` (`MetaContextWindow`, `MetaMaxTokens`, etc.). When adding new metadata keys, always define a const — don't use raw strings.

### Mutator Contract

Every mutator must follow this exact sequence:
1. `config.PrepareDir(configPath)` — ensure parent dir exists
2. `os.Stat` → `config.CreateBackup(configPath)` — backup before mutation
3. `config.ReadJSONWithLock(configPath)` — read with file lock (returns empty map if missing)
4. Merge mutations into the config map
5. `config.WriteAtomicJSON(configPath, root)` — write via temp file + rename (atomic)
6. `config.PruneBackups(configPath, 5)` — keep max 5 backups per config file

`ReadJSONWithLock` tolerates trailing commas (common in hand-edited JSON) by retrying with regex cleanup.

### Claude Code Mutator Specifics

- **Never use `ANTHROPIC_API_KEY`** — it triggers a login prompt in Claude Code. Always use `ANTHROPIC_AUTH_TOKEN`. The API key falls through to `ANTHROPIC_AUTH_TOKEN` if no auth token is set.
- Base URL path is force-normalized to `/anthropic` — Claude Code requires this path segment regardless of the provider's actual API type.
- Default extra env vars: `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_EFFORT_LEVEL=max`. Can be overridden via `mutator_config.extra_env` or disabled with `mutator_config.extra_env_disabled=true`.

### Multi-Provider Support

Not all CLIs support multi-provider. The field is per-CLI:
- **Single provider**: Claude Code (one binding at a time)
- **Multi-provider**: pi, OpenCode, Copilot (each provider gets a separate entry in the config)

The `ConfigMutator.Mutate` interface handles both cases — the mutator reads existing config and appends/replaces provider entries.

### Backup System

- Backups stored in `~/.config/aimux/backups/` (overridable via `backupRootFn` in tests)
- Named with timestamp + SHA1 prefix of the config path
- `PruneBackups` keeps max 5 per config path
- `RestoreBackup` can restore by exact backup path or config path (auto-picks latest)

### TUI State Machine

The TUI is a single Bubbletea model with ~20 `viewType` constants. State transitions happen via `SwitchToViewMsg` and `tea.Cmd` sequences. Forms reset their state every time they're entered (values aren't persisted across view switches). Key patterns:

- Layout computed once per `tea.WindowSizeMsg` via `computeLayout()` — stored in `uiLayout` struct
- Centered mode (default) vs fluid mode (diff/confirmation view only)
- Minimum terminal size: 50×15 characters
- Footer rendered via unified `renderFooter(keys...)` with `FooterKey`/`FooterDesc`/`FooterSep` styles

### Theme (Synthwave Outrun)

All components use canonical palette tokens from `theme.go` — **never hardcode hex values**. The theme instance `aimuxT` is a package-level variable with pre-built lipgloss styles. Key palette rule: **cyan (`#00F0FF`) replaces green** for success states — no green in the palette.

### SQLite Migrations

Migrations are **standalone functions called in sequence** from `main.go`, not an automated migration tool. Each migration:
- Is idempotent (uses `IF NOT EXISTS` / `ALTER TABLE ... IF NOT EXISTS`)
- Is a single function: `func(*sql.DB) error`
- Is added to the chain in `main.go`'s `for _, step := range []func(*sql.DB) error{...}`

There is NO migration version table or rollback mechanism. Schema changes are additive only.

### Database-Specific

- Database file: `~/.config/aimux/matrix.db`
- WAL mode enabled (`PRAGMA journal_mode=WAL`)
- Busy timeout: 5s (`PRAGMA busy_timeout=5000`)
- File permissions: 0600
- Foreign keys enabled at connection level (`PRAGMA foreign_keys = ON`)
- The `target_clis` table is seeded at startup (5 default CLIs) — idempotent via `INSERT OR IGNORE`
- All API keys and auth tokens stored in **plaintext** (no encryption)

### Self-Update

- Checks GitHub Releases API
- Downloads archive + `checksums.txt` — validates SHA256 before extraction
- Atomically replaces binary via temp file + rename (cross-platform via `os.Rename`)
- Falls back to `brew upgrade` if binary is installed via Homebrew
- Update check cached for 24h in SQLite

### CLI Commands

Built into main.go's `runCLI` (no Cobra/Cobra-like framework):
- `aimux apply <cli-name>` — mutate config for a CLI
- `aimux list` — show active multiplexes
- `aimux backups <cli-name>` — list backups
- `aimux restore <cli-name> [backup-path]` — restore from backup
- `aimux version` — print version
- `aimux update` — self-update

### CI/CD Pipeline

Push to `main` triggers:
1. `semantic-release` analyzes commit messages (Conventional Commits)
2. Sets version (feat→minor, fix→patch, BREAKING CHANGE→major)
3. `goreleaser` cross-compiles darwin/linux × amd64/arm64 (CGO_ENABLED=0)
4. Creates GitHub Release with tar.gz + checksums
5. Updates Homebrew tap `MileniumTick/homebrew-tap`

**Key**: `goreleaser release --clean` is invoked via `@semantic-release/exec`. The `.releaserc.json` disables goreleaser's own release creation (`release.disable: true` in goreleaser.yml) — the GitHub release is created by `@semantic-release/github`.

### Version Injection

```go
var version = "dev" // main.go
```
Override with: `-ldflags "-X main.version=x.y.z"`. The `dev` default is used for local builds.

---

## Adding a New CLI Mutator

1. Create `internal/infrastructure/mutators/new_cli.go` implementing `domain.ConfigMutator`
2. Register in `main.go`'s `mutatorRegistry` map
3. Add a seed row to `sqlite.SeedTargetCLIs` or let users add via TUI
4. Add per-mutator test file

The mutator `Mutate(ctx)` signature receives:
- `configPath` — resolved absolute path to the CLI's config file
- `modelMappings` — env var → model name map (may be empty for multi-provider)
- `provider` — full Provider struct with API key, auth token, base URL
- `mutatorConfig` — parsed JSON from `target_clis.mutator_config` column (includes `_model_metadata` injected by the application layer)

---

## File Conventions

- **No linter configured** — only `go vet` is available
- **Formatting**: `go fmt` is used but not checked in CI
- **Commits**: Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `ci:`, `revert:`)
- **Branch**: `main` is the only branch (trunk-based)
- **`.gitignore` excludes `*.sh`** — shell scripts are not tracked
- **No `Makefile`** — all commands are Go toolchain or goreleaser
- **SDD specs** live in `openspec/specs/` — may be stale; trust the code

---

## Dependencies (Key)

| Dependency | Purpose | Notes |
|-----------|---------|-------|
| `bubbletea` v1.3.10 | TUI framework | Elm-architecture (Model/Update/View) |
| `bubbles` v1.0.0 | TUI components | viewport, help, spinner, key |
| `huh` v1.0.0 | Form framework | Paginated forms, validation |
| `lipgloss` v1.1.0 | Styling | Styles, colors, layout |
| `modernc.org/sqlite` | Embedded DB | Pure Go SQLite, no CGO |
| `gofrs/flock` | File locking | Cross-platform flock |
| `BurntSushi/toml` | TOML parsing | Codex config mutator |
| `golang.org/x/mod/semver` | Semver comparison | Self-update |

---

## Config File Locations

| Path | Purpose |
|------|---------|
| `~/.config/aimux/matrix.db` | SQLite database |
| `~/.config/aimux/aimux.log` | Application log (also goes to stderr) |
| `~/.config/aimux/backups/` | Config backup directory (auto-managed) |
| `~/.config/claude/settings.json` | Claude Code config (mutated) |
| `~/.config/opencode/config.json` | OpenCode config (mutated) |
| `~/.codex/config.toml` | Codex config (mutated) |
| `~/.zshrc` (or shell profile) | Copilot config (mutated via env vars) |
| `~/.pi/agent/models.json` | pi config (mutated) |
