# Switch — Atomic Profile Switching Spec

## Scope

The core operation of aimux: applying a saved profile (provider + model mappings) to a target CLI's JSON config file atomically. This is the "conmutator" — the deterministic operation that changes what models a local AI tool uses, right now, with zero latency.

## Inputs

- `targetCLIID` — which CLI's config to mutate (from `target_clis` table)
- `providerID` — which provider to use (from `active_multiplex`)
- (implicit) `modelMappings` — already stored in `active_multiplex.model_mappings` for the given targetCLIID

## Path Resolution

- The config file path is resolved by reading `target_clis.config_path` for the given CLI.
- The path MAY contain a `~` prefix, which MUST be expanded to `os.UserHomeDir()`.
- The resolved path MUST be an absolute, cleaned path (`filepath.Clean`).

## Read Phase

### File Locking

- Before opening the config file, acquire an exclusive file lock (`syscall.LOCK_EX`) on the file descriptor.
- If the lock cannot be acquired within 2 seconds, the operation MUST fail with a timeout error.
- The lock MUST be held for the entire read-modify-write cycle.

### JSON Parsing

- Read the entire file into memory.
- Parse as `map[string]any` (the root object is a JSON object with arbitrary keys).
- If the file is empty or contains invalid JSON, treat it as an empty object (`map[string]any{}`).
- If the file does not exist, create an empty object (`map[string]any{}`) — do NOT return an error.

## Mutation Phase

### Remove API Key (Security Invariant)

- Delete the key `"ANTHROPIC_API_KEY"` from the root object if present.
- This MUST happen regardless of whether the profile has an API key set. This is a security measure to prevent the old API key from leaking into the new config when switching providers.
- Rationale: The provider's API key is injected via the `"env"` block (see below), and the root-level `ANTHROPIC_API_KEY` would override it in Claude Code. Cleaning the root key ensures the multiplexed value is the only one the tool sees.

### Inject / Overwrite "env" Block

- Set the key `"env"` on the root object to a new map with the following structure:
  ```json
  {
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "model-id-or-empty",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "model-id-or-empty",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "model-id-or-empty",
    "CLAUDE_CODE_SUBAGENT_MODEL": "model-id-or-empty"
  }
  ```
- Variables with an empty string value in `modelMappings` MUST be excluded from the `"env"` block entirely.
- If all variables have empty values, the `"env"` block MUST still be set (as an empty object `{}`).

### Inject API Key

- Add `"ANTHROPIC_API_KEY"` inside the `"env"` block with the value of the provider's `api_key` from the `providers` table.
- This MUST override any `ANTHROPIC_API_KEY` that previously existed in the `"env"` block.

### Preserve Global Keys

- All root-level keys EXCEPT `"ANTHROPIC_API_KEY"` MUST be preserved verbatim.
- The `"env"` key is overwritten (not merged with existing env keys).
- No other keys are modified, reordered, or removed.

## Write Phase

### Atomic Write

1. Create a temporary file in the same directory as the target config file: `{target}.tmp`
2. Write the JSON to the temp file with `json.MarshalIndent(root, "", "  ")` (2-space indent, trailing newline).
3. Sync the temp file: `file.Sync()`.
4. Close the temp file.
5. Rename temp file to target: `os.Rename(tmpPath, targetPath)` — atomic on POSIX (macOS primary target).

### Lock Release

- After the rename completes (or on any error path), release the file lock via `syscall.LOCK_UN`.
- Use a `defer` to ensure the lock is always released.

## Post-Switch

- After successful write, `ClearActiveMultiplex` is NOT called (the profile remains active — the same profile was applied, it stays active).
- The TUI MUST refresh its status table to reflect the newly applied state (though the state hasn't changed from the profile perspective — the confirmation is from the write success).

## Error Recovery

| Failure Point | Behavior |
|---------------|----------|
| Cannot acquire file lock (2s timeout) | Return error: "Could not lock {path}: another process may be using it" |
| JSON parse error in existing config | Treat file content as empty object, proceed with write |
| Temp file creation fails | Return error: "Could not create temporary file in {dir}" |
| Temp file write fails | Clean up temp file if exists, release lock, return error |
| Rename fails | Return error: "Could not write config atomically to {path}" |
| Sync fails | Return error: "Could not sync config file to disk" |

## Acceptance Scenarios

### Full Profile Switch

Given a target CLI config file at `~/.config/claude/settings.json` with existing global settings  
And an active multiplex exists for `claude-code` with all four model mappings and a provider API key  
When the switch operation executes  
Then the file is read, parsed, and an exclusive lock is acquired  
And `ANTHROPIC_API_KEY` is removed from the root (if present)  
And the `"env"` block contains all four mapped variables plus `ANTHROPIC_API_KEY`  
And all pre-existing root-level keys are preserved  
And the file is written atomically via temp file + rename  
And the lock is released

### Partial Profile Switch (Some Empty Mappings)

Given an active multiplex where `ANTHROPIC_DEFAULT_HAIKU_MODEL = ""`  
When the switch operation executes  
Then the `"env"` block does NOT contain `ANTHROPIC_DEFAULT_HAIKU_MODEL`  
But it still contains `ANTHROPIC_API_KEY` and the other three non-empty model variables

### Non-Existent Config File

Given no config file exists at the target path  
When the switch operation executes  
Then the file is treated as an empty JSON object  
And the `"env"` block is injected into the new object  
And the file is created via atomic write

### API Key Security Cleanup

Given the config file has `"ANTHROPIC_API_KEY": "old-key"` at the root level  
When the switch operation executes  
And the provider's api_key is `"new-key"`  
Then `ANTHROPIC_API_KEY` is deleted from the root  
And only the `"env"` block contains `"ANTHROPIC_API_KEY": "new-key"`  
And `"old-key"` appears nowhere in the output

### File Lock Contention

Given another process holds an exclusive lock on the config file  
When the switch operation tries to acquire the lock  
Then the operation fails with a timeout error after 2 seconds  
And the config file is not modified
