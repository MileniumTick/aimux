# Storage: Schema Migration for Config Mutators

## Requirement: New Columns on target_clis

The `target_clis` table must support mutator selection and configuration.

### Scenario: Schema migration adds columns

Given the existing `target_clis` table
When the migration runs
Then columns `mutator TEXT NOT NULL DEFAULT 'claude-settings-json'` and `mutator_config TEXT NOT NULL DEFAULT '{}'` are added.

### Migration DDL

```sql
-- Migration: add mutator support to target_clis
-- Safe: uses IF NOT EXISTS pattern via ALTER TABLE ADD COLUMN with defaults

-- Step 1: Add mutator column (backwards compatible: defaults to claude-settings-json)
ALTER TABLE target_clis ADD COLUMN mutator TEXT NOT NULL DEFAULT 'claude-settings-json';

-- Step 2: Add mutator_config column (JSON, each mutator interprets its own config)
ALTER TABLE target_clis ADD COLUMN mutator_config TEXT NOT NULL DEFAULT '{}';
```

### Scenario: Existing Claude Code row preserved

Given the `claude-code` row was seeded in a previous migration
When the new migration runs
Then the `claude-code` row now has `mutator = 'claude-settings-json'` and `mutator_config = '{}'`.

### Scenario: Seed includes new columns

Given `SeedTargetCLIs` runs
When the claude-code row is inserted
Then it includes `mutator` and `mutator_config` values.

## Requirement: Updated TargetCLI Domain Struct

```go
type TargetCLI struct {
    ID            int64
    Name          string
    ConfigPath    string
    EnvVars       string  // JSON array of env var names
    Mutator       string  // mutator registry key
    MutatorConfig string  // JSON object for mutator-specific config
}
```

### Scenario: Repository returns new fields

Given a `TargetCLIRepository.List()` call
When rows are scanned
Then `Mutator` and `MutatorConfig` fields are populated from the database.
