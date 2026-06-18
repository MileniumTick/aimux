package application

import (
	"fmt"
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

// ResolveConfigPath returns the resolved path for the aimux SQLite database,
// ensuring the config directory exists.
func ResolveConfigPath() (string, error) {
	home, err := getHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	configDir := filepath.Join(home, ".config", "aimux")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}

	return filepath.Join(configDir, "matrix.db"), nil
}

// ResolveTargetConfigPath resolves a stored config path for a target CLI.
func ResolveTargetConfigPath(targetCLIConfigPath string) (string, error) {
	return ExpandTilde(targetCLIConfigPath)
}
