# Infrastructure: Copilot CLI Env-Var Mutator

## Requirement: Copilot Environment Variable Injection

Copilot CLI has NO config file. All config is environment variables. The mutator writes to shell profile files.

### Scenario: Writes env vars to .env file

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

### Scenario: API key omitted for local providers

Given `mutator_config` contains `{"provider_type": "openai", "local": true}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_API_KEY` is NOT written (local providers don't need auth).

### Scenario: Anthropic provider type

Given `mutator_config` contains `{"provider_type": "anthropic"}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_TYPE=anthropic` is written.

### Scenario: Azure provider type

Given `mutator_config` contains `{"provider_type": "azure"}`
When `Mutate()` writes
Then `COPILOT_PROVIDER_TYPE=azure` is written
And base_url uses the full deployment URL.

### Scenario: Backup of existing .env

Given `~/.config/copilot/.env` already exists
When `Mutate()` writes
Then a timestamped backup is created before overwriting.

### Scenario: Directory created if missing

Given `~/.config/copilot/` does not exist
When `Mutate()` writes
Then the directory is created with 0700 permissions.

## Registered as: "copilot-env-file"
