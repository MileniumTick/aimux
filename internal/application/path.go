package application

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	homeDirOnce sync.Once
	homeDir     string
	homeDirErr  error
)

func getHomeDir() (string, error) {
	homeDirOnce.Do(func() {
		homeDir, homeDirErr = os.UserHomeDir()
	})
	return homeDir, homeDirErr
}

// ExpandTilde expands a tilde-prefixed path using os.UserHomeDir().
// If the path does not start with "~", it returns filepath.Clean(path).
func ExpandTilde(path string) (string, error) {
	if strings.HasPrefix(path, "~") {
		hd, err := getHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if len(path) == 1 {
			// Just "~"
			return filepath.Clean(hd), nil
		}
		// "~/..." or "~foo/..."
		return filepath.Join(hd, path[1:]), nil
	}
	return filepath.Clean(path), nil
}

// ResolveConfigDir returns the aimux config directory.
// Uses os.UserConfigDir() which returns:
//
//	Unix:    ~/.config/aimux
//	Windows: %APPDATA%/aimux (usually C:\Users\<user>\AppData\Roaming\aimux)
//	macOS:   ~/Library/Application Support/aimux
func ResolveConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback: use home/.config
		home, homeErr := getHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("resolve config directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	configDir = filepath.Join(configDir, "aimux")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	return configDir, nil
}

// ResolveConfigPath returns the resolved path for the aimux SQLite database.
func ResolveConfigPath() (string, error) {
	configDir, err := ResolveConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "matrix.db"), nil
}

// log package to write there with timestamps. Errors go to both stderr and
// the log file. Call once at startup.
func SetupLogFile() (func(), error) {
	configDir, err := ResolveConfigDir()
	if err != nil {
		return nil, err
	}

	logPath := filepath.Join(configDir, "aimux.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	// Write to both stderr and log file
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	return func() { f.Close() }, nil
}

// ResolveTargetConfigPath resolves a stored config path for a target CLI.
func ResolveTargetConfigPath(targetCLIConfigPath string) (string, error) {
	return ExpandTilde(targetCLIConfigPath)
}
