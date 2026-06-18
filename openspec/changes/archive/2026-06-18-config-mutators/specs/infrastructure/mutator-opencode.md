# Infrastructure: OpenCode opencode.json Mutator

## Requirement: OpenCode Nested JSON Config Mutation

OpenCode uses `opencode.json` with a deeply nested provider structure. The mutator must build the `provider.<id>.models` sub-tree.

### Scenario: Creates provider entry with models

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

### Scenario: Preserves other providers

Given `opencode.json` already has a `provider.anthropic` entry
When `Mutate()` writes the `bifrost` provider
Then `provider.anthropic` is preserved intact.

### Scenario: Preserves top-level keys

Given `opencode.json` has `"model": "..."` and `"$schema": "..."` at root
When `Mutate()` writes
Then those keys are preserved.

### Scenario: Model name used as display name

Given a model mapping `{DEFAULT_MODEL: "bifrost-sonnet-v2"}`
When `Mutate()` writes the model entry
Then the model key is `"bifrost-sonnet-v2"` and the display name is `"bifrost-sonnet-v2"`.

### Scenario: mutator_config required

Given no `provider_id` in `mutator_config`
When `Mutate()` is called
Then an error is returned: `"opencode mutator requires provider_id in mutator_config"`.

### Scenario: Backup and atomic write

Given `opencode.json` exists
When `Mutate()` writes
Then a timestamped backup is created and atomic replace with flock is used.

## Registered as: "opencode-provider-json"
