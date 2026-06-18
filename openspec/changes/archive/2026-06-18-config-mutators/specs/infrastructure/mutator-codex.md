# Infrastructure: Codex config.toml Mutator

## Requirement: Codex TOML Config Mutation

Codex uses TOML for `~/.codex/config.toml`. The mutator writes `[model_providers.<id>]` tables.

### Scenario: Creates provider table in TOML

Given model mappings `{CODEX_MODEL: "gpt-5.4"}` (single model under first env var key)
And a provider with name `"Bifrost"`, base_url `"https://bifrost.example.com"`, api_key `"sk-..."`
And `mutator_config` contains `{"provider_id": "bifrost", "wire_api": "responses"}`
When `Mutate()` is called on `~/.codex/config.toml`
Then the file contains:
```toml
model = "gpt-5.4"
model_provider = "bifrost"

[model_providers.bifrost]
name = "Bifrost"
base_url = "https://bifrost.example.com"
env_key = "CODEX_BIFROST_API_KEY"
wire_api = "responses"
```

### Scenario: API key written to env var

Given provider's api_key is `"sk-bifrost-..."`
When `Mutate()` writes
Then the env var `CODEX_BIFROST_API_KEY` is exported/embedded
And the API key value is NOT written to the TOML file
And `env_key` references the env var name.

### Scenario: Preserves other TOML sections

Given `config.toml` has `[editor]` section and other config
When `Mutate()` writes
Then non-provider sections are preserved.

### Scenario: Handles missing file

Given `~/.codex/config.toml` does not exist
When `Mutate()` is called
Then a new file is created with just the provider section.

### Scenario: API key in separate env file

Given Copilot/Codex pattern of env-var-only API keys
When `Mutate()` writes
Then the API key is NOT embedded in TOML
And instead written to `~/.codex/.env` or shown to user.

### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then error `"codex mutator requires provider_id in mutator_config"`.

## Registered as: "codex-config-toml"
