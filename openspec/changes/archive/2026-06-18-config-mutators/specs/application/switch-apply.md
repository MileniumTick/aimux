# Application: SwitchUseCases.Apply with Mutator Registry

## Requirement: Registry-Based Mutator Resolution

`SwitchUseCases.Apply()` must use the mutator registry instead of calling `config.MutateAndWrite` directly.

### Scenario: Resolves mutator by name

Given a target CLI with `mutator = "claude-settings-json"`
And the registry contains `"claude-settings-json" -> ClaudeSettingsJSONMutator`
When `Apply()` is called
Then `Mutate()` is called on the Claude mutator
And the old hardcoded `config.MutateAndWrite` is NOT called.

### Scenario: Parses mutator_config JSON

Given a target CLI with `mutator_config = '{"provider_id": "bifrost", "npm": "@ai-sdk/openai-compatible"}'`
When `Apply()` calls `Mutate()`
Then `mutatorConfig` parameter contains `{"provider_id": "bifrost", "npm": "@ai-sdk/openai-compatible"}`.

### Scenario: Handles empty mutator_config

Given a target CLI with `mutator_config = '{}'` or empty string
When `Apply()` parses the config
Then `mutatorConfig` is an empty map.

### Scenario: Handles invalid JSON in mutator_config

Given a target CLI has invalid JSON in `mutator_config`
When `Apply()` parses it
Then error `"parse mutator config for CLI 'x': ..."` is returned.

### Scenario: Returns BackupResult to TUI

Given mutator returns `BackupResult{BackupPath: "/backup/path"}`
When `Apply()` completes
Then `BackupPath` is available for display in the TUI confirmation message.

### Scenario: Falls back when mutator is empty

Given a target CLI with `mutator = ""` (empty string, legacy row)
When `Apply()` is called
Then `"claude-settings-json"` is used as default mutator.

## Requirement: SwitchUseCases Constructor

```go
func NewSwitchUseCases(
    providerRepo domain.ProviderRepository,
    cliRepo domain.TargetCLIRepository,
    multiplexRepo domain.MultiplexRepository,
    mutators map[string]domain.ConfigMutator,
) *SwitchUseCases
```

### Scenario: Mutators injected at construction

Given main.go creates the mutator map with all registered implementations
When `NewSwitchUseCases` is called
Then the mutators are available for `Apply()` to use.
