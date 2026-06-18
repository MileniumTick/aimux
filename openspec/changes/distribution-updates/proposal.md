# Proposal: Distribution and Auto-Update for aimux

## Problem Statement

aimux currently has zero distribution mechanism. The only way to install it is `go build .` from source, which requires Go toolchain installation, repo cloning, and manual binary management. This imposes an unacceptable friction barrier on adoption:

- **Non-Go users cannot install at all** -- they need Go 1.23+, `git clone`, and must figure out binary placement
- **No versioning** -- no `--version` flag, no `Version` variable baked into the binary, no way for users to know what they're running
- **No upgrade path** -- even if a user has the binary, there's no notification when a new release exists
- **No cross-platform builds** -- builds happen on the host machine only (Apple Silicon macOS), leaving Intel Mac and Linux users unable to use it
- **No package manager integration** -- developers who live in Homebrew have no way to install or update aimux through their existing workflow

Users of modern CLI tools expect: `brew install aimux` -> `aimux` -> `aimux update` -> done. Without this pipeline, adoption stays at "cool project I looked at once."

**Success looks like**: a user can `brew install aimux`, run it, get a notification when a new version is available, and run `aimux update` to self-upgrade. CI publishes new builds automatically on tag push.

---

## Scope

### In Scope

1. **GoReleaser configuration** (`.goreleaser.yml`) -- cross-platform release builds:
   - Matrix: darwin/amd64, darwin/arm64, linux/amd64, linux/arm64
   - Static binaries (CGO_ENABLED=0, enabled by modernc.org/sqlite)
   - `tar.gz` archives with checksums
   - Published to GitHub Releases on version tags
   - Homebrew tap formula auto-generation

2. **GitHub Actions release workflow** (`.github/workflows/release.yml`):
   - Trigger: push of `v*` tags (semver)
   - Run GoReleaser with GORELEASER_CURRENT_TAG
   - Upload artifacts to GitHub Releases
   - Publish Homebrew formula to a tap repository

3. **Version embedding via `-ldflags`**:
   - `main.Version` -- set to GoReleaser `{{.Version}}` (tag sans `v`)
   - `main.Commit` -- set to `{{.FullCommit}}`
   - `main.Date` -- set to build date
   - `aimux --version` flag (or `aimux version` subcommand)

4. **Update notification on startup**:
   - On app start, background goroutine fetches `/repos/{owner}/{repo}/releases/latest` from GitHub API
   - Compare with embedded version using `golang.org/x/mod/semver`
   - Cache result in SQLite with 24-hour TTL to stay within unauthenticated rate limits (60 req/hr per IP)
   - Display: in-TUI footer banner ("New version X.Y.Z available. Run `aimux update` to upgrade.")
   - Silent cache hit: no network call if cache is fresh, fast startup path

5. **`aimux update` self-update command** (stretch but in scope):
   - On invocation, fetch latest release asset URL from GitHub API
   - Download the appropriate archive for `runtime.GOOS`/`runtime.GOARCH`
   - Validate checksum against published checksums.txt
   - Self-replace binary: download to temp file, verify, rename over running binary (same pattern as `gh`, `lazygit`)
   - Cleanly exit the TUI, perform update in parent process, restart

6. **Database migration for update cache table**:
   - New SQLite table: `update_cache` with columns `key TEXT UNIQUE` (e.g. `"latest_version"`), `value TEXT`, `cached_at TEXT`
   - Future-proof: generic key-value cache, not just version checks

### Out of Scope

- Windows builds (could be added later with cross-compiled `.exe` archives but out of scope)
- APT/RPM/deb packaging (Homebrew + raw binary is sufficient for early adoption)
- Docker images (single-binary CLI doesn't benefit from containerization)
- Auto-update on startup (explicit user action only via `aimux update` -- safety principle)
- Rolling releases / prerelease channels (tags only; `latest` maps to latest stable tag)
- Telemetry or usage reporting
- Notarization / code signing (future concern for broader distribution)
- Snap / Flatpak / Scoop / Chocolatey packaging
- Inline update progress bar in the TUI (download happens after TUI exits, spinner in terminal is sufficient)

---

## Approach

### Distribution Flow

```
Developer tags v0.2.0 and pushes
            │
            ▼
  GitHub Actions: release workflow
            │
    ┌───────┴──────────┐
    │                   │
    ▼                   ▼
  GoReleaser          GoReleaser
  (build+archive)     (brew tap)
    │                   │
    ▼                   ▼
  GitHub Release     Push formula to
  (4 .tar.gz +       github.com/jchavarriam/
   checksums.txt)    homebrew-tap/Formula/
                      aimux.rb
```

### Update Check Flow (startup)

```
aimux starts
    │
    ▼
  SQLite: SELECT value FROM update_cache
  WHERE key='latest_version' AND
  cached_at > datetime('now', '-24 hours')
    │
    ├── cache HIT → compare cached version with binary Version
    │                 │
    │                 └── newer? → show notification in TUI footer
    │                                 │
    │                                 └── same? → silent, no notification
    │
    └── cache MISS → GET /repos/jchavarriam/aimux/releases/latest
                       │
                       ├── success → INSERT/UPDATE cache, compare versions
                       │               │
                       │               └── newer? → show notification
                       │
                       └── error (timeout, rate-limit) → silent, no notification
```

### Self-Update Flow (`aimux update`)

```
User runs `aimux update` (or selects from TUI menu)
    │
    ▼
  Fetch latest release tag from GitHub API
    │
    ├── same as running version → "Already up to date"
    │
    └── newer → fetch release assets for GOOS/GOARCH
    │           download .tar.gz, extract binary
    │           validate SHA256 from checksums.txt
    │           write to temp file in same directory
    │           os.Rename() over current binary
    │           restart as child process
    │
    ▼
  "Updated to vX.Y.Z. Restarting..."
```

### Version Variables (main.go)

```go
package main

var (
    Version = "dev"      // set by ldflags at build time
    Commit  = "none"     // set by ldflags at build time
    Date    = "unknown"  // set by ldflags at build time
)
```

GoReleaser ldflags:

```yaml
ldflags:
  - -s -w
  - -X main.Version={{.Version}}
  - -X main.Commit={{.FullCommit}}
  - -X main.Date={{.Date}}
```

### GoReleaser Configuration

```yaml
# .goreleaser.yml
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

### Update Checker Package

New package: `internal/infrastructure/update/`

Components:
- `checker.go` -- `CheckForUpdate(currentVersion string, db *sql.DB) (*UpdateInfo, error)`
  - Reads from SQLite cache first (24h TTL)
  - Cache miss: HTTP GET GitHub API `/repos/jchavarriam/aimux/releases/latest`
  - Compares semver, returns `UpdateInfo{LatestVersion, CurrentVersion, HasUpdate bool}`
  - Silently handles errors (timeout, rate-limit, network failure)
- `updater.go` -- `SelfUpdate(targetVersion string) error`
  - Fetches release assets for matching platform
  - Downloads archive, extracts binary, validates checksum
  - Temp-file-write-then-rename pattern
  - Handles the case where the binary has been updated while the process runs
- `cache.go` -- SQLite cache operations (generic key-value)

### Database Schema Addition

```sql
CREATE TABLE IF NOT EXISTS update_cache (
    key       TEXT PRIMARY KEY,
    value     TEXT NOT NULL,
    cached_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

This is a generic key-value cache table. For the initial use case, the key will be `"latest_version"` and value will be the semver string. Future uses could cache GitHub release IDs, checksums, or other metadata.

### CLI Entry Point Changes

Currently `main()` starts the TUI unconditionally. Need to introduce:

1. Parse flags at the top of `main()`:
   - `--version` / `version` subcommand: print version and exit
   - `update` subcommand: run self-update (TUI exit, parent-process update)

2. Keep the TUI as the default (no flags, no subcommand = TUI)

3. For `aimux update`, the flow is:
   - Initialize DB (needed for cache reads)
   - If not already up-to-date: fetch, download, replace, restart
   - The TUI is NOT started for the update command -- it's a pure CLI operation

---

## Architecture Decisions

### AD-01: Explicit Update Command Over Auto-Update

**Status**: Accepted

**Context**: Many tools auto-update on startup or periodically in the background. This can surprise users, break workflows, or introduce latency at startup.

**Decision**: Updates are always explicit -- `aimux update` (or selecting from the TUI menu). Startup does a lightweight network check and shows a notification but NEVER modifies the binary. This follows the pattern established by `gh`, `lazygit`, and `kubectl`.

**Consequence**: Users must consciously choose to update. Slightly higher friction for the user, but zero risk of surprise upgrades breaking their workflow.

### AD-02: SQLite Cache Over HTTP Cache Headers

**Status**: Accepted

**Context**: The GitHub API's `latest` endpoint returns HTTP cache headers (ETag/If-None-Match), which could be used to reduce bandwidth. However, the API rate limit for unauthenticated requests is 60/hour, and a real concern is hitting this limit during development or testing.

**Decision**: Use a SQLite cache layer with 24-hour TTL as the primary gate. HTTP cache headers are a secondary optimization (we'll set `If-None-Match` if available). The SQLite cache gives us transparent, persistent storage that works across process restarts without relying on the OS-level HTTP cache.

**Consequence**: One extra SQLite table. The cache table is generic enough to be reused for other ephemeral data (release notes, asset URLs).

### AD-03: GoReleaser Over Manual Release Scripting

**Status**: Accepted

**Context**: We could write a shell script or Makefile to build for all platforms, create archives, generate checksums, and publish to GitHub Releases. This is how many early-stage Go projects start.

**Decision**: Use GoReleaser from day one. It's the de-facto standard for Go CLI distribution, handles the full pipeline (build, archive, checksum, Homebrew formula generation, GitHub Release publishing) in a single config file with one `goreleaser release` command. The learning curve is small, and the setup cost is recovered on the second release.

**Consequence**: One additional CI dependency (GoReleaser action). No more flexibility than a custom script, but significantly less maintenance burden.

### AD-04: Generic Update Cache Table Over Dedicated Columns

**Status**: Accepted

**Context**: The version check cache could be stored as a dedicated column in `target_clis` or a purpose-built table with a `latest_version` column and `checked_at` timestamp.

**Decision**: Create a generic `update_cache` key-value table. This is more flexible for future needs (caching release asset URLs, checksum data, rate-limit backoff state) without requiring schema migrations for each new caching need.

**Consequence**: Slightly less type safety at the SQL level. Application code must validate that values are valid semver strings.

### AD-05: Self-Replace Binary Pattern (Temp + Rename)

**Status**: Accepted

**Context**: Updating a running binary is tricky on POSIX systems. The binary can be deleted or renamed while running (the inode remains mapped), but the replacement must be careful about permissions, atomicity, and concurrent processes.

**Decision**: Standard temp-file-then-rename pattern:
1. Write new binary to `<binary-path>.tmp.<random>` in the same directory
2. On success, `os.Rename()` over the current binary
3. Spawn a new process pointing to the replaced binary
4. Exit the current process

This is the same pattern used by `gh`, `lazygit`, and Helm.

**Consequence**: The temporary file is cleaned up on success (rename succeeds) or left as a `.tmp.*` artifact on failure. The restart pattern means the TUI must exit cleanly before the update begins.

---

## Risks and Mitigations

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| GitHub API rate limit (60/hr unauthenticated) | Medium | High | SQLite cache with 24h TTL; silent failure on rate-limit; ETag caching as a secondary optimization |
| Self-update leaves inconsistent binary on crash/during write | High | Low | Write to temp file first; `os.Rename()` is atomic on POSIX; verify checksum before renaming |
| GoReleaser config drift with Go version updates | Low | Medium | Pin Go version in GoReleaser config and GitHub Actions; match project's go.mod version |
| Homebrew formula contains hardcoded SHA that gets stale on new release | Low | Low | GoReleaser auto-updates the formula SHA on each release; only an issue if the release publish fails halfway |
| `--version` flag conflicts with Bubble Tea TUI startup | Low | Low | Parse flags BEFORE initializing Tea; if `--version` is set, print and exit before any DB/TUI init |
| Version comparison fails on non-standard tags (pre-release, rc) | Low | Low | Use `golang.org/x/mod/semver.Compare()` which handles pre-release suffixes correctly; rely on `latest` endpoint which ignores pre-releases |
| Network timeout on startup check delays TUI rendering | Medium | Medium | Use minimal context deadline (5s); non-blocking goroutine; TUI shows immediately, notification arrives asynchronously via `tea.Cmd` |
| binary is in read-only location (/usr/local/bin without sudo) | Medium | Medium | Detect write permission on binary path before attempting update; show clear error message suggesting `sudo aimux update` or manual install |
| Cross-platform build (linux/amd64) produces binary that doesn't run on older glibc | Medium | Low | `CGO_ENABLED=0` gives fully static binaries with no glibc dependency; `-trimpath` for reproducibility |

---

## Estimated Effort

| Module | Files | Estimated LOC | Complexity |
|--------|-------|---------------|------------|
| Version variables + `--version` flag | 1 | 20 | Low |
| GoReleaser config | 1 | 60 | Low |
| GitHub Actions release workflow | 1 | 40 | Low |
| Update cache table migration | 1 | 20 | Low |
| Update checker (GitHub API + SQLite cache) | 3 | 180 | Medium |
| Self-updater (download, checksum, replace) | 2 | 150 | Medium-High |
| TUI notification integration | 1 | 40 | Medium |
| `aimux update` CLI command + TUI menu item | 2 | 60 | Medium |
| Documentation + README install instructions | 1 | 30 | Low |
| **Total** | **13** | **~600** | — |

---

## Rollback Plan

### Per-Phase Rollback

**Phase 1 (GoReleaser + CI)**: Delete `.goreleaser.yml` and `.github/workflows/release.yml`. Revert to manual `go build .`. No state impact.

**Phase 2 (Version embedding)**: Revert `-ldflags` from CI config and `var Version` changes. Remove `--version` flag. No state impact.

**Phase 3 (Update notification)**: Revert `update_cache` migration (DROP TABLE IF EXISTS update_cache). Remove `internal/infrastructure/update/` package. State loss is acceptable (cache is ephemeral).

**Phase 4 (Self-update)**: Remove `aimux update` command from CLI entry point. Remove `SelfUpdate()` from the update package. No state impact.

### Full Rollback

1. **Remove binary**: `rm $(which aimux)` or `brew uninstall aimux`
2. **Remove state**: `rm -rf ~/.config/aimux/` (deletes matrix.db and update_cache)
3. **Remove tap**: `brew untap jchavarriam/tap`
4. **Remove CI**: Delete `.github/workflows/release.yml`
5. **Remove GitHub Release**: Delete the release and tag from GitHub (irreversible if users have downloaded, but prevents further installs)

The update_cache table is purely ephemeral -- losing it only means a one-time cache miss, which hits GitHub API once.

---

## Proposal Question Round

The following questions should be resolved before proceeding to specs/design:

1. **Homebrew tap repository name**: Should the tap be `homebrew-tap` (generic, could host other tools) or `homebrew-aimux` (specific)? GoReleaser default convention is `homebrew-tap` for a mono-tap.

2. **Update notification UX**: Should the notification appear as a one-time popup/flash in the TUI, a persistent footer, or a status bar icon? Persistent footer is simpler but the user can't dismiss it. A tea.Msg that shows once per session is more polished.

3. **`aimux update` during active TUI**: Should `aimux update` exit the TUI entirely, perform the update, then restart the TUI? Or should it be a CLI-only command that never enters the TUI? The self-replace pattern works best outside the TUI (no Bubble Tea state to manage).

4. **Pre-release tags**: Should `v0.x.y` tags publish releases? The current version is pre-1.0. If we publish pre-1.0 releases, do we want a separate Homebrew formula version or just push the latest formula update?

---

## Assumptions (Requiring User Review)

- **GoReleaser v2** is the current stable version; config uses `version: 2`
- **GitHub Actions** is the CI provider (project already on GitHub)
- **Homebrew tap** at `github.com/jchavarriam/homebrew-tap` -- the repo does not exist yet and must be created before the first release
- **CGO_ENABLED=0** is safe because modernc.org/sqlite is pure Go (no CGO dependency) and there are no other CGO requirements in the dependency tree
- **`golang.org/x/mod/semver`** is not yet a direct dependency but is transitively available via `golang.org/x/sys`; will add as explicit dependency
- **Unauthenticated GitHub API** (60 req/hr) is sufficient for development and the 24h cache ensures we never hit the limit in practice
- **Update notifications are informational only** -- the user must explicitly run `aimux update` to upgrade
- **Self-update assumes write permission** on the binary's current location. If the binary is in a protected directory, `sudo aimux update` is required (documented)
- **No Windows support** for the initial release pipeline (darwin + linux only)
- **Seed targets**: All 5 default CLIs (claude-code, opencode, codex, github-copilot, pi-ai) are already seeded in the current codebase via `SeedTargetCLIs()` -- no additional work needed there
