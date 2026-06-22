package mutators

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// CopilotShellProfile mutates the user's shell profile to set COPILOT_PROVIDER_*
// environment variables. Copilot doesn't read .env files — it only reads real
// process environment variables, so we write to the shell's rc file instead.
// Registered as: "copilot-shell-profile"
type CopilotShellProfile struct{}

// shellProfileCmds returns the profile file path, an export line prefix, and a
// "source" style suffix for the detected shell.
func shellProfileCmds() (profilePath string, exportFmt string, unsetSuffix string, err error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		// fallback: try to detect via getent/pwutil
		sh, lookupErr := exec.LookPath("sh")
		if lookupErr == nil {
			shell = sh
		} else {
			shell = "/bin/bash" // safest default
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", fmt.Errorf("resolve home directory: %w", err)
	}

	switch {
	case strings.Contains(shell, "fish"):
		return filepath.Join(home, ".config", "fish", "config.fish"),
			"set -gx %s \"%s\"",
			"set -e %s",
			nil
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zshrc"),
			"export %s=\"%s\"",
			"unset %s",
			nil
	default: // bash and everything else
		return filepath.Join(home, ".bashrc"),
			"export %s=\"%s\"",
			"unset %s",
			nil
	}
}

const (
	shellBlockStart = "# >>> aimux copilot provider"
	shellBlockEnd   = "# <<< aimux copilot provider"
)

// Mutate writes or updates the COPILOT_PROVIDER_* env vars in the user's shell
// profile, wrapped in markers so the block can be idempotently replaced or removed.
func (m *CopilotShellProfile) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("copilot shell profile mutator not supported on Windows — use WSL or another CLI")
	}
	profilePath, exportFmt, _, err := shellProfileCmds()
	if err != nil {
		return nil, err
	}

	// Build env var lines
	var envLines []string

	// Provider type
	providerType := "openai"
	if pt, ok := mutatorConfig["provider_type"].(string); ok && pt != "" {
		providerType = pt
	}

	isLocal := false
	if l, ok := mutatorConfig["local"].(bool); ok {
		isLocal = l
	}

	envLines = append(envLines, fmt.Sprintf(exportFmt, "COPILOT_PROVIDER_BASE_URL", provider.BaseURL))
	envLines = append(envLines, fmt.Sprintf(exportFmt, "COPILOT_PROVIDER_TYPE", providerType))

	if !isLocal && provider.APIKey != "" {
		envLines = append(envLines, fmt.Sprintf(exportFmt, "COPILOT_PROVIDER_API_KEY", provider.APIKey))
	}
	if !isLocal && provider.AuthToken != "" && provider.AuthToken != provider.APIKey {
		envLines = append(envLines, fmt.Sprintf(exportFmt, "COPILOT_PROVIDER_BEARER_TOKEN", provider.AuthToken))
	}

	// Model mapping with context suffix.
	// Priority: 1) COPILOT_MODEL from modelMappings (env-var mapping flow),
	//           2) first model from _registered_models (multi-select flow),
	//           3) first non-empty value from any modelMapping key.
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	var copilotModel string

	if m, ok := modelMappings["COPILOT_MODEL"]; ok && m != "" {
		copilotModel = m
	} else if reg, ok := mutatorConfig["_registered_models"]; ok {
		switch v := reg.(type) {
		case []string:
			if len(v) > 0 {
				copilotModel = v[0]
			}
		case []any:
			if len(v) > 0 {
				copilotModel = fmt.Sprintf("%v", v[0])
			}
		}
	}

	if copilotModel == "" {
		for _, val := range modelMappings {
			if val != "" {
				copilotModel = val
				break
			}
		}
	}

	if copilotModel != "" {
		if md, ok := modelMeta[copilotModel].(map[string]any); ok {
			suffix := config.LookupContextSuffix(md)
			if suffix != "" {
				copilotModel = copilotModel + suffix
			}
		}
		envLines = append(envLines, fmt.Sprintf(exportFmt, "COPILOT_MODEL", copilotModel))
	}

	// Extra env vars from mutator_config.extra_env
	if extra, ok := mutatorConfig["extra_env"].(map[string]any); ok {
		for k, v := range extra {
			envLines = append(envLines, fmt.Sprintf(exportFmt, k, fmt.Sprintf("%v", v)))
		}
	}

	// Build the marker-block
	block := shellBlockStart + "\n" +
		"# Managed by aimux — DO NOT EDIT BETWEEN MARKERS\n" +
		strings.Join(envLines, "\n") + "\n" +
		shellBlockEnd + "\n"

	// Read existing profile
	existing := ""
	if _, statErr := os.Stat(profilePath); statErr == nil {
		data, readErr := os.ReadFile(profilePath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", profilePath, readErr)
		}
		existing = string(data)
	}

	// Replace existing block or append at end
	var newContent string
	startIdx := strings.Index(existing, shellBlockStart)
	endIdx := strings.Index(existing, shellBlockEnd)

	if startIdx >= 0 && endIdx >= 0 {
		// Replace existing aimux block
		endIdx += len(shellBlockEnd)
		newContent = existing[:startIdx] + block + existing[endIdx:]
	} else {
		// Append at end of file
		if existing != "" {
			newContent = existing + "\n" + block
		} else {
			newContent = block
		}
	}

	if err := os.WriteFile(profilePath, []byte(newContent), 0644); err != nil {
		return nil, fmt.Errorf("write %s: %w", profilePath, err)
	}

	return nil, nil
}

// RemoveEnvBlock removes the aimux-managed COPILOT_PROVIDER_* block from the
// user's shell profile. Called when clearing copilot's config.
func RemoveShellEnvBlock() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	profilePath, _, _, err := shellProfileCmds()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", profilePath, err)
	}

	existing := string(data)
	startIdx := strings.Index(existing, shellBlockStart)
	endIdx := strings.Index(existing, shellBlockEnd)

	if startIdx < 0 || endIdx < 0 {
		return nil // no block to remove
	}

	endIdx += len(shellBlockEnd)
	newContent := existing[:startIdx] + existing[endIdx:]

	// Clean up leading/trailing blank lines
	newContent = strings.TrimLeft(newContent, "\n\r")
	newContent = strings.TrimRight(newContent, "\n\r")
	if newContent != "" {
		newContent += "\n"
	}

	return os.WriteFile(profilePath, []byte(newContent), 0644)
}

// ShellProfilePath returns the detected shell profile path.
func ShellProfilePath() string {
	path, _, _, err := shellProfileCmds()
	if err != nil {
		return ""
	}
	return path
}
