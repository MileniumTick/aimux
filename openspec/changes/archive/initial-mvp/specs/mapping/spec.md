# Mapping — Variable-to-Model Binding Spec

## Scope

The logic that binds raw provider model IDs to specific environment variables that a target CLI reads. For MVP, this is hardcoded to Claude Code's four env vars. Profiles are the atomic unit: one provider, one model ID per variable, saved as a single `active_multiplex` row.

## Definitions

- **Profile**: A single binding of one provider to four Claude Code env vars via model IDs.
- **Model Mapping**: A JSON object of the form `{"ENV_VAR_NAME": "model-id", ...}`.
- **Target CLI**: A registered CLI (MVP: only `claude-code`) with its known env vars stored in `target_clis.env_vars`.

## Claude Code Env Vars (MVP, hardcoded)

| Variable | Purpose | Example Value |
|----------|---------|---------------|
| `ANTHROPIC_DEFAULT_HAIKU_MODEL` | Default model for quick/cheap requests | `claude-haiku-3-20250313` |
| `ANTHROPIC_DEFAULT_SONNET_MODEL` | Default model for balanced requests | `claude-sonnet-4-20250514` |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` | Default model for complex reasoning | `claude-opus-4-20250514` |
| `CLAUDE_CODE_SUBAGENT_MODEL` | Model used by Claude Code for sub-agent delegated tasks | `claude-sonnet-4-20250514` |

## Profile Semantics

### All-or-Nothing Mapping

- A profile maps one provider to ALL four Claude Code env vars simultaneously.
- The user picks one model ID per variable from that provider's available models.
- Each variable SHALL have a "Not Selected" option, resulting in an empty string value. The empty string is stored but means: the variable is omitted from the injected `"env"` block (the target CLI uses its default).

### Uniqueness

- There is exactly ONE active multiplex row per target CLI. Switching profiles replaces the entire mapping for that CLI — no partial updates.

### Data Storage

- The model mappings are stored as a JSON string in `active_multiplex.model_mappings`:
  ```json
  {
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-3-20250313",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-20250514",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "",
    "CLAUDE_CODE_SUBAGENT_MODEL": "claude-sonnet-4-20250514"
  }
  ```
- Empty string values (`""`) MUST be stored but MUST be excluded when injecting into `settings.json` (the variable is simply not written).

## Binding Service

### `BindProfile(targetCLIID, providerID, mappings) -> error`

- Accepts `targetCLIID`, `providerID`, and a `map[string]string` of env var to model ID.
- Validates that all keys in `mappings` are a subset of the target CLI's known `env_vars` (from `target_clis` table).
- Validates that the provider has `status = 'active'` and that each model ID value that is non-empty exists in `provider_models` for that provider.
- Calls `SetActiveMultiplex()` with the validated data.

### `GetBoundModels(targetCLIID) -> (map[string]string, error)`

- Returns the current model mappings for the given CLI, or empty map if none set.

### `GetProviderForCLI(targetCLIID) -> (providerID, error)`

- Returns the provider currently bound to the given CLI (from `active_multiplex`), or error if none.

## Acceptance Scenarios

### Full Profile Binding

Given a provider with models `["claude-haiku-3", "claude-sonnet-4", "claude-opus-4"]`  
When `BindProfile` is called with targetCLIID=1, providerID=1, and mappings:
- ANTHROPIC_DEFAULT_HAIKU_MODEL = "claude-haiku-3"
- ANTHROPIC_DEFAULT_SONNET_MODEL = "claude-sonnet-4"
- ANTHROPIC_DEFAULT_OPUS_MODEL = "claude-opus-4"
- CLAUDE_CODE_SUBAGENT_MODEL = "claude-sonnet-4"

Then `SetActiveMultiplex` is called with modelMappings containing all four entries  
And `GetBoundModels` returns the same four entries

### Partial Profile Binding

Given the same provider and models  
When `BindProfile` is called with ANTHROPIC_DEFAULT_OPUS_MODEL = "" (not selected)  
Then the stored modelMappings JSON includes ANTHROPIC_DEFAULT_OPUS_MODEL = ""  
And when the profile is later injected into settings.json, ANTHROPIC_DEFAULT_OPUS_MODEL MUST be omitted from the env block

### Unknown Model ID

Given a provider with model `"claude-sonnet-4"`  
When `BindProfile` is called with ANTHROPIC_DEFAULT_HAIKU_MODEL = "non-existent-model"  
Then the call MUST return an error: "Model 'non-existent-model' not found for this provider"  
And the active multiplex row is NOT created or modified

### Unknown Env Var

When `BindProfile` is called with a key "UNKNOWN_VAR" that is not in the target CLI's env_vars  
Then the call MUST return an error  
And the active multiplex row is NOT created or modified

### Get Binding — No Active Profile

Given no active multiplex exists for targetCLIID=1  
When `GetBoundModels` is called  
Then it returns an empty map and no error
