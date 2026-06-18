# Infrastructure: Claude Code Settings.json Mutator

## Requirement: Claude Code JSON Config Mutation

Refactor existing `MutateAndWrite` into a `ConfigMutator` implementation. Preserves all existing behavior.

### Scenario: Writes env block with model mappings

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

### Scenario: Preserves existing root-level keys

Given `settings.json` contains `{"CLAUDE_CODE_EFFORT_LEVEL": "high", "API_TIMEOUT_MS": 30000}`
When `Mutate()` writes new env block
Then `CLAUDE_CODE_EFFORT_LEVEL` and `API_TIMEOUT_MS` are preserved
And the `env` block is merged in.

### Scenario: Cleans ANTHROPIC_API_KEY from root

Given `settings.json` has `ANTHROPIC_API_KEY` at root level (legacy)
When `Mutate()` writes
Then `ANTHROPIC_API_KEY` is removed from root level (security invariant).

### Scenario: Creates backup before mutation

Given `settings.json` exists with content
When `Mutate()` writes
Then a timestamped backup is created at `~/.config/claude/settings.json.aimux-backup-{RFC3339}`.

### Scenario: Atomic write with flock

Given concurrent processes may access `settings.json`
When `Mutate()` writes
Then flock exclusive lock is acquired, temp file is written, and `os.Rename` is used for atomic replacement.

### Scenario: Empty model vars not written

Given a mapping includes `{"ANTHROPIC_DEFAULT_OPUS_MODEL": ""}`
When `Mutate()` writes
Then `ANTHROPIC_DEFAULT_OPUS_MODEL` is omitted from the env block.

## Registered as: "claude-settings-json"
