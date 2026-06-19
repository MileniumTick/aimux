# Development Guide

> Build, test, release, and contribute to aimux.

---

## Prerequisites

- **Go 1.25+** (check `go.mod` for exact version)
- **No C toolchain** — aimux uses `modernc.org/sqlite` (pure Go SQLite)
- **No database server** — SQLite is embedded

## Quick Start

```bash
git clone https://github.com/MileniumTick/aimux.git
cd aimux

# Build
go build -o aimux .

# Run
./aimux

# Build with version
go build -ldflags "-X main.version=0.3.0" -o aimux .
```

## Project Structure

```
.
├── main.go                          Entrypoint, CLI routing, DB setup, migration chain, mutator registry
├── go.mod / go.sum                  Dependencies
├── ARCHITECTURE.md                   Full architecture reference
├── DESIGN.md                         Visual design decisions (TUI theme, layout, interaction)
├── README.md                         User-facing documentation
├── docs/
│   ├── manual-de-usuario.md          Spanish user manual
│   ├── SECURITY.md                   Security model and threat mitigation
│   └── DATABASE.md                   Schema reference and migration history
├── openspec/
│   ├── config.yaml                   SDD configuration
│   ├── specs/                        Current specs (domain, infrastructure, mapping, tui, path, provider, storage, switch)
│   └── changes/                      SDD change archives
├── internal/
│   ├── domain/
│   │   ├── provider.go               Provider, ProviderModel, ModelMetadata, ProviderRepository
│   │   ├── targetcli.go              TargetCLI, TargetCLIRepository, ConfigMutator, BackupResult
│   │   └── multiplex.go              ActiveMultiplex, MultiplexRepository
│   ├── application/
│   │   ├── path.go                   Path resolution (tilde, config dir, logging)
│   │   ├── provider_svc.go           ProviderUseCases (CRUD, fetch, test, retry)
│   │   ├── multiplex_svc.go          SwitchUseCases (apply, bind, dry-run, multi-provider, backups)
│   │   ├── helpers_test.go           Test utilities (in-memory DB, seed data)
│   │   ├── provider_svc_test.go      Provider use case tests
│   │   └── multiplex_svc_test.go     Switch use case tests
│   ├── infrastructure/
│   │   ├── config/
│   │   │   ├── utils.go              AtomicWrite, ReadJSONWithLock, CreateBackup, PruneBackups, RestoreBackup, flock
│   │   │   ├── utils_test.go         Config utility tests
│   │   │   └── model_catalog.go      Known model catalog (40+ models), metadata lookup, cost formatting
│   │   ├── mutators/
│   │   │   ├── claude_json.go        Claude Code settings.json mutator
│   │   │   ├── claude_json_test.go
│   │   │   ├── opencode_json.go      OpenCode config.json mutator
│   │   │   ├── opencode_json_test.go
│   │   │   ├── codex_toml.go         Codex config.toml mutator
│   │   │   ├── codex_toml_test.go
│   │   │   ├── copilot_shell.go      Copilot shell profile mutator
│   │   │   ├── copilot_shell_test.go
│   │   │   ├── pi_dual.go            pi models.json mutator
│   │   │   └── pi_dual_test.go
│   │   ├── sqlite/
│   │   │   ├── db.go                 Open(), migration chain, seed data, indexes
│   │   │   ├── provider_repo.go      ProviderRepository implementation
│   │   │   ├── targetcli_repo.go     TargetCLIRepository implementation
│   │   │   ├── multiplex_repo.go     MultiplexRepository implementation
│   │   │   └── queries_test.go       SQLite integration tests
│   │   └── update/
│   │       ├── models.go             UpdateInfo struct
│   │       ├── checker.go            CheckForUpdate (GitHub Releases API, semver)
│   │       ├── cache.go              24h cache with SQL expiry
│   │       └── updater.go            SelfUpdate (download, validate SHA256, atomic replace)
│   └── tui/
│       ├── model.go                  Core Bubbletea model (Init/Update/View)
│       ├── forms.go                  All huh form builders
│       ├── table.go                  Dashboard table renderer
│       ├── menu.go                   Menu rendering
│       ├── theme.go                  Synthwave theme, HuhTheme()
│       ├── stepper.go                Switch flow step indicator
│       ├── truncate.go               Text truncation utilities
│       ├── logo.go                   ASCII art logo
│       └── tui_test.go               TUI tests
├── .github/workflows/release.yml      CI/CD: semantic-release + goreleaser
├── .goreleaser.yml                    goreleaser config (multi-platform, Homebrew tap)
└── .releaserc.json                    semantic-release config
```

## Running Tests

```bash
# All tests
go test ./... -v -count=1

# Specific package
go test ./internal/infrastructure/config/ -v
go test ./internal/infrastructure/sqlite/ -v
go test ./internal/application/ -v
go test ./internal/infrastructure/mutators/ -v

# With race detector
go test ./... -race -count=1

# Coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

**Test configuration**:

- **All tests use in-memory SQLite** (`:memory:`) — no filesystem or real database needed
- **Config utility tests** use `t.TempDir()` for file I/O
- **Backup tests** use a custom `backupRootFn` override to isolate from `~/.config/aimux/backups`
- **Non-TDD mode**: Tests are written for coverage but not strictly test-first

## Test Architecture

| Package | Test File | What's Tested |
|---------|----------|--------------|
| `config` | `utils_test.go` | ReadJSONWithLock (existing, empty, missing, invalid), AtomicWrite (create, overwrite, no stale temp), WriteAtomicJSON, CreateBackup (existing, missing), RestoreBackup (round-trip), PruneBackups (excess, under-limit), PrepareDir |
| `sqlite` | `queries_test.go` | Provider CRUD (add, duplicate, list sorted, update status, delete cascade), InsertModels (clear+reinsert, empty list), ActiveMultiplex (set/get, not-found, multi-provider, clear), ListActive (join), SeedTargetCLIs (idempotent), ListAllModels (join), Cascade delete, TargetCLIRepository |
| `application` | `provider_svc_test.go` | ProviderUseCases: Add flow, List, Get, Update, Delete |
| `application` | `multiplex_svc_test.go` | SwitchUseCases: Apply (resolved path), BindProfile validation, DryRun, GetBoundModels, ListBindings |
| `mutators` | `*_test.go` | Per-mutator tests: JSON structure, env var injection, backup creation, multi-provider entries |

## Building for Release

```bash
# Local build with version
go build -ldflags "-s -w -X main.version=$(git describe --tags --abbrev=0 | sed 's/^v//')" -o aimux .

# Simulate goreleaser build
goreleaser build --snapshot --clean
```

## Release Pipeline

Pushing to `main` triggers:

1. **semantic-release** analyzes commit messages (Conventional Commits)
2. **goreleaser** builds cross-platform binaries (darwin/linux × amd64/arm64)
3. **GitHub Release** is created with tar.gz assets and checksums
4. **Homebrew tap** (`MileniumTick/homebrew-tap`) is updated

### Commit Convention

```
feat: multi-provider, model selection, and cross-platform flock
fix: copilot reads COPILOT_MODEL from _registered_models
docs: comprehensive README with architecture and examples
chore: bump to v0.1.0
ci: add automated release pipeline
revert: remove model selection form
```

- `feat:` → minor version bump
- `fix:` → patch version bump
- `BREAKING CHANGE:` footer → major version bump

## Adding a New CLI Mutator

1. Create `internal/infrastructure/mutators/new_cli.go`:

   ```go
   type NewCLIMutator struct{}
   
   func (m *NewCLIMutator) Mutate(
       configPath string,
       modelMappings map[string]string,
       provider domain.Provider,
       mutatorConfig map[string]any,
   ) (*domain.BackupResult, error) {
       // 1. Validate mutator_config
       // 2. PrepareDir
       // 3. CreateBackup (if file exists)
       // 4. ReadJSONWithLock (or ReadFile for non-JSON)
       // 5. Merge mutations
       // 6. WriteAtomicJSON (or AtomicWrite)
       // 7. PruneBackups
       // 8. Return BackupResult
   }
   ```

2. Register in `main.go`:

   ```go
   mutatorRegistry := map[string]domain.ConfigMutator{
       // ... existing mutators
       "new-cli-mutator": &mutators.NewCLIMutator{},
   }
   ```

3. Add seed row in `sqlite/db.go` `SeedTargetCLIs()`:

   ```go
   {
       "new-cli",
       "~/.config/new-cli/config.json",
       `["NEW_CLI_MODEL"]`,
       "new-cli-mutator",
       `{"provider_id":"custom"}`,
   },
   ```

4. Write tests in `new_cli_test.go`

5. Add specs under `openspec/specs/`

## Adding Model Metadata

Add entry to `config.KnownModelCatalog` in `internal/infrastructure/config/model_catalog.go`:

```go
"new-model-name": {
    domain.MetaContextWindow:   128_000,
    domain.MetaMaxTokens:       16_384,
    domain.MetaReasoning:       false,
    domain.MetaInputModalities: []any{"text"},
    domain.MetaCost:            map[string]any{"input": 1.0, "output": 4.0},
},
```

Prefix matching is supported — `new-model-name-20260101` will match `new-model-name`.

## Code Style

- `gofmt` for formatting
- Standard library preference over third-party
- `ponytail:` comments mark deliberate simplifications with upgrade path
- Error wrapping: `fmt.Errorf("context: %w", err)`
- No panics in library code
- OS exit only in `main.go`

## Debugging

### Logs

```bash
tail -f ~/.config/aimux/aimux.log
```

Log format: `YYYY/MM/DD HH:MM:SS file.go:line: message`

### Database Inspection

```bash
sqlite3 ~/.config/aimux/matrix.db

.tables
.schema providers
SELECT * FROM providers;
SELECT * FROM active_multiplex;
SELECT * FROM target_clis;
SELECT pm.model_name, p.name FROM provider_models pm JOIN providers p ON pm.provider_id = p.id;
```

### Backup Inspection

```bash
ls -la ~/.config/aimux/backups/
find ~/.config/aimux/backups/ -type f | sort
```
