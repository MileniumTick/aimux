# Design: Distribution and Auto-Update for aimux

## 1. Package Structure

### New and Modified Files

```
aimux/
  main.go                          [MODIFY] Add Version vars, flag parsing, update cmd
  .goreleaser.yml                  [CREATE] GoReleaser v2 config
  .github/workflows/release.yml    [CREATE] GitHub Actions release pipeline

  internal/infrastructure/update/  [CREATE] New package
    models.go                      -- UpdateInfo struct
    cache.go                       -- CacheGet / CacheSet (SQLite key-value, 24h TTL)
    checker.go                     -- CheckForUpdate (GitHub API + cache orchestration)
    updater.go                     -- SelfUpdate (download, checksum, atomic replace)
    updater_test.go                -- Tests for SelfUpdate logic

  internal/tui/
    model.go                       [MODIFY] Add updateInfo field, SetUpdateInfo, footer
    styles.go                      [MODIFY] Add dimItalicStyle for footer

  internal/infrastructure/sqlite/
    db.go                          [MODIFY] Add update_cache table to RunMigrations
```

### No Changes To

```
internal/application/       -- No changes needed (path resolution, use cases)
internal/domain/            -- No changes needed (domain types)
internal/tui/forms.go        -- No changes needed
internal/tui/table.go        -- No changes needed
internal/tui/menu.go         -- No changes needed
```

### Dependency Graph

```
main.go
  ├── internal/infrastructure/sqlite/  (existing — DB init)
  ├── internal/application/            (existing — use cases)
  ├── internal/tui/                    (existing + footer)
  └── internal/infrastructure/update/  [NEW]
        ├── checker.go  → calls GitHub API, uses cache.go
        ├── cache.go    → reads/writes SQLite (update_cache table)
        ├── updater.go  → download, checksum, atomic replace
        └── models.go   → UpdateInfo struct (shared type)
```

---

## 2. `--version` Flag: Parsing in main.go

### Design

Parsing happens BEFORE any database or TUI initialization. This ensures zero side effects for `--version`.

```
main.go entry point:
  1. Check os.Args for "--version" / "version" → print, exit 0
  2. Check os.Args for "update" → run Update handler → exit
  3. Normal startup: resolve config path, open DB, init TUI
```

### Package-Level Variables

```go
var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
)
```

These default to development values when ldflags are NOT set (local `go build .`).

### Flag Handling Logic (in main())

```go
func main() {
    // Phase 1: Simple flags — NO DB or TUI init
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "--version", "-v", "version":
            fmt.Printf("aimux %s (commit %s, built %s)\n", Version, Commit, Date)
            os.Exit(0)
        case "update":
            os.Exit(runUpdate())
        }
    }

    // Phase 2: DB init (existing code)
    dbPath, err := application.ResolveConfigPath()
    // ... rest of existing flow including DB, TUI, program.Run() ...
}
```

The `runUpdate()` function calls the update package but does NOT start the TUI. It handles:
- Homebrew detection
- Homebrew upgrade delegation
- Standalone binary self-update

### Design Decision: Why not `flag` package

Using `os.Args` directly instead of `flag.FlagSet`:
- Simpler for two positional-style commands (`--version`, `update`)
- Avoids interference between `flag` and Bubble Tea (tea doesn't use `flag`)
- The `update` subcommand is positional (no leading `--`), which `flag` doesn't handle well
- If future subcommands need a proper CLI framework, a dedicated parser can be introduced later without breaking `--version` and `update`

Trade-off: no `-h` / `--help` support beyond what `tea` provides at runtime. Acceptable for a TUI-focused tool.

---

## 3. `internal/infrastructure/update/` Package

### 3.1 `models.go` — Shared Types

```go
package update

// UpdateInfo carries the result of a version check.
type UpdateInfo struct {
    CurrentVersion string // e.g. "1.2.0" (no v prefix)
    LatestVersion  string // e.g. "1.3.0" (no v prefix)
    HasUpdate      bool
}
```

This struct is produced by `CheckForUpdate()` and consumed by the TUI model. It is separate from the TUI's own `UpdateInfo` DTO — the infrastructure package returns this type, and `model.SetUpdateInfo()` stores it (or a TUI-local copy).

### 3.2 `cache.go` — SQLite Cache

```go
const CacheKeyLatestVersion = "latest_version"

// CacheGet retrieves a cached value if stored within the last 24 hours.
// Returns ("", nil) on cache miss (expired or missing).
func CacheGet(db *sql.DB, key string) (string, error)

// CacheSet stores a key-value pair with current timestamp.
// Uses INSERT ... ON CONFLICT REPLACE for idempotency.
func CacheSet(db *sql.DB, key, value string) error
```

### SQL Query Design

CacheGet query:
```sql
SELECT value FROM update_cache
WHERE key = ?
  AND checked_at > datetime('now', '-24 hours')
```

CacheSet query:
```sql
INSERT INTO update_cache (key, value, checked_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  checked_at = excluded.checked_at
```

### 24-Hour TTL Rationale

- GitHub API unauthenticated rate limit: 60 requests/hour
- 24-hour TTL = max 1 request per 24 hours per user = well within rate limit
- Cache miss triggers exactly 1 HTTP request, not a burst
- Cache expires at query time, not write time — a single SQL WHERE clause enforces TTL

### 3.3 `checker.go` — Update Check Orchestration

```go
// CheckForUpdate performs a cached version check.
// Never blocks startup: errors are swallowed, returns zero-value UpdateInfo
// with HasUpdate=false on any failure.
func CheckForUpdate(currentVersion string, db *sql.DB, httpClient *http.Client) UpdateInfo
```

#### Flow

1. Try SQLite cache (CacheGet)
2. Cache HIT: compare cached version with `currentVersion` using `golang.org/x/mod/semver`
3. Cache MISS: HTTP GET `https://api.github.com/repos/jchavarriam/aimux/releases/latest`
   - Parse JSON response, extract `tag_name` (e.g. `"v1.3.0"`), strip `v` prefix
   - Store in cache via CacheSet
   - Compare with `currentVersion`
4. On any error (network, parse, DB): return zero-value `UpdateInfo{HasUpdate: false}`
5. NEVER log, NEVER print to stderr — silent failures

#### HTTP Request Details

```go
req, _ := http.NewRequestWithContext(ctx, "GET",
    "https://api.github.com/repos/jchavarriam/aimux/releases/latest", nil)
req.Header.Set("User-Agent", "aimux/"+Version)
req.Header.Set("Accept", "application/vnd.github+json")
// No auth header — unauthenticated access
```

#### Error Handling Matrix

| Failure Mode | Behavior |
|---|---|
| Network timeout (>5s) | Silently fail, preserve existing cache |
| HTTP 403 (rate limit) | Silently fail, preserve existing cache |
| HTTP 304 (not modified) | Treat as miss (no ETag caching in v1) |
| HTTP 200 + invalid JSON | Silently fail, treat as miss |
| DB error on CacheGet | Silently fail, proceed to HTTP check |
| DB error on CacheSet | Silently fail, check result still returned |
| semver parse error | Treat as no update (HasUpdate=false) |

### 3.4 `updater.go` — Self-Update Logic

```go
// SelfUpdate downloads the latest release for the current platform,
// validates checksum, replaces the binary atomically.
func SelfUpdate(currentVersion, execPath string) (latestVersion string, err error)
```

#### Flow

```
SelfUpdate(currentVersion, execPath):
  1. Check write permission in binary directory
  2. Fetch latest release tag from GitHub API
  3. Compare versions — if already latest, return nil
  4. Build archive name: aimux_{version}_{GOOS}_{GOARCH}.tar.gz
  5. Find matching asset in release
  6. Download archive to temp file (same directory as binary)
  7. Extract binary from archive
  8. Download checksums.txt from same release
  9. Compute SHA256 of extracted binary
  10. Validate against checksums.txt entry
  11. Write verified binary to temp path in same directory
  12. os.Rename() temp file over current binary
  13. Print success message, return new version
```

#### Archive Name Format (mirrors GoReleaser)

```
aimux_1.3.0_darwin_amd64.tar.gz
aimux_1.3.0_darwin_arm64.tar.gz
aimux_1.3.0_linux_amd64.tar.gz
aimux_1.3.0_linux_arm64.tar.gz
```

#### Atomic Replace Pattern

```go
func atomicReplace(verifiedPath, targetPath string) error {
    tmpPath := targetPath + ".tmp." + randomString(8)

    // Move verified binary to temp path in same directory
    if err := os.Rename(verifiedPath, tmpPath); err != nil {
        return fmt.Errorf("move to temp: %w", err)
    }

    // Set executable permissions
    if err := os.Chmod(tmpPath, 0755); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("chmod: %w", err)
    }

    // Atomic rename over target (POSIX: atomic if same filesystem)
    if err := os.Rename(tmpPath, targetPath); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("rename over target: %w", err)
    }

    return nil
}
```

Key invariants:
- Temp file always in the same directory as the target (ensures same filesystem mount)
- `os.Rename()` is atomic on POSIX for same-filesystem moves
- On failure at any step, temp file is cleaned up
- The running binary's inode remains mapped until the process exits — replacing it while running is safe on POSIX (the old inode stays open)

#### Checksum Validation

```go
func validateChecksum(binaryPath, expectedSHA256 string) error {
    f, err := os.Open(binaryPath)
    // ... sha256 hash, compare hex strings ...
}
```

#### Write Permission Check

```go
func checkWritePermission(execPath string) error {
    dir := filepath.Dir(execPath)
    testFile := filepath.Join(dir, ".aimux_write_test_" + randomString(6))
    f, err := os.Create(testFile)
    if err != nil {
        return fmt.Errorf("no write permission in %s: %w\nTry: sudo aimux update", dir, err)
    }
    f.Close()
    os.Remove(testFile)
    return nil
}
```

#### Error Handling Matrix

| Failure Mode | Behavior |
|---|---|
| No write permission in binary dir | Informative error: suggest `sudo aimux update` |
| GitHub API unreachable | Network error printed to stderr, exit 1 |
| Archive name not found in assets | Error listing available assets, exit 1 |
| Downloaded archive corrupt | Remove temp file, exit 1 |
| Checksum mismatch | Remove temp file, exit 1 (binary NOT modified) |
| os.Rename fails (device busy, etc.) | Remove temp file, exit 1 |
| Already up to date | Print "already up to date", exit 0 (no download) |
| brew not found | Fall through to standalone update logic |

#### Homebrew Detection

```go
func isHomebrewInstall(execPath string) bool {
    cmd := exec.Command("brew", "--prefix")
    output, err := cmd.Output()
    if err != nil {
        return false // brew not available
    }
    brewPrefix := strings.TrimSpace(string(output))
    return strings.HasPrefix(execPath, brewPrefix)
}
```

- `brew --prefix` returns e.g. `/opt/homebrew` on Apple Silicon
- Commands installed via tap go to `/opt/homebrew/bin/`
- If the binary path starts with the brew prefix, it's a Homebrew install

#### Homebrew Update

```go
func homebrewUpdate() int {
    cmd := exec.Command("brew", "upgrade", "jchavarriam/tap/aimux")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: brew upgrade failed: %v\n", err)
        return 1
    }
    fmt.Println("aimux updated successfully via Homebrew.")
    return 0
}
```

No atomic replace needed — `brew upgrade` handles this internally.

### 3.5 `runUpdate()` Wiring in main.go

```go
func runUpdate() int {
    execPath, err := os.Executable()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: cannot determine binary path: %v\n", err)
        return 1
    }

    if isHomebrewInstall(execPath) {
        return homebrewUpdate()
    }

    latestVersion, err := SelfUpdate(Version, execPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: update failed: %v\n", err)
        return 1
    }
    if latestVersion == "" {
        // Already up to date (SelfUpdate printed the message)
        return 0
    }
    fmt.Printf("aimux updated from v%s to v%s. Please restart aimux.\n", Version, latestVersion)
    return 0
}
```

The function does NOT initialize the database. The update command downloads and replaces the binary without touching SQLite. This keeps the update path fast and independent of DB health.

---

## 4. TUI Footer: Update Notification in All Views

### 4.1 Model Changes (`internal/tui/model.go`)

Add to the `model` struct:

```go
type model struct {
    // ... existing fields ...

    // Update information (set asynchronously after startup)
    updateInfo UpdateInfo
}
```

Add the TUI-local `UpdateInfo` type (in the tui package to avoid import cycles):

```go
type UpdateInfo struct {
    CurrentVersion string
    LatestVersion  string
    HasUpdate      bool
}
```

Add accessor:

```go
func (m *model) SetUpdateInfo(info UpdateInfo) {
    m.updateInfo = info
}
```

### 4.2 Footer Rendering

Add a `renderFooter()` method:

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

### 4.3 Footer Style

In `styles.go` (new file in `internal/tui/`):

```go
var dimItalicStyle = lipgloss.NewStyle().
    Italic(true).
    Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"})
```

This matches the existing color palette in `menu.go` (`menuDimmedStyle` uses `#243`, which is close). AdaptiveColor provides good contrast on both light and dark terminals.

### 4.4 View Integration

Modify `model.View()` to append footer to all non-form views:

```go
func (m *model) View() string {
    if m.form != nil {
        return m.form.View()
    }

    var content string
    switch m.currentView {
    case dashboardView:
        table := RenderTable(m.providers, m.activeMultiplexes, m.width)
        menu := RenderMenu(m.menuSelected, len(m.providers) > 0)
        content = table + "\n" + menu
    case providerListView:
        content = RenderProviderList(m.providers, m.selectedProviderID, m.width)
    case switchConfirmationView:
        // ... existing ...
    case errorView:
        // ... existing ...
    default:
        content = "Loading..."
    }

    // Append footer for all views EXCEPT active forms
    // (forms return early above)
    return content + "\n" + m.renderFooter()
}
```

Forms return early (`if m.form != nil { return m.form.View() }`), so the footer is automatically excluded from form views.

### 4.5 Footer Visibility Matrix

| View | Footer Visible? |
|---|---|
| dashboardView | Yes |
| providerListView | Yes |
| switchConfirmationView | Yes |
| errorView | Yes |
| manageCLIView | Yes (not a form) |
| editCLIPathView | Yes (not a form) |
| addProviderView | No (form renders independently) |
| deleteProviderView | No (form renders independently) |
| switchTargetCLIView | No (active form) |
| switchProviderView | No (active form) |
| switchMapModelsView | No (active form) |

### 4.6 Async Update Check Integration (in main.go)

After DB init but before `tea.NewProgram()`:

```go
// Launch background update check (non-blocking)
updateInfo := make(chan update.UpdateInfo, 1)
go func() {
    httpClient := &http.Client{Timeout: 5 * time.Second}
    info := update.CheckForUpdate(Version, db, httpClient)
    updateInfo <- info
}()

// Create model
model := tui.NewModel(providerUseCases, switchUseCases)

// Try to read update check result (non-blocking)
select {
case info := <-updateInfo:
    model.SetUpdateInfo(tui.UpdateInfo{
        CurrentVersion: info.CurrentVersion,
        LatestVersion:  info.LatestVersion,
        HasUpdate:      info.HasUpdate,
    })
default:
    // Not done yet — model shows "?" until SetUpdateInfo is called
    // The goroutine completes in background, result is silently dropped
}

program := tea.NewProgram(model, tea.WithAltScreen())
```

This design:
- Starts the HTTP request BEFORE model creation to overlap network I/O
- Uses a buffered channel so the goroutine never blocks
- Non-blocking select: if the check isn't done yet, the model starts with no update info
- If the check completes after the select, the data is silently dropped (next startup will use the cache)

---

## 5. SQLite Migration: update_cache Table

### 5.1 Table Definition

```sql
CREATE TABLE IF NOT EXISTS update_cache (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    checked_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

### 5.2 Integration in `db.go`

Add the migration statement to the `RunMigrations()` function's `statements` slice, after the `active_multiplex` table creation.

```go
func RunMigrations(db *sql.DB) error {
    statements := []string{
        // ... existing CREATE TABLE statements ...
        `CREATE TABLE IF NOT EXISTS update_cache (
            key        TEXT PRIMARY KEY,
            value      TEXT NOT NULL,
            checked_at TEXT NOT NULL DEFAULT (datetime('now'))
        )`,
    }
    // ... existing loop ...
}
```

### 5.3 Design Rationale

- **Generic key-value**: not tightly coupled to update checking. Future uses could cache release asset URLs, rate-limit backoff state, or any ephemeral config.
- **checked_at at query time**: the TTL is enforced by the WHERE clause, not by a background cleanup job. No stale data or cleanup needed.
- **PRIMARY KEY = key**: upsert semantics via `INSERT ... ON CONFLICT DO UPDATE`. No auto-increment needed.
- **TEXT timestamps**: SQLite has no native datetime type; `datetime('now')` stores ISO-8601 strings. Comparison works natively via `datetime()` function.

### 5.4 Example Data

| key | value | checked_at |
|---|---|---|
| `latest_version` | `1.3.0` | `2026-06-18 14:30:00` |

---

## 6. `cmd/update` Subcommand (via `aimux update`)

### 6.1 Entry Point

As described in Section 2, `os.Args[1] == "update"` is caught at the very top of `main()` before any initialization. This makes the update command:
- Fast (no DB init)
- Resilient (works even with a broken DB)
- Clean (no TUI to manage)

### 6.2 Install Method Detection

The detection is two-step:
1. Check if `brew` is available on PATH
2. If yes, check if the current executable path starts with `brew --prefix`

### 6.3 Install Method Routing

```
aimux update
  ├── Homebrew install → exec.Command("brew", "upgrade", "jchavarriam/tap/aimux")
  └── Standalone binary
        ├── write permission check
        ├── GitHub API: fetch latest version
        ├── Already latest? Print and exit
        └── Download, checksum, atomic replace
```

### 6.4 No TUI Menu Item for Update

The proposal mentions "TUI menu item" for update, but the design keeps it CLI-only:
- Self-replace while TUI is running is dangerous (Bubble Tea state, terminal rendering)
- The update notification footer tells users to run `aimux update` — they exit the TUI first
- Cleaner UX: exit TUI, run update in shell, restart

---

## 7. GoReleaser Configuration

### 7.1 `.goreleaser.yml`

```yaml
version: 2
project_name: aimux

before:
  hooks:
    - go mod tidy

builds:
  - env:
      - CGO_ENABLED=0
    goos:
      - darwin
      - linux
    goarch:
      - amd64
      - arm64
    mod_timestamp: "{{ .CommitTimestamp }}"
    flags:
      - -trimpath
    ldflags:
      - -s -w
      - -X main.Version={{.Version}}
      - -X main.Commit={{.FullCommit}}
      - -X main.Date={{.Date}}

archives:
  - format: tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    files:
      - LICENSE
      - README.md

checksum:
  name_template: "checksums.txt"

brews:
  - name: aimux
    repository:
      owner: jchavarriam
      name: homebrew-tap
      branch: main
    homepage: "https://github.com/jchavarriam/aimux"
    description: "AI Multiplexer — switch AI models across CLIs from a single TUI"
    license: "MIT"
    test: |
      system "#{bin}/aimux --version"
```

### 7.2 GitHub Actions: `.github/workflows/release.yml`

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: "1.25"

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

### 7.3 Release Workflow Details

- **Trigger**: push of any tag matching `v*` (e.g., `v0.2.0`, `v1.0.0`)
- **Runner**: `ubuntu-latest` — cross-compiles all platforms from a single Linux CI runner
- **fetch-depth: 0**: required by GoReleaser for commit timestamp in builds
- **GITHUB_TOKEN**: auto-provided by GitHub Actions; sufficient for publishing GitHub Releases and (potentially) pushing to `jchavarriam/homebrew-tap` if the cross-repo token scope is configured
- **Archive contents**: binary + LICENSE + README.md (no man pages, no shell completions in v1)

### 7.4 GoReleaser Version Policy

Pin `"~> v2"` (major version constraint) to avoid unexpected breaking changes. Minor and patch updates are automatically picked up.

### 7.5 LDflags Mapping Table

| Variable | GoReleaser Template | Example Value |
|---|---|---|
| `main.Version` | `{{.Version}}` | `1.2.0` |
| `main.Commit` | `{{.FullCommit}}` | `a1b2c3d4e5f6...` |
| `main.Date` | `{{.Date}}` | `2026-06-18T12:00:00Z` |

---

## 8. Data Flow Diagrams

### 8.1 Startup Version Check

```
main() starts
  │
  ├── os.Args check: --version or update? → handle and exit
  │
  ├── DB init: open, run migrations, seed
  │
  ├── [GOROUTINE] update.CheckForUpdate(Version, db)
  │     │
  │     ├── cache.go: CacheGet("latest_version", db)
  │     │     │
  │     │     ├── cache HIT (≤24h old)
  │     │     │     └── semver.Compare(cached, Version)
  │     │     │           ├── newer? → UpdateInfo{HasUpdate: true}
  │     │     │           └── same? → UpdateInfo{HasUpdate: false}
  │     │     │
  │     │     └── cache MISS (expired or missing)
  │     │           │
  │     │           ├── HTTP GET /repos/jchavarriam/aimux/releases/latest
  │     │           │     │
  │     │           │     ├── success: parse tag_name, CacheSet(...), compare
  │     │           │     │     └── UpdateInfo{...}
  │     │           │     │
  │     │           │     └── error: UpdateInfo{HasUpdate: false}
  │     │           │
  │     │           └── → channel updateInfo
  │     │
  ├── model := tui.NewModel(...)
  │
  ├── select { case info := <-updateInfo: model.SetUpdateInfo(...) }
  │
  └── tea.NewProgram(model).Run()
```

### 8.2 `aimux update` Flow

```
User runs: aimux update
  │
  ├── os.Args[1] == "update"
  │
  ├── os.Executable() → find binary path
  │
  ├── isHomebrewInstall(execPath)?
  │     │
  │     ├── YES → exec.Command("brew", "upgrade", "jchavarriam/tap/aimux")
  │     │           ├── success → exit 0
  │     │           └── failure → exit 1
  │     │
  │     └── NO → SelfUpdate(Version, execPath)
  │              │
  │              ├── checkWritePermission(execPath)
  │              │     └── error? → print sudo hint, exit 1
  │              │
  │              ├── fetch latest release tag from GitHub API
  │              │     └── error? → print error, exit 1
  │              │
  │              ├── semver.Compare(...) ≥ 0?
  │              │     └── YES → print "already up to date", exit 0
  │              │
  │              ├── build archive name for GOOS/GOARCH
  │              │
  │              ├── download .tar.gz to temp file
  │              │     └── error? → clean temp, exit 1
  │              │
  │              ├── extract binary from archive
  │              │
  │              ├── download checksums.txt
  │              │
  │              ├── SHA256(extracted binary) vs checksums.txt entry
  │              │     └── mismatch? → clean temp files, exit 1
  │              │
  │              ├── atomicReplace (temp → rename over current binary)
  │              │     └── error? → clean temps, exit 1
  │              │
  │              └── print "updated to vX.Y.Z. Please restart.", exit 0
```

### 8.3 Release Pipeline Flow

```
Developer: git tag v1.0.0 && git push --tags
  │
  ▼
GitHub Actions: release.yml triggered on tag push
  │
  ├── actions/checkout@v4 (fetch-depth: 0)
  ├── actions/setup-go@v5 (go 1.25)
  │
  └── goreleaser-action@v6
        │
        ├── go build (4 platforms, CGO_ENABLED=0)
        │     ├── darwin/amd64 → aimux_1.0.0_darwin_amd64
        │     ├── darwin/arm64 → aimux_1.0.0_darwin_arm64
        │     ├── linux/amd64  → aimux_1.0.0_linux_amd64
        │     └── linux/arm64  → aimux_1.0.0_linux_arm64
        │
        ├── archive each + LICENSE + README.md → .tar.gz
        │
        ├── checksums.txt (4 SHA256 entries)
        │
        ├── publish to GitHub Releases
        │     ├── aimux_1.0.0_darwin_amd64.tar.gz
        │     ├── aimux_1.0.0_darwin_arm64.tar.gz
        │     ├── aimux_1.0.0_linux_amd64.tar.gz
        │     ├── aimux_1.0.0_linux_arm64.tar.gz
        │     └── checksums.txt
        │
        └── brew tap step
              └── push Formula/aimux.rb → jchavarriam/homebrew-tap
```

---

## 9. Error Handling Per Component

### 9.1 Startup Update Check (checker.go)

| Scenario | Behavior | UX Impact |
|---|---|---|
| Cache hit, no update | No HTTP request | None (instant footer) |
| Cache hit, update available | No HTTP request | Footer shows notification |
| Cache miss, network timeout | Silent fail | Footer shows version only |
| Cache miss, rate limited (403) | Silent fail, preserve cache | Footer shows version only |
| Cache miss, success | Cache updated, check compared | Footer shows notification if update available |
| DB unavailable | Silent fail (no dependency on DB for optional feature) | Footer shows `?` version |

### 9.2 Self-Update (updater.go)

| Scenario | Behavior | Exit Code |
|---|---|---|
| Already up to date | Print message | 0 |
| Download success + checksum match | Replace binary | 0 |
| No write permission | Print sudo hint | 1 |
| GitHub API unreachable | Print error | 1 |
| Archive not found for platform | Print error | 1 |
| Download corrupt (checksum mismatch) | Remove temp, print error | 1 |
| os.Rename fails | Remove temp, print error | 1 |

### 9.3 Version Flag (main.go)

| Scenario | Behavior | Exit Code |
|---|---|---|
| Release binary | Print version, commit, date | 0 |
| Dev binary | Print "dev", "none", "unknown" | 0 |
| Invalid args after `--version` | Ignored, version printed, exit | 0 |

### 9.4 Update Cache DB (cache.go)

The cache layer uses best-effort error handling:
- `CacheGet` returns ("", nil) on any error — not ("", err). The caller treats this as a cache miss and falls through to HTTP.
- `CacheSet` returns the error. The caller (checker) ignores it — the version check succeeds even if caching fails.
- No transactions needed (single-row operations, PK upsert).

### 9.5 Release Pipeline (CI)

| Scenario | Behavior |
|---|---|
| No tag push | Workflow does not run |
| Tag without `v` prefix | Workflow does not run |
| GoReleaser build failure | Workflow fails, no release created |
| GoReleaser archive failure | Workflow fails |
| Homebrew push fails | Release still published (brew tap updated manually) |
| `secrets.GITHUB_TOKEN` insufficient for cross-repo push | Configure HOMEBREW_TAP_TOKEN secret |

---

## 10. Dependency Management

### 10.1 New Explicit Dependency

```bash
go get golang.org/x/mod@latest
```

`golang.org/x/mod/semver` is the only new Go library dependency. It is already present in `go.sum` as a transitive dependency of `golang.org/x/sys` or other `x/` packages. This makes it a zero-new-cost addition.

### 10.2 No New External Dependencies

No additional third-party libraries are introduced:
- **HTTP client**: `net/http` (stdlib)
- **JSON parsing**: `encoding/json` (stdlib)
- **Archive extraction**: `archive/tar` + `compress/gzip` (stdlib)
- **Checksum**: `crypto/sha256` (stdlib)
- **Version comparison**: `golang.org/x/mod/semver` (golang.org/x, NOT third-party)
- **SQLite**: `modernc.org/sqlite` (already used)
- **File operations**: `os` + `io` (stdlib)
- **Random suffixes**: `crypto/rand` (stdlib)

---

## 11. Security Considerations

### 11.1 Binary Integrity

- SHA256 checksum validation BEFORE overwriting the current binary
- Checksum is fetched from the SAME release as the binary (GitHub Release API)
- `os.Rename()` is atomic — no partial writes visible to the filesystem
- Temp files use random suffixes to prevent race conditions with concurrent processes

### 11.2 GitHub API Access

- No authentication tokens embedded in the binary
- Unauthenticated rate limit (60 req/hr) is sufficient for startup checks with 24h cache
- User-Agent header identifies the client (`aimux/<version>`) as per GitHub API best practices
- HTTPS only (stdlib enforces this for `https://` URLs)

### 11.3 Homebrew Update

- Delegates to `brew upgrade`, which verifies formula SHA checksum
- Formula is auto-generated by GoReleaser with the correct SHA256 for the binary
- The Homebrew tap is a separate GitHub repository owned by the same user

### 11.4 Safe Default

- Update is ALWAYS explicit (`aimux update`). Never automatic.
- No background binary modification.
- Write permission failure provides actionable guidance, not a silent hang.

---

## 12. Testing Strategy

### 12.1 Unit Tests: `updater_test.go`

| Test Case | Target |
|---|---|
| `TestIsHomebrewInstall_brewPrefixMatch` | Detection logic |
| `TestIsHomebrewInstall_brewPrefixNoMatch` | Detection logic |
| `TestIsHomebrewInstall_brewNotAvailable` | Detection logic |
| `TestValidateChecksum_match` | Checksum validation |
| `TestValidateChecksum_mismatch` | Checksum validation |
| `TestAtomicReplace_sameFilesystem` | Atomic replace |
| `TestCheckWritePermission_writable` | Permission check |
| `TestCheckWritePermission_notWritable` | Permission check |

### 12.2 Integration Tests: `checker_test.go`

| Test Case | Target |
|---|---|
| `TestCacheGet_hit` | SQLite cache with valid TTL |
| `TestCacheGet_miss_expired` | SQLite cache with expired TTL |
| `TestCacheGet_miss_missing` | SQLite cache with no entry |
| `TestCacheGet_set_and_get` | Round-trip cache |
| `TestCompareVersions_newer` | semver comparison |
| `TestCompareVersions_same` | semver comparison |
| `TestCompareVersions_older` | semver comparison |

### 12.3 What Is NOT Tested

- GitHub API live calls (tested manually or via CI on release)
- GoReleaser pipeline (tested by running `goreleaser release --snapshot` locally)
- TUI footer rendering (tested manually; Bubble Tea view testing is low value)
- Homebrew integration (tested by the `brew upgrade` command itself)

---

## 13. Migration Path for Existing Users

Users upgrading from a pre-distribution build:

1. They have a manually compiled binary at e.g. `~/go/bin/aimux` or somewhere in PATH
2. They can either:
   a. `rm $(which aimux)` then `brew install jchavarriam/tap/aimux` (clean Homebrew path)
   b. Keep the binary and run `aimux update` in the future (standalone path)
3. SQLite database at `~/.config/aimux/matrix.db` is NOT affected by the binary update
4. The `update_cache` table is added by `RunMigrations()` automatically when the new binary starts
5. No data migration needed — all existing tables are unchanged

---

## 14. Non-Goals (Explicit Constraints)

- No Windows builds or update support
- No inline progress bar during download (spinner in terminal is sufficient)
- No auto-start of TUI after update (user must run `aimux` again)
- No notification sound or visual flash when update is available
- No background goroutine polling (check once at startup)
- No changelog display in TUI
- No `--channel` flag for prerelease channels
- No binary signing or notarization
- No GitLab / Gitea / self-hosted release support
