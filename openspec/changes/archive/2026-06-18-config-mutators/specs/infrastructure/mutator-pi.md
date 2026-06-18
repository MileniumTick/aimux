# Infrastructure: Pi models.json + auth.json Mutator

## Requirement: Pi Dual-JSON Config Mutation

Pi uses two files: `models.json` (provider definition + models) and `auth.json` (credentials).

### Scenario: Writes provider entry to models.json

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

### Scenario: Writes API key to auth.json

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

### Scenario: Preserves other providers in both files

Given `models.json` already has `openai` and `anthropic` providers
And `auth.json` already has credentials for those
When `Mutate()` writes the `bifrost` provider
Then existing providers and credentials are preserved in both files.

### Scenario: Both files backed up

Given both `models.json` and `auth.json` exist
When `Mutate()` writes
Then timestamped backups are created for both files.

### Scenario: config_path is directory

Given `mutator_config` contains `{"models_path": "~/.config/pi/models.json", "auth_path": "~/.config/pi/auth.json"}`
When `Mutate()` is called
Then both specific paths are used.

### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then error `"pi mutator requires provider_id in mutator_config"`.

## Registered as: "pi-dual-json"
