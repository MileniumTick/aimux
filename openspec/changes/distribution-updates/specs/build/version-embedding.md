# Spec: Version Embedding

## Requirement

Version metadata must be baked into the binary at build time via `-ldflags`, so the binary can report its version, commit hash, and build date at runtime without any external files.

## Scope

- Declare `Version`, `Commit`, and `Date` package-level variables in `main.go`
- Wire `-ldflags` in both `.goreleaser.yml` and development builds (via Makefile or `go build` command)
- Implement `--version` flag handling in `main()` to print version info and exit before any database or TUI initialization

## Implementation

### Variables in `main.go`

Add these package-level variables:

```go
var (
    Version = "dev"
    Commit  = "none"
    Date    = "unknown"
)
```

`Version` defaults to `"dev"` when not built through GoReleaser (local development builds).

### `--version` Flag Handling

In `main()`, before any database or TUI initialization:

```go
flag.Bool("version", false, "print version and exit")
flag.Parse()

if versionFlag {
    fmt.Printf("aimux %s (commit %s, built %s)\n", Version, Commit, Date)
    os.Exit(0)
}
```

The version check must happen before:
- `application.ResolveConfigPath()` (no config directory needed)
- `sqlite2.Open()` (no database needed)
- `sqlite2.RunMigrations()` (no database needed)
- TUI initialization

### LDflags Mapping

GoReleaser sets these variables:

| Variable | LDflag Value | Description |
|----------|-------------|-------------|
| `main.Version` | `{{.Version}}` | Tag sans `v` prefix (e.g. `1.2.0`) |
| `main.Commit` | `{{.FullCommit}}` | Full SHA of the commit |
| `main.Date` | `{{.Date}}` | RFC3339 build timestamp |

Development builds always show `dev` / `none` / `unknown` for all three fields.

## Scenarios

### S1: `aimux --version` prints version, commit, build date

**Given** a release build with ldflags set
**When** the user runs `aimux --version`
**Then** output is `aimux 1.2.0 (commit abcdef123456..., built 2026-06-18T12:00:00Z)`
**And** exit code is 0

### S2: `aimux --version` shows "dev" when not built with ldflags

**Given** a local development build (no ldflags)
**When** the user runs `aimux --version`
**Then** output is `aimux dev (commit none, built unknown)`
**And** exit code is 0

### S3: Version flag does not initialize database or TUI

**Given** the binary is run with `--version`
**When** it prints version info and exits
**Then** no database file is created or opened
**And** no TUI is initialized
**And** exit code is 0

### S4: No flag starts TUI normally

**Given** the binary is run with no arguments
**When** it starts up
**Then** the TUI renders as normal
**And** no version banner is printed to stdout
