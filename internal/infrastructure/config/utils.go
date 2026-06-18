package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

var (
	ErrConfigNotFound   = errors.New("config file not found")
	ErrConfigParse      = errors.New("could not parse config file: invalid JSON")
	ErrFlockTimeout     = errors.New("could not acquire file lock: timeout")
	ErrTempFileCreate   = errors.New("could not create temporary file")
	ErrTempFileWrite    = errors.New("could not write to temporary file")
	ErrAtomicRename     = errors.New("could not rename temp file atomically")
	ErrSyncFile         = errors.New("could not sync config file to disk")
)

const flockTimeout = 2 * time.Second

// AcquireFlock attempts to acquire a flock on the given fd with a timeout.
func AcquireFlock(fd uintptr, lockType int, timeout time.Duration) error {
	ch := make(chan error, 1)
	go func() {
		ch <- syscall.Flock(int(fd), lockType)
	}()

	select {
	case err := <-ch:
		return err
	case <-time.After(timeout):
		// Attempt to cancel by sending LOCK_UN; best-effort.
		syscall.Flock(int(fd), syscall.LOCK_UN)
		return ErrFlockTimeout
	}
}

// ReadJSONWithLock reads a JSON file with a shared lock and returns its content.
// If the file does not exist, returns an empty map and no error.
// If the file exists but is empty, returns an empty map and no error.
// If the file has content but cannot be parsed as JSON, returns an error.
func ReadJSONWithLock(path string) (map[string]any, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("open config file: %w", err)
	}
	defer f.Close()

	if err := AcquireFlock(f.Fd(), syscall.LOCK_SH, flockTimeout); err != nil {
		return nil, fmt.Errorf("acquire read lock on %s: %w", path, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	// Check if file has content before decoding
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat config file: %w", err)
	}

	result := make(map[string]any)
	if err := json.NewDecoder(f).Decode(&result); err != nil {
		// Empty file — treat as empty object
		if fi.Size() == 0 {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("%w: %w", ErrConfigParse, err)
	}
	return result, nil
}

// AtomicWrite writes data to the given path atomically by writing to a temp file,
// syncing, and renaming. The directory must already exist.
func AtomicWrite(data []byte, path string) error {
	dir := filepath.Dir(path)

	tmpFile, err := os.CreateTemp(dir, "*.tmp")
	if err != nil {
		return ErrTempFileCreate
	}

	written := false
	defer func() {
		if !written {
			os.Remove(tmpFile.Name())
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return ErrTempFileWrite
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return ErrSyncFile
	}

	if err := tmpFile.Close(); err != nil {
		return ErrTempFileWrite
	}

	if err := os.Rename(tmpFile.Name(), path); err != nil {
		return ErrAtomicRename
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

// CreateBackup copies the file at path to a timestamped backup in the same directory.
// Backup name: <filename>.aimux-backup-<RFC3339 timestamp>
func CreateBackup(path string) (string, error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	timestamp := time.Now().UTC().Format(time.RFC3339)
	backupName := base + ".aimux-backup-" + timestamp
	backupPath := filepath.Join(dir, backupName)

	input, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read original file for backup: %w", err)
	}

	if err := os.WriteFile(backupPath, input, 0644); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}

	return backupPath, nil
}

// PruneBackups removes old backups beyond maxBackups, keeping the most recent ones.
// prefix is derived from filepath.Base(path) + ".aimux-backup-".
func PruneBackups(path string, max int) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	prefix := base + ".aimux-backup-"

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			backups = append(backups, filepath.Join(dir, e.Name()))
		}
	}

	if len(backups) <= max {
		return
	}

	// Sort by name (which includes timestamp, so lexicographic = chronological)
	sort.Strings(backups)

	// Remove oldest ones beyond the limit
	toRemove := len(backups) - max
	for _, bp := range backups[:toRemove] {
		os.Remove(bp)
	}
}

// PrepareDir creates the directory for the given path if it does not exist.
func PrepareDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}
