package daemon

import (
	"testing"

	"github.com/MileniumTick/aimux/internal/domain"
)

func TestResolveEnvVars_AuthToken(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key", AuthToken: "tk-token"}
	mappings := map[string]string{"ANTHROPIC_MODEL": "deepseek-v4-pro"}
	mc := map[string]any{"_env_var_names": []string{"ANTHROPIC_AUTH_TOKEN"}}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["ANTHROPIC_AUTH_TOKEN"] != "tk-token" {
		t.Errorf("expected tk-token, got %q", result.Env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestResolveEnvVars_AuthTokenFallback(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key", AuthToken: ""}
	mappings := map[string]string{"ANTHROPIC_MODEL": "deepseek-v4-pro"}
	mc := map[string]any{"_env_var_names": []string{"ANTHROPIC_AUTH_TOKEN"}}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["ANTHROPIC_AUTH_TOKEN"] != "sk-key" {
		t.Errorf("expected sk-key (fallback), got %q", result.Env["ANTHROPIC_AUTH_TOKEN"])
	}
}

func TestResolveEnvVars_APIKey(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key", AuthToken: "tk-token"}
	mappings := map[string]string{}
	mc := map[string]any{"_env_var_names": []string{"OPENAI_API_KEY"}}

	result, err := resolveEnvVars("some-cli", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["OPENAI_API_KEY"] != "sk-key" {
		t.Errorf("expected sk-key, got %q", result.Env["OPENAI_API_KEY"])
	}
}

func TestResolveEnvVars_BaseURL(t *testing.T) {
	p := domain.Provider{BaseURL: "https://api.example.com/v1"}
	mappings := map[string]string{}
	mc := map[string]any{"_env_var_names": []string{"ANTHROPIC_BASE_URL"}}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["ANTHROPIC_BASE_URL"] != "https://api.example.com/v1" {
		t.Errorf("expected base url, got %q", result.Env["ANTHROPIC_BASE_URL"])
	}
}

func TestResolveEnvVars_Model(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key"}
	mappings := map[string]string{"ANTHROPIC_MODEL": "deepseek-v4-pro"}
	mc := map[string]any{"_env_var_names": []string{"ANTHROPIC_MODEL"}}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["ANTHROPIC_MODEL"] != "deepseek-v4-pro" {
		t.Errorf("expected deepseek-v4-pro, got %q", result.Env["ANTHROPIC_MODEL"])
	}
}

func TestResolveEnvVars_ModelMappingsIncluded(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key"}
	mappings := map[string]string{"ANTHROPIC_MODEL": "deepseek-v4-pro", "ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-v4-flash"}
	mc := map[string]any{"_env_var_names": []string{}}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["ANTHROPIC_MODEL"] != "deepseek-v4-pro" {
		t.Errorf("expected deepseek-v4-pro, got %q", result.Env["ANTHROPIC_MODEL"])
	}
	if result.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"] != "deepseek-v4-flash" {
		t.Errorf("expected deepseek-v4-flash, got %q", result.Env["ANTHROPIC_DEFAULT_SONNET_MODEL"])
	}
}

func TestResolveEnvVars_ClaudeExtraEnv(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key"}
	mappings := map[string]string{}
	mc := map[string]any{
		"_env_var_names": []string{"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "CLAUDE_CODE_EFFORT_LEVEL"},
	}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.Env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"] != "1" {
		t.Errorf("expected 1, got %q", result.Env["CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC"])
	}
	if result.Env["CLAUDE_CODE_EFFORT_LEVEL"] != "max" {
		t.Errorf("expected \"max\", got %q", result.Env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}

func TestResolveEnvVars_ExtraEnvDisabled(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key"}
	mappings := map[string]string{}
	mc := map[string]any{
		"_env_var_names":       []string{"CLAUDE_CODE_EFFORT_LEVEL"},
		"extra_env_disabled": true,
	}

	result, err := resolveEnvVars("claude-code", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := result.Env["CLAUDE_CODE_EFFORT_LEVEL"]; ok {
		t.Errorf("expected extra_env to be disabled, got %q", v)
	}
}

func TestResolveEnvVars_ProviderName(t *testing.T) {
	p := domain.Provider{APIKey: "sk-key", Name: "My Test Provider"}
	mappings := map[string]string{}
	mc := map[string]any{"_env_var_names": []string{}}

	result, err := resolveEnvVars("test-cli", p, mappings, mc)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProviderName != "My Test Provider" {
		t.Errorf("expected My Test Provider, got %q", result.ProviderName)
	}
	if result.CLI != "test-cli" {
		t.Errorf("expected test-cli, got %q", result.CLI)
	}
}
