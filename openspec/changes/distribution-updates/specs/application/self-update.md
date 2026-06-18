# Spec: Self-Update Command

## Requirement

The `aimux update` command detects how aimux was installed and performs the appropriate upgrade — using Homebrew for Homebrew installs, or a direct binary download-and-replace for standalone installs. The TUI is NOT started for this command; it's a pure CLI operation.

## Scope

- New `SelfUpdate()` function in `internal/infrastructure/update/updater.go`
- CLI entry point in `main.go`: parse `update` subcommand before TUI creation
- Homebrew detection via `brew --prefix`
- Atomic binary replacement (temp-file-then-rename)
- Github API integration for release asset download
- Check Homebrew tap formula for the correct upgrade command

## Implementation

### CLI Entry Point

In `main()`, BEFORE database initialization, parse the first argument:

```go
func main() {
    // Check for simple flags first (no DB needed)
    if len(os.Args) > 1 {
        switch os.Args[1] {
        case "--version", "version":
            fmt.Printf("aimux %s (commit %s, built %s)\n", Version, Commit, Date)
            os.Exit(0)
        case "update":
            // Defer to update handler
            os.Exit(runUpdate())
        }
    }

    // ... rest of existing main() (DB init, TUI) ...
}
```

The `runUpdate()` function:

```go
func runUpdate() int {
    // Resolve binary path
    execPath, err := os.Executable()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: cannot determine binary path: %v\n", err)
        return 1
    }

    // Detect install method
    if isHomebrewInstall(execPath) {
        return homebrewUpdate()
    }

    // Standalone binary update
    return selfUpdate(Version, execPath)
}
```

### Homebrew Detection

```go
func isHomebrewInstall(execPath string) bool {
    // Check if brew is available
    cmd := exec.Command("brew", "--prefix")
    output, err := cmd.Output()
    if err != nil {
        return false // brew not installed
    }

    brewPrefix := strings.TrimSpace(string(output))
    // Check if binary is inside the Homebrew prefix
    return strings.HasPrefix(execPath, brewPrefix)
}
```

### Homebrew Update

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

### Standalone Self-Update (`updater.go`)

```go
// SelfUpdate downloads the latest release for the current platform,
// checks the SHA256 checksum, replaces the current binary atomically,
// and prints the new version. Does NOT restart the process — the user
// must restart manually.
func SelfUpdate(currentVersion, execPath string) error
```

#### Flow

1. Fetch latest release tag and assets from GitHub API
2. Match asset for `runtime.GOOS` / `runtime.GOARCH`
3. Download asset to a temporary file in the same directory as the binary
4. Download and validate `checksums.txt`
5. Compute SHA256 of downloaded binary and compare
6. On match: `os.Rename()` temp file over current binary
7. On mismatch: remove temp file, return error
8. Print new version, exit

#### Asset URL Resolution

```go
func getAssetURL(goos, goarch, latestTag string) (downloadURL, checksumURL string, err error) {
    // GET https://api.github.com/repos/jchavarriam/aimux/releases/latest
    // Parse JSON to find release tag
    // Build archive name: aimux_{version}_{goos}_{goarch}.tar.gz
    // Find asset with matching name
    // checksums.txt is always at the same release
}
```

Archive name template (mirrors GoReleaser config):
- `aimux_{version}_darwin_amd64.tar.gz`
- `aimux_{version}_darwin_arm64.tar.gz`
- `aimux_{version}_linux_amd64.tar.gz`
- `aimux_{version}_linux_arm64.tar.gz`

#### Atomic Replace

```go
func atomicReplace(downloadPath, targetPath string) error {
    // Write to temp file in the same directory
    tmpPath := targetPath + ".tmp." + randomString(8)
    if err := os.Rename(downloadPath, tmpPath); err != nil {
        return fmt.Errorf("rename to temp: %w", err)
    }

    // Set executable permissions
    if err := os.Chmod(tmpPath, 0755); err != nil {
        os.Remove(tmpPath)
        return fmt.Errorf("chmod: %w", err)
    }

    // Atomic rename over target
    if err := os.Rename(tmpPath, targetPath); err != nil {
        os.Remove(tmpPath) // Clean up temp file
        return fmt.Errorf("rename over binary: %w", err)
    }

    return nil
}
```

Key points:
- Temp file is in the same directory as the target binary (ensures same filesystem for `os.Rename()`)
- `os.Rename()` is atomic on POSIX systems
- On failure at any step, clean up the temp file
- The temp file name includes a random suffix to avoid collisions with concurrent processes

#### Checksum Validation

```go
func validateChecksum(binaryPath, expectedSHA256 string) error {
    f, err := os.Open(binaryPath)
    if err != nil {
        return err
    }
    defer f.Close()

    h := sha256.New()
    if _, err := io.Copy(h, f); err != nil {
        return err
    }

    actual := hex.EncodeToString(h.Sum(nil))
    if !strings.EqualFold(actual, expectedSHA256) {
        return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedSHA256, actual)
    }
    return nil
}
```

### Write Permission Check

Before any download, check that the binary's directory is writable:

```go
func checkWritePermission(execPath string) error {
    dir := filepath.Dir(execPath)
    testFile := filepath.Join(dir, ".aimux_update_test")
    f, err := os.Create(testFile)
    if err != nil {
        return fmt.Errorf("no write permission in %s: %w\nTry: sudo aimux update", dir, err)
    }
    f.Close()
    os.Remove(testFile)
    return nil
}
```

### Already Up-to-Date Check

Before downloading, check if the current version matches the latest:

```go
latestVersion, err := fetchLatestVersion(httpClient)
if err != nil {
    return fmt.Errorf("failed to check latest version: %w", err)
}

if semver.Compare("v"+currentVersion, "v"+latestVersion) >= 0 {
    fmt.Printf("aimux v%s is already up to date.\n", currentVersion)
    return nil
}
```

### Cleanup on Error

- Downloaded archive is removed after extraction
- Temp binary file is removed if checksum validation fails
- Temp binary file is removed if `os.Rename()` fails
- Only the final `os.Rename()` success leaves no temp files

### Output on Success

```go
fmt.Printf("aimux updated from v%s to v%s. Please restart aimux.\n", currentVersion, latestVersion)
```

The user must manually restart the application. No automatic restart.

## Scenarios

### S1: `aimux update` checks how aimux was installed

**Given** the user runs `aimux update`
**When** the update flow starts
**Then** it checks if the binary is inside the Homebrew prefix
**And** selects the appropriate update method

### S2: Homebrew install: runs `brew upgrade`

**Given** the binary is installed via Homebrew
**When** the user runs `aimux update`
**Then** `brew upgrade jchavarriam/tap/aimux` is executed
**And** stdout and stderr are passed through to the terminal
**And** the process exits with brew's exit code
**And** the TUI is NOT started

### S3: Standalone binary: downloads latest from GitHub Releases

**Given** the binary is a standalone installation (not Homebrew)
**And** a newer version exists on GitHub
**When** the user runs `aimux update`
**Then** the latest release assets are fetched from the GitHub API
**And** the matching platform archive is downloaded (`aimux_1.2.0_darwin_arm64.tar.gz`)
**And** the binary is extracted from the archive
**And** the SHA256 checksum is validated against `checksums.txt`

### S4: Atomic replace: download to temp file, chmod, rename over current binary

**Given** the checksum validates successfully
**When** the binary is written
**Then** it is first written to a temp file in the same directory as the current binary
**And** the temp file is renamed to the current binary's path
**And** the rename is atomic (no partial writes visible to the filesystem)

### S5: After update, prints new version and exits

**Given** the update completes successfully
**When** the binary is replaced
**Then** the message `aimux updated from v1.2.0 to v1.3.0. Please restart aimux.` is printed
**And** exit code is 0
**And** the TUI is NOT started

### S6: Already at latest: prints "already up to date"

**Given** the current binary is at version `1.3.0`
**And** the latest release is also `1.3.0`
**When** the user runs `aimux update`
**Then** the message `aimux v1.3.0 is already up to date.` is printed
**And** exit code is 0
**And** no HTTP download is performed
**And** the TUI is NOT started

### S7: Binary in protected directory shows actionable error

**Given** the binary is in `/usr/local/bin` (writable only by root)
**When** the user runs `aimux update`
**Then** the message `Error: no write permission in /usr/local/bin: permission denied\nTry: sudo aimux update` is printed
**And** exit code is 1

### S8: Network error during update shows error

**Given** the GitHub API is unreachable
**When** the user runs `aimux update`
**Then** an error message is printed to stderr
**And** exit code is 1
**And** the current binary is not modified

### S9: Checksum mismatch does not overwrite binary

**Given** the downloaded binary's checksum does not match `checksums.txt`
**When** the checksum validation runs
**Then** the temp file is removed
**And** the current binary is NOT modified
**And** an error about checksum mismatch is printed
**And** exit code is 1
