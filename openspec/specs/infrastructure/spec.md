# Infrastructure: Config Mutators

## Claude Code Settings.json Mutator

**Registered as**: `"claude-settings-json"`

### Requirement: Claude Code JSON Config Mutation

Refactor existing `MutateAndWrite` into a `ConfigMutator` implementation. Preserves all existing behavior.

#### Scenario: Writes env block with model mappings

Given model mappings `{ANTHROPIC_DEFAULT_HAIKU_MODEL: "claude-haiku-4-5", ANTHROPIC_DEFAULT_SONNET_MODEL: "claude-sonnet-4-6"}`
And a provider with api_key `"sk-ant-..."`
When `Mutate()` is called on `~/.config/claude/settings.json`
Then the file contains:
```json
{
  "env": {
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-4-5",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-6",
    "ANTHROPIC_API_KEY": "sk-ant-..."
  }
}
```

#### Scenario: Preserves existing root-level keys

Given `settings.json` contains `{"CLAUDE_CODE_EFFORT_LEVEL": "high", "API_TIMEOUT_MS": 30000}`
When `Mutate()` writes new env block
Then `CLAUDE_CODE_EFFORT_LEVEL` and `API_TIMEOUT_MS` are preserved
And the `env` block is merged in.

#### Scenario: Cleans ANTHROPIC_API_KEY from root

Given `settings.json` has `ANTHROPIC_API_KEY` at root level (legacy)
When `Mutate()` writes
Then `ANTHROPIC_API_KEY` is removed from root level (security invariant).

#### Scenario: Creates backup before mutation

Given `settings.json` exists with content
When `Mutate()` writes
Then a timestamped backup is created at `~/.config/claude/settings.json.aimux-backup-{RFC3339}`.

#### Scenario: Atomic write with flock

Given concurrent processes may access `settings.json`
When `Mutate()` writes
Then flock exclusive lock is acquired, temp file is written, and `os.Rename` is used for atomic replacement.

#### Scenario: Empty model vars not written

Given a mapping includes `{"ANTHROPIC_DEFAULT_OPUS_MODEL": ""}`
When `Mutate()` writes
Then `ANTHROPIC_DEFAULT_OPUS_MODEL` is omitted from the env block.

---

## OpenCode opencode.json Mutator

**Registered as**: `"opencode-provider-json"`

### Requirement: OpenCode Nested JSON Config Mutation

OpenCode uses `opencode.json` with a deeply nested provider structure. The mutator must build the `provider.<id>.models` sub-tree.

#### Scenario: Creates provider entry with models

Given model mappings `{DEFAULT_MODEL: "gpt-4o", FAST_MODEL: "gpt-4o-mini"}`
And a provider with name `"Bifrost"`, base_url `"https://bifrost.example.com/v1"`, api_key `"sk-bifrost-..."`
And `mutator_config` contains `{"provider_id": "bifrost", "npm": "@ai-sdk/openai-compatible"}`
When `Mutate()` is called on `~/opencode.json`
Then the file contains:
```json
{
  "$schema": "https://opencode.ai/config.json",
  "provider": {
    "bifrost": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Bifrost",
      "options": {
        "baseURL": "https://bifrost.example.com/v1",
        "apiKey": "sk-bifrost-..."
      },
      "models": {
        "gpt-4o": { "name": "GPT-4o" },
        "gpt-4o-mini": { "name": "GPT-4o Mini" }
      }
    }
  }
}
```

#### Scenario: Preserves other providers

Given `opencode.json` already has a `provider.anthropic` entry
When `Mutate()` writes the `bifrost` provider
Then `provider.anthropic` is preserved intact.

#### Scenario: Preserves top-level keys

Given `opencode.json` has `"model": "..."` and `"$schema": "..."` at root
When `Mutate()` writes
Then those keys are preserved.

#### Scenario: Model name used as display name

Given a model mapping `{DEFAULT_MODEL: "bifrost-sonnet-v2"}`
When `Mutate()` writes the model entry
Then the model key is `"bifrost-sonnet-v2"` and the display name is `"bifrost-sonnet-v2"`.

#### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then an error is returned: `"opencode mutator requires provider_id in mutator_config"`.

#### Scenario: Backup and atomic write

Given `opencode.json` exists
When `Mutate()` writes
Then a timestamped backup is created and atomic replace with flock is used.

---

## Codex config.toml Mutator

**Registered as**: `"codex-config-toml"`

### Requirement: Codex TOML Config Mutation

Codex uses TOML for `~/.codex/config.toml`. The mutator writes `[model_providers.<id>]` tables.

#### Scenario: Creates provider table in TOML

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

#### Scenario: API key written to env var

Given provider's api_key is `"sk-bifrost-..."`
When `Mutate()` writes
Then the env var `CODEX_BIFROST_API_KEY` is exported/embedded
And the API key value is NOT written to the TOML file
And `env_key` references the env var name.

#### Scenario: Preserves other TOML sections

Given `config.toml` has `[editor]` section and other config
When `Mutate()` writes
Then non-provider sections are preserved.

#### Scenario: Handles missing file

Given `~/.codex/config.toml` does not exist
When `Mutate()` is called
Then a new file is created with just the provider section.

#### Scenario: API key in separate env file

Given Copilot/Codex pattern of env-var-only API keys
When `Mutate()` writes
Then the API key is NOT embedded in TOML
And instead written to `~/.codex/.env` or shown to user.

#### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then error `"codex mutator requires provider_id in mutator_config"`.

---

## Copilot CLI Env-Var Mutator

**Registered as**: `"copilot-env-file"`

### Requirement: Copilot Environment Variable Injection

Copilot CLI has NO config file. All config is environment variables. The mutator writes to shell profile files.

#### Scenario: Writes env vars to .env file

Given model mappings `{COPILOT_MODEL: "bifrost-sonnet"}`
And a provider with base_url `"https://bifrost.example.com/v1"`, api_key `"sk-..."`
And `mutator_config` contains `{"provider_type": "openai"}`
When `Mutate()` is called
Then a `.env` file is created/updated at `~/.config/copilot/.env` with:
```
COPILOT_PROVIDER_BASE_URL=https://bifrost.example.com/v1
COPILOT_PROVIDER_TYPE=openai
COPILOT_PROVIDER_API_KEY=sk-...
COPILOT_MODEL=bifrost-sonnet
```

#### Scenario: API key omitted for local providers

Given `mutator_config` contains `{"provider_type": "openai", "local": true}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_API_KEY` is NOT written (local providers don't need auth).

#### Scenario: Anthropic provider type

Given `mutator_config` contains `{"provider_type": "anthropic"}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_TYPE=anthropic` is written.

#### Scenario: Azure provider type

Given `mutator_config` contains `{"provider_type": "azure"}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_TYPE=azure` is written
And base_url uses the full deployment URL.

#### Scenario: Backup of existing .env

Given `~/.config/copilot/.env` already exists
When `Mutate()` writes
Then a timestamped backup is created before overwriting.

#### Scenario: Directory created if missing

Given `~/.config/copilot/` does not exist
When `Mutate()` writes
Then the directory is created with 0700 permissions.

---

## Pi models.json + auth.json Mutator

**Registered as**: `"pi-dual-json"`

### Requirement: Pi Dual-JSON Config Mutation

Pi uses two files: `models.json` (provider definition + models) and `auth.json` (credentials).

#### Scenario: Writes provider entry to models.json

Given model mappings `{DEFAULT_MODEL: "bifrost-sonnet", FAST_MODEL: "bifrost-haiku"}`
And a provider with name `"Bifrost"`, base_url `"https://bifrost.example.com/v1"`, api_key `"sk-..."`
And `mutator_config` contains `{"provider_id": "bifrost", "provider_type": "openai-compatible"}`
When `Mutate()` is called on `models.json`
Then `models.json` contains:
```json
{
  "providers": {
    "bifrost": {
      "type": "openai-compatible",
      "base_url": "https://bifrost.example.com/v1",
      "models": {
        "bifrost-sonnet": { "name": "bifrost-sonnet" },
        "bifrost-haiku": { "name": "bifrost-haiku" }
      }
    }
  }
}
```

#### Scenario: Writes API key to auth.json

Given provider's api_key is `"sk-bifrost-..."`
When `Mutate()` writes
Then `auth.json` contains:
```json
{
  "bifrost": {
    "type": "api_key",
    "key": "sk-bifrost-..."
  }
}
```

#### Scenario: Preserves other providers in both files

Given `models.json` already has `openai` and `anthropic` providers
And `auth.json` already has credentials for those
When `Mutate()` writes the `bifrost` provider
Then existing providers and credentials are preserved in both files.

#### Scenario: Both files backed up

Given both `models.json` and `auth.json` exist
When `Mutate()` writes
Then timestamped backups are created for both files.

#### Scenario: config_path is directory

Given `mutator_config` contains `{"models_path": "~/.config/pi/models.json", "auth_path": "~/.config/pi/auth.json"}`
When `Mutate()` is called
Then both specific paths are used.

#### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then error `"pi mutator requires provider_id in mutator_config"`.
