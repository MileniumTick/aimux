package mutators

import (
	"net/url"
	"os"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// defaultClaudeExtraEnv provides recommended default env vars for Claude Code.
// Users can override these via mutator_config.extra_env.
// ponytail: sensible defaults; users who know better can set extra_env.
var defaultClaudeExtraEnv = map[string]string{
	"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
	"CLAUDE_CODE_EFFORT_LEVEL":                 "max",
}

// ClaudeSettingsJSON mutates Claude Code's settings.json by building an env
// block from model mappings, provider API key, and optional extra env vars.
// Registered as: "claude-settings-json"
type ClaudeSettingsJSON struct{}

// Mutate reads the JSON config, builds an env block from model mappings, sets
// the API key, writes extra env vars (CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC,
// CLAUDE_CODE_EFFORT_LEVEL), appends context window suffix to model IDs based
// on model metadata, and writes atomically with backup.
func (m *ClaudeSettingsJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	if err := config.PrepareDir(configPath); err != nil {
		return nil, err
	}

	// Create backup BEFORE reading — file existence on disk, not parse success
	var backupResult *domain.BackupResult
	if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(configPath)
		if err != nil {
			return nil, err
		}
		backupResult = &domain.BackupResult{BackupPath: bp}
	}

	root, err := config.ReadJSONWithLock(configPath)
	if err != nil {
		return nil, err
	}

	// Delete ANTHROPIC_API_KEY from root (security invariant)
	delete(root, "ANTHROPIC_API_KEY")

	// Merge into existing env instead of replacing — Claude Code stores
	// ANTHROPIC_AUTH_TOKEN (OAuth) here and replacing wipes it.
	env := make(map[string]any)
	if existing, ok := root["env"].(map[string]any); ok {
		for k, v := range existing {
			env[k] = v
		}
	}

	// Write model mappings with context window suffix (skip empty values).
	// Claude Code uses "[1m]" for 1M context, "[200k]" for 200K, etc.
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	for key, val := range modelMappings {
		if val != "" {
			if md, ok := modelMeta[val].(map[string]any); ok {
				suffix := config.LookupContextSuffix(md)
				if suffix != "" {
					val = val + suffix
				}
			}
			env[key] = val
		}
	}

	// Always use ANTHROPIC_AUTH_TOKEN — ANTHROPIC_API_KEY causes login prompts
	// in Claude Code instead of using the key directly.
	token := provider.AuthToken
	if token == "" {
		token = provider.APIKey
	}
	if token != "" {
		env["ANTHROPIC_AUTH_TOKEN"] = token
		delete(env, "ANTHROPIC_API_KEY")
	}

	if provider.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = ensureClaudeBaseURL(provider.BaseURL)
	}

	// Extra Claude Code env vars (CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC, etc.)
	applyClaudeExtraEnv(env, mutatorConfig)

	root["env"] = env

	if err := config.WriteAtomicJSON(configPath, root); err != nil {
		return nil, err
	}

	// Clean up old backups
	config.PruneBackups(configPath, 5)

	return backupResult, nil
}

// ensureClaudeBaseURL ensures the URL path is always /anthropic for Claude Code.
// Claude Code requires the API path to be /anthropic regardless of provider API type.
// ponytail: net/url handles edge cases; no need for custom string manipulation.
func ensureClaudeBaseURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	// Normalize: strip existing path and set to /anthropic
	u.Path = "/anthropic"
	u.RawPath = "" // let Go reconstruct
	return u.String()
}

// applyClaudeExtraEnv writes default and user-override extra env vars into the env map.
// Defaults: CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1, CLAUDE_CODE_EFFORT_LEVEL=max.
// User can override via mutator_config.extra_env, or disable all extras via
// mutator_config.extra_env_disabled=true.
// ponytail: defaults are sensible; override via extra_env when you know better.
func applyClaudeExtraEnv(env map[string]any, mutatorConfig map[string]any) {
	if disabled, ok := mutatorConfig["extra_env_disabled"].(bool); ok && disabled {
		return
	}

	// Apply defaults (won't overwrite existing keys from merge above)
	for k, v := range defaultClaudeExtraEnv {
		if _, exists := env[k]; !exists {
			env[k] = v
		}
	}

	// Apply user overrides from mutator_config.extra_env
	if extra, ok := mutatorConfig["extra_env"].(map[string]any); ok {
		for k, v := range extra {
			env[k] = v
		}
	}
}
