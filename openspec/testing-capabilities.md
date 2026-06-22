## Testing Capabilities

**Strict TDD Mode**: false (explicit in openspec/config.yaml — config is stale, project now has 116 tests)
**Detected**: 2026-06-22
**Persistence**: hybrid (openspec/ + Engram)

### Test Runner

- Command: `go test ./... -v -count=1`
- Framework: Go standard `testing` package
- Race detector: `go test ./... -race -count=1`

### Test Layers

| Layer | Available | Tool |
|-------|-----------|------|
| Unit | ✅ | Go `testing` package |
| Integration | ✅ | In-memory SQLite (`:memory:`), real temp dirs via `t.TempDir()` |
| E2E | ❌ | — (TUI integration not E2E-tested) |

### Coverage

- Available: ✅
- Command: `go test ./... -coverprofile=coverage.out`

### Quality Tools

| Tool | Available | Command |
|------|-----------|---------|
| Linter | ❌ | None configured — rely on `go vet` |
| Type checker | ✅ | `go vet ./...` |
| Formatter | ✅ | `go fmt ./...` (not enforced in CI) |

### Test Files (12)

| Package | Files | Approach |
|---------|-------|----------|
| `internal/application/` | provider_svc_test.go, multiplex_svc_test.go, helpers_test.go | In-memory repos + real mutator instances |
| `internal/infrastructure/config/` | utils_test.go | Real temp dirs, backup isolation via `backupRootFn` |
| `internal/infrastructure/sqlite/` | queries_test.go | In-memory DB, CRUD, cascade deletes |
| `internal/infrastructure/mutators/` | 5 files (one per mutator) | Temp config files, verify structure |
| `internal/infrastructure/daemon/` | resolve_test.go | Integration |
| `internal/tui/` | tui_test.go | Unit tests for rendering helpers |

### Test Harness Patterns

- `setupTestDB(t)` — in-memory DB, foreign keys, migrations + seed
- `setupProviderTest(t)` — `*ProviderUseCases` with in-memory repos
- `setupSwitchTest(t)` — `*SwitchUseCases` with in-memory repos + mutator
- `setupSwitchHarness(t)` — harness struct with `uc` + `db`

### CI/CD

- **CI**: No test step in CI pipeline (`release.yml` only runs semantic-release + goreleaser)
- **Tests pass locally**: 116 passed, `go vet` clean
- **Cache bypass**: Always use `-count=1`

### Findings

1. `openspec/config.yaml` outdated — says "greenfield project with no go.mod or test files", but project has both
2. No linter configured — only `go vet`
3. No E2E layer — TUI not integration-tested
4. No CI test step — tests run locally only
5. Strict TDD explicit `false` in config, but project has mature testing infrastructure
