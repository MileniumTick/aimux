# Spec: Startup Update Check

## Requirement

On application startup (before the TUI renders), perform a lightweight background check against the GitHub API for the latest release version. Cache the result in SQLite with a 24-hour TTL to stay within unauthenticated API rate limits and ensure fast startup on cache hits.

## Scope

- New package: `internal/infrastructure/update/`
- New SQLite table: `update_cache`
- Background check on startup with non-blocking notification to the TUI
- Silent failure on network errors

## Implementation

### Package Structure

```
internal/infrastructure/update/
  checker.go    -- CheckForUpdate, GitHub API interaction
  cache.go      -- SQLite cache read/write
  models.go     -- UpdateInfo struct
```

### Data Model

```go
type UpdateInfo struct {
    CurrentVersion string
    LatestVersion  string
    HasUpdate      bool
}
```

### SQLite Cache Table

```sql
CREATE TABLE IF NOT EXISTS update_cache (
    key       TEXT PRIMARY KEY,
    value     TEXT NOT NULL,
    checked_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

Migration must be added to `sqlite2.RunMigrations()` alongside existing table creation statements.

### Cache Operations (`cache.go`)

```go
// CacheGet retrieves a cached value for the given key if it was stored
// within the last 24 hours. Returns ("", nil) on cache miss.
func CacheGet(db *sql.DB, key string) (string, error)

// CacheSet stores a key-value pair with the current timestamp.
func CacheSet(db *sql.DB, key, value string) error
```

The 24-hour TTL is enforced at query time:

```sql
SELECT value FROM update_cache
WHERE key = ?
AND checked_at > datetime('now', '-24 hours')
```

Keys are constants:

```go
const CacheKeyLatestVersion = "latest_version"
```

### Checker (`checker.go`)

```go
// CheckForUpdate queries the SQLite cache first. On cache miss, fetches
// the latest release from GitHub API. Returns UpdateInfo.
// Errors are logged but never returned — the function always returns a
// valid UpdateInfo with HasUpdate=false on failure.
func CheckForUpdate(currentVersion string, db *sql.DB, httpClient *http.Client) UpdateInfo
```

GitHub API endpoint:

```
GET https://api.github.com/repos/jchavarriam/aimux/releases/latest
```

Response contains a `tag_name` field (e.g., `"v1.2.0"`). Strip the `v` prefix before comparing.

Version comparison uses `golang.org/x/mod/semver`:

```go
import "golang.org/x/mod/semver"

func compareVersions(current, latest string) bool {
    // semver.Compare returns +1 if latest > current
    return semver.Compare("v" + latest, "v" + current) > 0
}
```

The `golang.org/x/mod/semver` package must be added as an explicit dependency via `go get golang.org/x/mod`.

HTTP timeout: 5 seconds (context deadline). If the request takes longer, it fails silently.

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
req := req.WithContext(ctx)
```

### Startup Integration in `main()`

The update check runs as a non-blocking goroutine after database initialization but before TUI creation:

```go
// Launch background update check
updateInfo := make(chan update.UpdateInfo, 1)
go func() {
    info := update.CheckForUpdate(Version, db, &http.Client{Timeout: 5 * time.Second})
    updateInfo <- info
}()

// Create TUI model (pass the channel result eventually)
model := tui.NewModel(providerUseCases, switchUseCases)

// After model creation, consume update check result
// This happens before tea.NewProgram so the model has the data
// before the first render
select {
case info := <-updateInfo:
    model.SetUpdateInfo(info)
default:
    // Check didn't complete yet — model will check on first refresh
}
```

The goroutine is launched BEFORE `tea.NewProgram()` to overlap network I/O with model initialization. The model reads the result via a channel with a non-blocking `select` — if the check hasn't completed yet, the model proceeds without update data and the check completes in the background (no notification on slow networks).

### HTTP Request Details

- User-Agent header: `aimux/{Version}`
- Accept header: `application/vnd.github+json`
- No authentication (unauthenticated, 60 req/hr limit)
- On HTTP 403 (rate limit) or HTTP 304 (not modified): treat as cache miss, fail silently
- On HTTP 200: parse JSON body, extract `tag_name`, cache in SQLite
- On any other response or network error: fail silently, leave cache unchanged

## Scenarios

### S1: On app startup, check GitHub API for latest release tag

**Given** the application starts
**When** database initialization completes
**Then** a background goroutine checks `GET https://api.github.com/repos/jchavarriam/aimux/releases/latest`
**And** the TUI rendering is NOT blocked by the network request

### S2: First check succeeds, caches result in SQLite

**Given** the startup check returns a tag_name of `"v1.2.0"`
**When** the response is parsed
**Then** the value `"1.2.0"` is stored in `update_cache` with key `"latest_version"`
**And** the `checked_at` field is set to the current timestamp

### S3: Cache result in SQLite update_cache table with 24h TTL

**Given** a cached value exists from less than 24 hours ago
**When** the next startup runs
**Then** the cached value is returned without making an HTTP request
**And** the startup is instant (no network latency)

### S4: On network error, fail silently, no notification

**Given** the startup check encounters a network error (timeout, DNS failure, connection refused)
**When** the check fails
**Then** no error is shown to the user
**And** `HasUpdate` is false
**And** the existing cache (if any) is preserved

### S5: On same version, no notification

**Given** the binary version is `1.2.0` and the latest release is also `1.2.0`
**When** the version comparison runs
**Then** `HasUpdate` is false
**And** no update notification is shown

### S6: On newer version, set UpdateAvailable flag for TUI

**Given** the binary version is `1.2.0` and the latest release is `1.3.0`
**When** the version comparison runs
**Then** `HasUpdate` is true
**And** `LatestVersion` is `"1.3.0"`
**And** the model's `UpdateInfo` is set for the TUI to render

### S7: Cache is a generic key-value table

**Given** the `update_cache` table exists
**When** any component stores a cache entry
**Then** the table stores `(key TEXT, value TEXT, checked_at TEXT)`
**And** future cache entries (different keys) can reuse the same table without schema changes

### S8: Cache miss on expired entry

**Given** a cached value exists from more than 24 hours ago
**When** the next startup runs
**Then** the cache is treated as a miss (no value returned)
**And** an HTTP request is made to refresh the data
