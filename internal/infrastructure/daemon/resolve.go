package daemon

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// ResolveResult holds the resolved env vars for a CLI binding.
type ResolveResult struct {
	CLI          string            `json:"cli"`
	ProviderName string            `json:"provider_name,omitempty"`
	Env          map[string]string `json:"env"`
}

// resolveEnvVars resolves the known env vars for a CLI from its provider binding.
// Model metadata from mutatorCfg["_model_metadata"] is used to append context
// window suffixes (e.g., "deepseek-v4-flash" → "deepseek-v4-flash[1m]").
func resolveEnvVars(cliName string, provider domain.Provider, modelMappings map[string]string, mutatorCfg map[string]any) (*ResolveResult, error) {
	env := make(map[string]string)
	modelMeta, _ := mutatorCfg["_model_metadata"].(map[string]any)

	// ── Special env vars (extras) ──────────────────────────────────────
	specials := map[string]string{}
	if extra, ok := mutatorCfg["extra_env"].(map[string]any); ok {
		for k, v := range extra {
			specials[k] = fmt.Sprintf("%v", v)
		}
	}
	if disabled, _ := mutatorCfg["extra_env_disabled"].(bool); !disabled {
		if _, ok := specials["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"]; !ok {
			specials["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] = "1"
		}
		if _, ok := specials["CLAUDE_CODE_EFFORT_LEVEL"]; !ok {
			specials["CLAUDE_CODE_EFFORT_LEVEL"] = "max"
		}
	}
	for k, v := range specials {
		env[k] = v
	}

	// ── Declared env vars (from CLI's env_vars config) ─────────────────
	var envVarNames []string
	if names, ok := mutatorCfg["_env_var_names"].([]string); ok {
		envVarNames = names
	}
	for _, name := range envVarNames {
		if _, exists := env[name]; exists {
			continue
		}
		if val := resolveByName(name, provider, modelMappings, modelMeta); val != "" {
			env[name] = val
		}
	}

	// ── Standard credential env vars ───────────────────────────────────
	injectCredentialEnvVars(env, provider, mutatorCfg)

	// ── Model mappings as direct env vars ──────────────────────────────
	for k, v := range modelMappings {
		if v != "" && !strings.HasPrefix(k, "_") {
			if _, exists := env[k]; !exists {
				env[k] = v
			}
		}
	}

	return &ResolveResult{
		CLI:          cliName,
		ProviderName: provider.Name,
		Env:          env,
	}, nil
}

// injectCredentialEnvVars adds standard credential env vars per mutator type.
func injectCredentialEnvVars(env map[string]string, p domain.Provider, mutatorCfg map[string]any) {
	mutatorName, _ := mutatorCfg["_mutator_name"].(string)

	token := p.AuthToken
	if token == "" {
		token = p.APIKey
	}

	switch mutatorName {
	case "claude-settings-json":
		if token != "" {
			setIfMissing(env, "ANTHROPIC_AUTH_TOKEN", token)
			delete(env, "ANTHROPIC_API_KEY")
		}
		if p.BaseURL != "" {
			normalized := ensureClaudeBaseURL(p.BaseURL)
			setIfMissing(env, "ANTHROPIC_BASE_URL", normalized)
		}

	case "opencode-provider-json":
		if token != "" {
			setIfMissing(env, "OPENAI_API_KEY", token)
		}
		if p.APIKey != "" {
			setIfMissing(env, "ANTHROPIC_API_KEY", p.APIKey)
		}
		if p.BaseURL != "" {
			setIfMissing(env, "OPENAI_BASE_URL", p.BaseURL)
		}

	case "codex-config-toml":
		if token != "" {
			setIfMissing(env, "ANTHROPIC_AUTH_TOKEN", token)
		}

	case "copilot-shell-profile":

	case "pi-dual-json":
		if token != "" {
			setIfMissing(env, "ANTHROPIC_AUTH_TOKEN", token)
		}

	default:
		if token != "" {
			setIfMissing(env, "ANTHROPIC_AUTH_TOKEN", token)
		}
		if p.APIKey != "" {
			setIfMissing(env, "OPENAI_API_KEY", p.APIKey)
		}
		if p.BaseURL != "" {
			setIfMissing(env, "OPENAI_BASE_URL", p.BaseURL)
		}
	}
}

func setIfMissing(env map[string]string, key, val string) {
	if _, exists := env[key]; !exists && val != "" {
		env[key] = val
	}
}

// ensureClaudeBaseURL ensures the URL path is always /anthropic for Claude Code.
func ensureClaudeBaseURL(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.Path = "/anthropic"
	u.RawPath = ""
	return u.String()
}

// resolveByName resolves a single env var name based on its suffix pattern.
func resolveByName(name string, p domain.Provider, mappings map[string]string, modelMeta map[string]any) string {
	upper := strings.ToUpper(name)

	switch {
	case strings.HasSuffix(upper, "_AUTH_TOKEN"):
		if p.AuthToken != "" {
			return p.AuthToken
		}
		return p.APIKey

	case strings.HasSuffix(upper, "_API_KEY"):
		return p.APIKey

	case strings.Contains(upper, "BASE_URL"):
		return p.BaseURL

	case strings.HasSuffix(upper, "_URL"):
		if p.BaseURL != "" {
			return p.BaseURL
		}
		return p.DiscoveryURL

	case strings.HasSuffix(upper, "_DISCOVERY_URL"):
		if p.DiscoveryURL != "" {
			return p.DiscoveryURL
		}
		return p.BaseURL

	case strings.HasSuffix(upper, "_MODEL"):
		for _, v := range mappings {
			if v != "" {
				return v
			}
		}
	}

	return ""
}


