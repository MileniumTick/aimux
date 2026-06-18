# Path — Multi-Shell Path Resolution Spec

## Scope

All filesystem path logic in aimux. Paths are resolved at runtime using only `os.UserHomeDir()`. No shell scripts, no `$SHELL` detection, no shell expansion via `os/exec`.

## Core Principle

The tool MUST resolve `~` to the user's home directory using Go's `os.UserHomeDir()` exclusively. All other path expansion mechanisms (shell expansion, env var substitution in paths, tilde expansion via shell) are explicitly forbidden.

## Resolver Service

### `ResolvePath(path string) -> (string, error)`

- Accepts a path string that MAY start with `~` or `~/`.
- If the path starts with `~`: replace the `~` prefix with the value of `os.UserHomeDir()`, joined without extra separator:
  - `"~/.config/aimux/matrix.db"` -> `"/Users/username/.config/aimux/matrix.db"`
  - `"~"` -> `"/Users/username"`
- If the path does not start with `~`, return `filepath.Clean(path)`.
- If `os.UserHomeDir()` returns an error, propagate it.

### `ResolveConfigPath() -> string`

- Returns `ResolvePath("~/.config/aimux/matrix.db")`.
- MUST call `os.MkdirAll("~/.config/aimux/", 0700)` on first use to ensure the directory exists.

### `ResolveTargetConfigPath(targetCLIConfigPath string) -> string`

- Resolves the stored config path from `target_clis.config_path` using `ResolvePath`.
- The path is read from the database, no additional resolution logic.

## Default Paths

| Item | Default Path | Configurable? |
|------|-------------|---------------|
| SQLite database | `~/.config/aimux/matrix.db` | No (MVP) |
| aimux config directory | `~/.config/aimux/` | No (MVP) |
| Claude Code settings | `~/.config/claude/settings.json` | Via `target_clis.config_path` |

## Constraints

- NO shell invocation: `os/exec.Command("echo", "$HOME")` or similar MUST NOT be used.
- NO `os.Getenv("HOME")` — use `os.UserHomeDir()` exclusively.
- `os.UserHomeDir()` MUST be called exactly once per program startup and the result cached for the lifetime of the process.
- On non-macOS systems (Linux), `os.UserHomeDir()` reads `$HOME` environment variable, which is the standard mechanism. No additional logic needed.

## Acceptance Scenarios

### Standard Tilde Expansion

Given a path `"~/.config/aimux/matrix.db"`  
When `ResolvePath` is called  
Then the result is `"/Users/testuser/.config/aimux/matrix.db"` when `os.UserHomeDir()` returns `"/Users/testuser"`

### No Tilde in Path

Given a path `"/etc/aimux/matrix.db"`  
When `ResolvePath` is called  
Then the result is `"/etc/aimux/matrix.db"` (unchanged, cleaned)

### Home Directory Error

Given `os.UserHomeDir()` returns an error  
When `ResolvePath` is called with any tilde-containing path  
Then the error is propagated and the call fails

### Config Directory Creation

Given `~/.config/aimux/` does not exist  
When `ResolveConfigPath` is called for the first time  
Then `os.MkdirAll("~/.config/aimux/", 0700)` is called  
And the directory is created with permissions 0700
