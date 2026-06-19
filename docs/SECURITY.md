# Security Model

> aimux handles API keys, auth tokens, and config file mutations. This document describes the security boundaries, data at rest protections, integrity guarantees, and operational security considerations.

---

## Data at Rest

### SQLite Database

- **Location**: `~/.config/aimux/matrix.db`
- **Permissions**: `0600` (owner read/write only) — set at database open, **after** file creation
- **Engine**: `modernc.org/sqlite` (pure Go, no CGO, no external process)
- **WAL mode**: Enabled for concurrent read safety
- **Foreign keys**: `PRAGMA foreign_keys = ON` for referential integrity
- **Busy timeout**: 5000ms to prevent "database is locked" errors

**Sensitive columns**:

| Table | Column | Risk |
|-------|--------|------|
| `providers` | `api_key` | High — plaintext API key |
| `providers` | `auth_token` | High — plaintext auth token (may differ from API key) |

**Why plaintext**: aimux must write API keys into CLI config files. Encryption at rest in the database would require a master key, adding complexity without meaningful protection — the key would need to be accessible at runtime to decrypt credentials for config file mutation.

### Config Files

- aimux writes API keys into CLI config files (`settings.json`, `config.json`, `config.toml`, shell profiles, `models.json`)
- Config files retain their original permissions (aimux does not change them)
- Backups inherit `0644` permissions

### Backups

- **Location**: `~/.config/aimux/backups/<basename>-<hash>/`
- **Directory permissions**: `0700` (owner only)
- **File permissions**: `0644` (owner read/write, others read)
- **Retention**: 5 most recent per config file, automatically pruned
- **Hash isolation**: Backup directories keyed by SHA1 of absolute source path (first 10 hex chars). Prevents filename collision (e.g., multiple `settings.json` files).
- **Override**: `AIMUX_BACKUP_ROOT` environment variable lets users move backups to encrypted volumes

### Logs

- **Location**: `~/.config/aimux/aimux.log`
- **Permissions**: `0600`
- **Content**: Application logs only. API keys are **never** logged. Network errors, migration results, TUI warnings are logged.

---

## Data in Transit

### Provider Model Fetch & Connectivity Test

- **Protocol**: HTTPS only (aimux enforces `https://` prefix if none provided)
- **Auth header**: `Authorization: Bearer <token>` for OpenAI-compatible endpoints
- **Timeout**: 5 seconds for HTTP requests
- **User-Agent**: `aimux/<version>`
- **Response validation**: Non-JSON responses (HTML) are detected and rejected before parsing

### Self-Update Download

- **Protocol**: HTTPS via GitHub Releases API
- **Integrity**: SHA256 checksum validated against `checksums.txt` from the release
- **Atomic replace**: Binary is extracted to temp file, checksum-verified, then renamed atomically
- **Pre-check**: Write permission verified before any download

---

## Mutation Integrity

### Atomic Writes

All config file mutations use a 3-step atomic write pattern:

1. **Write to temp file** (`file.tmp.random`)
2. **Sync to disk** (`fsync`)
3. **Rename over target** (`os.Rename` is atomic on same filesystem)

On failure at any step, the temp file is cleaned up. The original file is never left in a partial state.

### Flock Locking

- **Read**: Shared lock via `gofrs/flock.RLock()` before reading config files
- **Write**: Implicit exclusive lock — read-modify-write happens within mutator's scope; config is read and written in the same call
- **Timeout**: 2 seconds. Returns `ErrFlockTimeout` if lock cannot be acquired.
- **Cross-platform**: `gofrs/flock` uses `fcntl` on Unix, `LockFileEx` on Windows

This prevents TOCTOU races when Claude Code (or another process) simultaneously reads `settings.json` while aimux mutates it.

### Backup Before Mutate

- Every `Mutate()` call creates a backup **before** reading the config file
- Backup is created if the file exists on disk (not if parse succeeds — a corrupted file should still be backed up before mutation)
- 5-backup retention with automatic pruning

### Trailing Comma Tolerance

Hand-edited JSON configs often have trailing commas. aimux strips them with regex before parsing, preventing silent parse failures that would either crash or revert to an empty config.

```go
var trailingCommaRE = regexp.MustCompile(`,(\s*[}\]])`)
```

---

## Operational Security

### Shell Profile Mutation (Copilot)

- Copilot requires environment variables in the shell process
- aimux writes to `~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`
- Managed block is wrapped in idempotent markers:

  ```bash
  # >>> aimux copilot provider
  # Managed by aimux — DO NOT EDIT BETWEEN MARKERS
  export COPILOT_PROVIDER_BASE_URL="..."
  # <<< aimux copilot provider
  ```

- `ClearCLIConfig()` removes the block cleanly — no residue left

### ANTHROPIC_API_KEY Sanitization

Claude Code's login flow interferes with `ANTHROPIC_API_KEY`. On every mutation:

- `ANTHROPIC_API_KEY` is **deleted** from `settings.json` root level
- `ANTHROPIC_AUTH_TOKEN` is written to the `env` block instead

This is a security invariant — the key must exist only in the env block, never at root.

### Homebrew Detection

`aimux update` detects if the binary is installed via Homebrew and delegates to `brew upgrade` instead of self-replacing. Prevents permission issues and package manager conflicts.

---

## Threat Model

| Threat | Mitigation |
|--------|-----------|
| Malicious binary from fake release | SHA256 checksum validation against GitHub-hosted `checksums.txt` |
| Concurrent config corruption | `gofrs/flock` with 2s timeout |
| Crash during config write | Atomic write (temp file + fsync + rename) |
| Stale temp files | Cleanup on failure; temp files have random suffix, won't conflict |
| API key leakage in logs | API keys never passed to `log.Printf()` |
| Trailing comma in hand-edited JSON | Regex-based sanitization before parsing |
| Homebrew conflict with self-update | Homebrew detection delegates to `brew upgrade` |
| API key exposure in shell history | Copilot env vars written to shell profile file, not executed in shell |
| Unauthorized backup access | Backup directory `0700`, files `0644` |
| Provider name collision in backup store | SHA1 hash of absolute path ensures unique backup directories |
