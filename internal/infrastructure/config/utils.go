package config

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

var (
	ErrConfigNotFound = errors.New("config file not found")
	ErrConfigParse    = errors.New("could not parse config file: invalid JSON")
)

var errFlockTimeout = errors.New("could not acquire file lock: timeout")
var errTempFileCreate = errors.New("could not create temporary file")
var errTempFileWrite = errors.New("could not write to temporary file")
var errAtomicRename = errors.New("could not rename temp file atomically")
var errSyncFile = errors.New("could not sync config file to disk")

const flockTimeout = 2 * time.Second

// AcquireFlock acquires a shared/exclusive lock on the given file using
// gofrs/flock (cross-platform). Uses context-based TryLockContext.
func AcquireFlock(f *os.File, exclusive bool, timeout time.Duration) error {
	fl := flock.New(f.Name())
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	lockFn := fl.TryRLockContext
	if exclusive {
		lockFn = fl.TryLockContext
	}
	ok, err := lockFn(ctx, 50*time.Millisecond)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return errFlockTimeout
	}
	return nil
}

// trailingCommaRE matches a comma followed by optional whitespace and a closing
// brace or bracket — the most common JSON syntax error in hand-edited config files.
var trailingCommaRE = regexp.MustCompile(`,(\s*[}\]])`)

// ReadJSONWithLock reads a JSON file with a shared lock and returns its content.
// If the file does not exist, returns an empty map and no error.
// If the file exists but is empty, returns an empty map and no error.
// Tolerates trailing commas (common in hand-edited JSON) by retrying with cleanup.
func ReadJSONWithLock(path string) (map[string]any, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	if err := AcquireFlock(f, false, flockTimeout); err != nil {
		return nil, fmt.Errorf("acquire read lock on %s: %w", path, err)
	}
	// gofrs/flock releases on Fd close (defer f.Close runs first), so no explicit unlock needed.

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat config file: %w", err)
	}
	if fi.Size() == 0 {
		return make(map[string]any), nil
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	result := make(map[string]any)
	if err := json.Unmarshal(data, &result); err != nil {
		// ponytail: strip trailing commas and retry — covers the most common
		// hand-edited JSON mistake without pulling in a lenient parser.
		sanitized := trailingCommaRE.ReplaceAll(data, []byte("$1"))
		if err2 := json.Unmarshal(sanitized, &result); err2 != nil {
			return nil, fmt.Errorf("%w: %w", ErrConfigParse, err)
		}
	}
	return result, nil
}

// AtomicWrite writes data to the given path atomically by writing to a temp file,
// syncing, and renaming. The directory must already exist.
func AtomicWrite(data []byte, path string) error {
	dir := filepath.Dir(path)

	tmpFile, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", errTempFileCreate)
	}

	written := false
	defer func() {
		if !written {
			os.Remove(tmpFile.Name())
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp file: %w", errTempFileWrite)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("sync file: %w", errSyncFile)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("write temp file: %w", errTempFileWrite)
	}

	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return fmt.Errorf("rename temp file atomically: %w", errAtomicRename)
	}

	written = true
	return nil
}

// WriteAtomicJSON marshals the given map to indented JSON and writes it atomically.
func WriteAtomicJSON(path string, data map[string]any) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	jsonData = append(jsonData, '\n')
	return AtomicWrite(jsonData, path)
}

// backupRootFn returns the directory where aimux stores ALL config backups.
// Overridable in tests via the package-level backupRootFn variable.
// Backups no longer live next to each CLI's config file — they are centralized
// on aimux's side so the user's tool directories stay clean.
var backupRootFn = defaultBackupRoot

func defaultBackupRoot() (string, error) {
	// AIMUX_BACKUP_ROOT overrides the default location — useful for tests and
	// for users who want backups on a different volume/path.
	if root := os.Getenv("AIMUX_BACKUP_ROOT"); root != "" {
		return root, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		hd, hdErr := os.UserHomeDir()
		if hdErr != nil {
			return "", fmt.Errorf("resolve backup root: %w", hdErr)
		}
		dir = filepath.Join(hd, ".config")
	}
	return filepath.Join(dir, "aimux", "backups"), nil
}

// backupTimestampFormat is an RFC3339 variant with no colons so it is filename-safe.
const backupTimestampFormat = "2006-01-02T15-04-05Z"

// backupDirFor returns the centralized backup directory for a given source file.
// It is keyed by a hash of the absolute path so multiple CLIs sharing a basename
// (e.g. settings.json) do not collide.
func backupDirFor(srcPath string) (string, error) {
	root, err := backupRootFn()
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(srcPath)
	if err != nil {
		abs = srcPath
	}
	sum := sha1.Sum([]byte(filepath.ToSlash(abs)))
	key := hex.EncodeToString(sum[:])[:10]
	return filepath.Join(root, filepath.Base(srcPath)+"-"+key), nil
}

// CreateBackup copies the file at path into aimux's centralized backup store
// (~/.config/aimux/backups/<base>-<hash>/<base>.<ts>) and returns that path.
func CreateBackup(path string) (string, error) {
	dir, err := backupDirFor(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create backup directory: %w", err)
	}

	input, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read original file for backup: %w", err)
	}

	backupPath := filepath.Join(dir, filepath.Base(path)+"."+time.Now().UTC().Format(backupTimestampFormat))
	if err := os.WriteFile(backupPath, input, 0644); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}
	return backupPath, nil
}

// BackupEntry describes a stored backup for a source config file.
type BackupEntry struct {
	Path string
	When string // filename timestamp segment, lexicographically sortable
}

// ListBackups returns backups for the given source path, newest first.
func ListBackups(path string) ([]BackupEntry, error) {
	dir, err := backupDirFor(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	base := filepath.Base(path) + "."
	var out []BackupEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), base) {
			continue
		}
		out = append(out, BackupEntry{
			Path: filepath.Join(dir, e.Name()),
			When: strings.TrimPrefix(e.Name(), base),
		})
	}
	// newest first (timestamp is lexicographically sortable)
	slices.SortStableFunc(out, func(a, b BackupEntry) int {
		return strings.Compare(b.When, a.When)
	})
	return out, nil
}

// PruneBackups removes old backups beyond max, keeping the most recent ones.
func PruneBackups(path string, max int) {
	backups, err := ListBackups(path)
	if err != nil || len(backups) <= max {
		return
	}
	// ListBackups is newest-first; drop the oldest (tail).
	for _, b := range backups[max:] {
		os.Remove(b.Path)
	}
}

// RestoreBackup copies a backup file back to destPath atomically.
func RestoreBackup(backupPath, destPath string) error {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}
	if err := PrepareDir(destPath); err != nil {
		return fmt.Errorf("prepare destination directory: %w", err)
	}
	return AtomicWrite(data, destPath)
}

// PrepareDir creates the directory for the given path if it does not exist.
func PrepareDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}
