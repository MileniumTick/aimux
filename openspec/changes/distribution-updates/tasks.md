# Tasks: Distribution and Auto-Update for aimux

## Dependency Graph

```
Phase 1: Version Embedding ──────┬──────┐
  (main.go: Version, Commit, Date)│      │
                                  │      │
Phase 2: DB Migration ───────────┐│      │
  (update_cache table)           ││      │
                                  ▼▼      │
Phase 3: update Package ─── Phase 3b ─────┤
  (models.go, cache.go,         (checker) │
   checker.go, updater.go)                │
                                  │       │
                    ┌─────────────┘       │
                    ▼                     ▼
          Phase 5: Startup Check     Phase 6: update Cmd
          (main.go integration)      (main.go wiring)
                    │
                    ▼
          Phase 4: TUI Footer
          (model.go, styles.go)

Phase 7: GoReleaser + CI  ──── independent (any time)
  (.goreleaser.yml, release.yml)

Phase 8: Tests  ──── spans Phases 2, 3, 4, 6
```

### Dependency Rules

- **Phase 3b** (checker.go) depends on Phase 3a (models.go, cache.go)
- **Phase 5** depends on Phase 1 (Version var), Phase 3 (checker.go), Phase 4 (SetUpdateInfo)
- **Phase 6** depends on Phase 1 (Version var), Phase 3 (updater.go, isHomebrewInstall)
- **Phase 7** has zero code dependencies; can be done at any point
- **Phase 8** is cross-cutting; tasks are embedded per-phase below but the full test pass requires all phases complete

---

## Phase 1: Version Embedding + `--version` Flag

**Files**: `/Users/jchavarriam/workspace/personal/aimux/main.go`
**Import map**: none (stdlib only)
**Dependencies**: none
**Tests**: manual (`go build && ./aimux --version`)

### T1.1: Add package-level vars

Add `Version`, `Commit`, `Date` variables to `main.go` with defaults `"dev"`, `"none"`, `"unknown"`.

### T1.2: Add `--version` handling at top of `main()`

Before any DB or TUI init, parse `os.Args[1]`:

```go
switch os.Args[1] {
case "--version", "version":
    fmt.Printf("aimux %s (commit %s, built %s)\n", Version, Commit, Date)
    os.Exit(0)
}
```

Print version info and exit 0. Must NOT touch config directory, database, or TUI.

### T1.3: Verify with `go build && ./aimux --version`

Output must show `dev`, `none`, `unknown`.

### Entry Point

`func main()` — add before line 17 (`// Resolve database path`).

---

## Phase 2: `update_cache` SQLite Migration

**Files**: `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/sqlite/db.go`
**Import map**: none (stdlib SQL)
**Dependencies**: none
**Tests**: visual review + existing `queries_test.go` should pass (schema unchanged aside from new table)

### T2.1: Add `update_cache` table to `RunMigrations()`

Append to `statements` slice in `RunMigrations()`:

```sql
CREATE TABLE IF NOT EXISTS update_cache (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    checked_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### Entry Point

`RunMigrations(db *sql.DB)` in `db.go` line 47 — append after existing table creation statements.

### Verification

- `go build` must succeed
- Existing `go test ./internal/infrastructure/sqlite/` must pass (no regressions from new table)

---

## Phase 3: `internal/infrastructure/update/` Package

**Files to create**:
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/models.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/cache.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/checker.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/updater.go`

**Import map**: depends on `database/sql` (cache.go), `net/http`, `encoding/json`, `golang.org/x/mod/semver` (checker.go), `archive/tar`, `compress/gzip`, `crypto/sha256` (updater.go)

**Dependencies**: Phase 2 (update_cache table must exist for cache ops)

### Sub-phase 3a: Shared Types + Cache Layer

#### T3a.1: Create `models.go`

Define `UpdateInfo` struct with `CurrentVersion`, `LatestVersion`, `HasUpdate` fields. Pure data type, no methods.

#### T3a.2: Create `cache.go`

Functions:
- `CacheGet(db *sql.DB, key string) (string, error)` — SELECT value FROM update_cache WHERE key=? AND checked_at > datetime('now', '-24 hours')
- `CacheSet(db *sql.DB, key, value string) error` — INSERT ... ON CONFLICT DO UPDATE
- Constant `CacheKeyLatestVersion = "latest_version"`

Error handling: `CacheGet` returns `("", nil)` on any error (caller treats as cache miss). `CacheSet` returns the error.

### Sub-phase 3b: Update Checker

#### T3b.1: Add `golang.org/x/mod` as explicit dependency

```bash
go get golang.org/x/mod@latest
```

(This package is already in go.sum as transitive dep; this makes it explicit.)

#### T3b.2: Create `checker.go`

Function `CheckForUpdate(currentVersion string, db *sql.DB, httpClient *http.Client) UpdateInfo`:

1. Try SQLite cache (CacheGet with CacheKeyLatestVersion)
2. Cache HIT: compare cached version with `currentVersion` using `semver.Compare()`
3. Cache MISS: HTTP GET `https://api.github.com/repos/jchavarriam/aimux/releases/latest`
   - Headers: User-Agent `aimux/<Version>`, Accept `application/vnd.github+json`
   - Timeout: 5s context deadline
   - Parse JSON, extract `tag_name`, strip `v` prefix
   - Store via CacheSet
   - Compare versions
4. On ANY error: return `UpdateInfo{HasUpdate: false}` (silent failure)
5. NEVER log or print to stderr

### Sub-phase 3c: Self-Updater

#### T3c.1: Create `updater.go`

Functions:
- `SelfUpdate(currentVersion, execPath string) error` — full update flow
- `getAssetURL(goos, goarch, latestTag string) (downloadURL, checksumURL string, err error)` — asset resolution
- `checkWritePermission(execPath string) error` — write test file in binary dir
- `validateChecksum(binaryPath, expectedSHA256 string) error` — SHA256 validation
- `atomicReplace(verifiedPath, targetPath string) error` — temp + rename
- `isHomebrewInstall(execPath string) bool` — detect Homebrew install
- `homebrewUpdate() int` — delegate to `brew upgrade`

SelfUpdate flow:
1. Check write permission in binary directory
2. Fetch latest release tag from GitHub API
3. Compare versions via semver — if already latest, print message and return nil
4. Build archive name per platform (`aimux_{version}_{GOOS}_{GOARCH}.tar.gz`)
5. Download archive to temp file in same directory as binary
6. Extract binary from archive
7. Download checksums.txt from same release
8. Compute SHA256 of extracted binary
9. Validate against checksums.txt
10. Atomic replace: os.Rename() temp over current binary
11. Print success message, return nil

Error handling per the design matrix — all temp files cleaned up on failure.

### Entry Point

New package at `internal/infrastructure/update/`. No init() functions needed. Package name: `update` (already aliased as `update` in main.go usage).

---

## Phase 4: TUI Footer Rendering

**Files to modify/create**:
- `/Users/jchavarriam/workspace/personal/aimux/internal/tui/model.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/tui/styles.go` (CREATE)

**Import map**: `github.com/charmbracelet/lipgloss` (already in go.mod)
**Dependencies**: Phase 3 (UpdateInfo DTO) — but TUI defines its OWN `UpdateInfo` DTO to avoid import cycle

### T4.1: Add `UpdateInfo` DTO and field to model

In `model.go`, add:

```go
type UpdateInfo struct {
    CurrentVersion string
    LatestVersion  string
    HasUpdate      bool
}
```

Add `updateInfo UpdateInfo` field to the `model` struct.

### T4.2: Add `SetUpdateInfo()` method

```go
func (m *model) SetUpdateInfo(info UpdateInfo) {
    m.updateInfo = info
}
```

### T4.3: Create `styles.go` with `dimItalicStyle`

Package-level variable:

```go
var dimItalicStyle = lipgloss.NewStyle().
    Italic(true).
    Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"})
```

### T4.4: Add `renderFooter()` method

```go
func (m *model) renderFooter() string {
    version := m.updateInfo.CurrentVersion
    if version == "" {
        version = "?"
    }
    if m.updateInfo.HasUpdate {
        return dimItalicStyle.Render(
            fmt.Sprintf("aimux v%s · v%s available — run `aimux update`", version, m.updateInfo.LatestVersion),
        )
    }
    return dimItalicStyle.Render(fmt.Sprintf("aimux v%s", version))
}
```

### T4.5: Integrate in `View()`

Append `"\n" + m.renderFooter()` to every non-form view's return value. Forms return early (before footer) — no change needed for form views.

Affected views: `dashboardView`, `providerListView`, `switchConfirmationView`, `errorView`, default (Loading...).

### Unchanged Views

Form-driven views (`addProviderView`, `deleteProviderView`, `switchTargetCLIView`, `switchProviderView`, `switchMapModelsView`, `manageCLIView`, `editCLIPathView`) all return `m.form.View()` before reaching the footer append — these are unchanged.

---

## Phase 5: Startup Update Check Integration in `main.go`

**Files**: `/Users/jchavarriam/workspace/personal/aimux/main.go`
**Import map**: adds `net/http`, `time`, `update "github.com/jchavarriam/aimux/internal/infrastructure/update"`, `tui "github.com/jchavarriam/aimux/internal/tui"`
**Dependencies**: Phase 1 (Version var), Phase 3 (update package), Phase 4 (SetUpdateInfo)

### T5.1: Wire background goroutine after DB init

After `seedTargetCLIs` completes and before `model := tui.NewModel(...)`, add:

```go
// Start background update check (overlaps with model init)
updateInfoCh := make(chan update.UpdateInfo, 1)
go func() {
    httpClient := &http.Client{Timeout: 5 * time.Second}
    info := update.CheckForUpdate(Version, db, httpClient)
    updateInfoCh <- info
}()

model := tui.NewModel(providerUseCases, switchUseCases)

// Consume update check result (non-blocking)
select {
case info := <-updateInfoCh:
    model.SetUpdateInfo(tui.UpdateInfo{
        CurrentVersion: info.CurrentVersion,
        LatestVersion:  info.LatestVersion,
        HasUpdate:      info.HasUpdate,
    })
default:
    // Not done yet — model starts with zero UpdateInfo, shows "?"
}
```

### Placement

Insert between line 94 (`model := tui.NewModel(...)`) and line 99 (`program := tea.NewProgram(model, ...)`). Specifically: launch goroutine before model creation, consume channel between model creation and program creation.

### Verification

- `go build` must succeed
- On first run with no cache: one HTTP request to GitHub API
- On second run (within 24h): no HTTP request (cache hit)
- On network error: footer shows `aimux v?` (no crash, no error message)

---

## Phase 6: `aimux update` Subcommand

**Files**: `/Users/jchavarriam/workspace/personal/aimux/main.go`
**Import map**: adds `os/exec`, `runtime` (already imported via `net/http` etc.), `update` package
**Dependencies**: Phase 1 (Version var), Phase 3 (SelfUpdate, isHomebrewInstall, homebrewUpdate)

### T6.1: Add `runUpdate()` function in `main.go`

```go
func runUpdate() int {
    execPath, err := os.Executable()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: cannot determine binary path: %v\n", err)
        return 1
    }
    if update.IsHomebrewInstall(execPath) {
        return update.HomebrewUpdate()
    }
    if err := update.SelfUpdate(Version, execPath); err != nil {
        fmt.Fprintf(os.Stderr, "Error: update failed: %v\n", err)
        return 1
    }
    return 0
}
```

(Note: the `update.IsHomebrewInstall` and `update.HomebrewUpdate` functions are exported from the update package. If not needed externally, they can remain unexported and `runUpdate` calls internal wrappers.)

### T6.2: Add `"update"` case in `os.Args` switch

Add to existing switch in `main()` (from T1.2):

```go
case "update":
    os.Exit(runUpdate())
```

This case runs BEFORE the `--version` case in the switch — both are mutually exclusive, but `update` must not be swallowed by the TUI startup.

### Placement

After the `--version` case in the `os.Args[1]` switch, before TUI initialization.

### Verification

- `go build && ./aimux update` on dev build: prints "failed to check latest version" or similar, exits 1 (no GitHub release exists yet)
- `go build && ./aimux update` with network off: prints network error, exits 1
- `go build && ./aimux` (no args): TUI starts normally

---

## Phase 7: GoReleaser + GitHub Actions

**Files to create**:
- `/Users/jchavarriam/workspace/personal/aimux/.goreleaser.yml`
- `/Users/jchavarriam/workspace/personal/aimux/.github/workflows/release.yml`

**Import map**: none (CI config)
**Dependencies**: none (can be done any time; ideally after Phase 1 so ldflags are confirmed working)

### T7.1: Create `.goreleaser.yml`

Per design spec: version 2, project_name aimux, CGO_ENABLED=0, 4 platform targets (darwin amd64/arm64, linux amd64/arm64), ldflags for Version/Commit/Date, tar.gz archives, checksums.txt, Homebrew tap to `jchavarriam/homebrew-tap`.

### T7.2: Create `.github/workflows/release.yml`

Per design spec: trigger on `v*` tag push, checkout with fetch-depth:0, setup-go with go 1.23 (match go.mod), goreleaser-action v6 with `args: release --clean`, GITHUB_TOKEN.

### T7.3: Verify with `goreleaser release --snapshot` (optional)

If GoReleaser is available locally, run to validate the config produces correct archives.

### Precondition for Release

The tap repository `github.com/jchavarriam/homebrew-tap` must exist before the first release. It needs a `Formula/` directory.

---

## Phase 8: Tests

**Files to create**:
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/cache_test.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/checker_test.go`
- `/Users/jchavarriam/workspace/personal/aimux/internal/infrastructure/update/updater_test.go`

**Import map**: `testing`, `database/sql`, in-memory SQLite, `net/http/httptest`
**Dependencies**: Phase 2 (update_cache table for cache tests), Phase 3 (all updater functions)

### T8.1: Cache unit tests (`cache_test.go`)

- `TestCacheGet_hit` — INSERT a value, retrieve it
- `TestCacheGet_miss_expired` — INSERT with old checked_at, verify miss
- `TestCacheGet_miss_missing` — Query non-existent key
- `TestCacheSet_idempotent` — Double-set same key, verify upsert

Use an in-memory SQLite DB (`:memory:`) with the update_cache table created.

### T8.2: Checker integration tests (`checker_test.go`)

- `TestCheckForUpdate_cacheHit_noUpdate` — Seed cache with same version, verify HasUpdate=false
- `TestCheckForUpdate_cacheHit_updateAvailable` — Seed cache with newer version, verify HasUpdate=true
- `TestCheckForUpdate_cacheMiss_httpFallback` — Use httptest.Server to mock GitHub API
- `TestCompareVersions` — Direct test of semver comparison logic (extracted helper)

### T8.3: Updater unit tests (`updater_test.go`)

- `TestIsHomebrewInstall_brewPrefixMatch` — Path starts with brew prefix
- `TestIsHomebrewInstall_brewPrefixNoMatch` — Path outside brew prefix
- `TestIsHomebrewInstall_brewNotAvailable` — brew not on PATH
- `TestValidateChecksum_match` — Correct SHA256 passes
- `TestValidateChecksum_mismatch` — Wrong SHA256 fails
- `TestAtomicReplace_sameFilesystem` — Temp + rename in temp dir
- `TestCheckWritePermission_writable` — Writable dir passes
- `TestCheckWritePermission_notWritable` — Read-only dir fails

### T8.4: Full regression pass

```bash
go test ./...  # all packages must pass
```

### What Is NOT Tested

- Live GitHub API calls (tested manually or via CI on release)
- GoReleaser pipeline (tested by `goreleaser release --snapshot` locally)
- TUI footer rendering (manual verification; Bubble Tea view testing is low-value)
- Homebrew integration (tested by `brew upgrade` command itself)

---

## Task Summary Table

| # | Phase | Files (Create/Modify) | Est. LOC | Dependencies |
|---|-------|----------------------|----------|--------------|
| T1.1 | Version vars | main.go (M) | 5 | none |
| T1.2 | --version flag | main.go (M) | 10 | T1.1 |
| T1.3 | Verify --version | (manual) | — | T1.2 |
| T2.1 | update_cache migration | db.go (M) | 10 | none |
| T3a.1 | models.go | update/models.go (C) | 10 | none |
| T3a.2 | cache.go | update/cache.go (C) | 40 | T2.1 |
| T3b.1 | Add golang.org/x/mod dep | go.mod (M) | 1 | none |
| T3b.2 | checker.go | update/checker.go (C) | 80 | T3a.1, T3a.2 |
| T3c.1 | updater.go | update/updater.go (C) | 150 | T3b.1 |
| T4.1 | UpdateInfo + field | model.go (M) | 10 | none |
| T4.2 | SetUpdateInfo | model.go (M) | 5 | T4.1 |
| T4.3 | dimItalicStyle | tui/styles.go (C) | 10 | none |
| T4.4 | renderFooter | model.go (M) | 20 | T4.3 |
| T4.5 | View integration | model.go (M) | 5 | T4.4 |
| T5.1 | Background goroutine | main.go (M) | 25 | T1.1, T3b.2, T4.2 |
| T6.1 | runUpdate() | main.go (M) | 30 | T1.1, T3c.1 |
| T6.2 | "update" case in switch | main.go (M) | 3 | T6.1 |
| T7.1 | .goreleaser.yml | (C) | 50 | none |
| T7.2 | release.yml | (C) | 35 | none |
| T7.3 | Snapshot verify | (manual) | — | T7.1 |
| T8.1 | Cache tests | cache_test.go (C) | 60 | T2.1, T3a.2 |
| T8.2 | Checker tests | checker_test.go (C) | 80 | T3b.2 |
| T8.3 | Updater tests | updater_test.go (C) | 100 | T3c.1 |
| T8.4 | Full regression | (manual) | — | T8.1-3 |
| **Total** | | **~19 files (7 new, 6 modified)** | **~740** | |

---

## Review Workload Forecast

### Changed Line Budget

| Metric | Value |
|--------|-------|
| Estimated net LOC | ~740 |
| Files created | 7 |
| Files modified | 6 |
| New package trees | 1 (`internal/infrastructure/update/`) |
| CI config files | 2 (`.goreleaser.yml`, `.github/workflows/release.yml`) |
| Test files | 3 |

### 400-Line Budget Risk

**High**. Estimated net change is ~740 LOC, well above the 400-line threshold. However, the code is:

1. **Loosely coupled** — 6 independent files (7 test files are separate), no circular dependencies
2. **Mechanical** — the update package is pure infrastructure with no domain model changes
3. **Self-contained** — the three largest components (checker, updater, tests) are in their own package with no cross-package entanglement

### Chained PRs Recommendation

**Not recommended**. Despite exceeding 400 lines, the change is:

- A single feature with tight internal coupling within the `update` package
- Splitting would mean PRs that individually cannot be tested or verified
- Phase 1 (version embedding) and Phase 7 (GoReleaser/CI) are the only clean split points, but creating a PR with just `--version` and `.goreleaser.yml` provides no user value independently

### Decision: `exception-ok` (pre-configured)

The `delivery_strategy` is `exception-ok`. Proceed with a single PR containing all phases. The code is additive (new package, new CI files, small modifications to existing files) with zero risk of breaking existing functionality.

### Recommended Review Order

1. `main.go` diff — version vars, flag handling, update wiring, background goroutine
2. `update/*.go` — models, cache, checker, updater
3. `db.go` diff — migration statement
4. `tui/model.go` + `tui/styles.go` — footer rendering
5. `.goreleaser.yml` + `release.yml` — CI config
6. `*_test.go` — test coverage and correctness
