# Domain: ConfigMutator Interface

## Requirement: Pluggable Config Mutation

The application layer must be able to call a config mutator without knowing the target CLI's config format.

### Scenario: Mutator resolves by registry lookup

Given `SwitchUseCases` has a `mutators map[string]ConfigMutator`
And `target_clis.mutator` column contains `"claude-settings-json"`
When `Apply()` is called
Then the mutator for `"claude-settings-json"` is retrieved and its `Mutate()` method invoked.

### Scenario: Missing mutator

Given `target_clis.mutator` contains `"unknown-mutator"`
When `Apply()` is called
Then an error `"mutator 'unknown-mutator' not registered for CLI 'my-cli'"` is returned.

### Scenario: NULL mutator falls back to default

Given `target_clis.mutator` is NULL or empty
When `Apply()` is called
Then the `"claude-settings-json"` mutator is used (backwards compatibility).

## Requirement: ConfigMutator Interface

```go
package domain

type BackupResult struct {
    BackupPath string
}

type ConfigMutator interface {
    // Mutate writes model mappings and provider config to the target CLI's config file(s).
    // provider contains the full Provider record (base_url, api_key, auth_token).
    // mutatorConfig is the parsed JSON from target_clis.mutator_config.
    Mutate(configPath string, modelMappings map[string]string, provider Provider, mutatorConfig map[string]any) (*BackupResult, error)
}
```

### Scenario: Mutator receives provider data

Given a provider with base_url, api_key, and auth_token
When `Mutate()` is called
Then the mutator receives all provider fields to use as needed.

### Scenario: Mutator config is optional

Given `target_clis.mutator_config` is NULL
When `Mutate()` is called
Then `mutatorConfig` is an empty map — mutator uses defaults.

## Requirement: Mutator Registry

The registry is populated at startup in `main.go`. Each mutator is registered by its string name.

### Scenario: Registry populated

Given all mutator packages are imported in main.go
When the binary starts
Then all mutator names are registered and available for lookup.
