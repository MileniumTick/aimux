# Database Schema & Migrations

> SQLite schema reference, migration history, seed data, and query patterns.

---

## Current Schema (v8)

After all migrations applied. All tables use `IF NOT EXISTS`; migrations are idempotent.

### providers

```sql
CREATE TABLE IF NOT EXISTS providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    discovery_url TEXT NOT NULL DEFAULT '',
    default_context_window INTEGER NOT NULL DEFAULT 0,
    api_key TEXT NOT NULL DEFAULT '',
    auth_token TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active','error')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

| Column | Type | Constraint | Description |
|--------|------|-----------|-------------|
| `id` | INTEGER | PK, AUTOINCREMENT | Provider ID |
| `name` | TEXT | NOT NULL, UNIQUE | Human-readable provider name |
| `base_url` | TEXT | NOT NULL | API base URL with scheme |
| `discovery_url` | TEXT | DEFAULT '' | Separate URL for model discovery; empty = use base_url |
| `default_context_window` | INTEGER | DEFAULT 0 | Fallback context window for models without catalog entry |
| `api_key` | TEXT | DEFAULT '' | API key (plaintext) |
| `auth_token` | TEXT | DEFAULT '' | Auth token, may differ from api_key |
| `status` | TEXT | CHECK('active','error') | Provider health: active after successful model fetch, error otherwise |
| `created_at` | TEXT | DEFAULT datetime('now') | ISO 8601 creation timestamp |
| `updated_at` | TEXT | DEFAULT datetime('now') | ISO 8601 last update timestamp |

### provider_models

```sql
CREATE TABLE IF NOT EXISTS provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_name TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}',
    UNIQUE(provider_id, model_name)
);
```

| Column | Type | Constraint | Description |
|--------|------|-----------|-------------|
| `id` | INTEGER | PK, AUTOINCREMENT | Model row ID |
| `provider_id` | INTEGER | FK → providers(id) CASCADE | Owning provider |
| `model_name` | TEXT | UNIQUE(provider_id, model_name) | Model identifier from API |
| `metadata` | TEXT | DEFAULT '{}' | JSON object with model capabilities (context_window, max_tokens, reasoning, cost, etc.) |

**Indexes**:

```sql
CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON provider_models(provider_id);
```

### target_clis

```sql
CREATE TABLE IF NOT EXISTS target_clis (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    config_path TEXT NOT NULL,
    env_vars TEXT NOT NULL,
    mutator TEXT NOT NULL DEFAULT 'claude-settings-json',
    mutator_config TEXT NOT NULL DEFAULT '{}'
);
```

| Column | Type | Constraint | Description |
|--------|------|-----------|-------------|
| `id` | INTEGER | PK, AUTOINCREMENT | CLI row ID |
| `name` | TEXT | NOT NULL, UNIQUE | CLI identifier (e.g. "claude-code") |
| `config_path` | TEXT | NOT NULL | Path to config file (tilde-expanded at runtime) |
| `env_vars` | TEXT | NOT NULL | JSON array of known env var names |
| `mutator` | TEXT | DEFAULT 'claude-settings-json' | Mutator registry key |
| `mutator_config` | TEXT | DEFAULT '{}' | JSON object with mutator-specific configuration |

### active_multiplex

```sql
CREATE TABLE IF NOT EXISTS active_multiplex (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_cli_id INTEGER NOT NULL REFERENCES target_clis(id) ON DELETE CASCADE,
    provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    model_mappings TEXT NOT NULL,
    activated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(target_cli_id, provider_id)
);
```

| Column | Type | Constraint | Description |
|--------|------|-----------|-------------|
| `id` | INTEGER | PK, AUTOINCREMENT | Multiplex row ID |
| `target_cli_id` | INTEGER | FK → target_clis(id) CASCADE | Target CLI |
| `provider_id` | INTEGER | FK → providers(id) CASCADE | Bound provider |
| `model_mappings` | TEXT | NOT NULL | JSON object mapping env vars to model names |
| `activated_at` | TEXT | DEFAULT datetime('now') | ISO 8601 activation timestamp |
| UNIQUE | | (target_cli_id, provider_id) | One binding per CLI-provider pair |

### update_cache

```sql
CREATE TABLE IF NOT EXISTS update_cache (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    checked_at TEXT NOT NULL
);
```

| Column | Type | Constraint | Description |
|--------|------|-----------|-------------|
| `key` | TEXT | PK | Cache key (e.g. "latest_version") |
| `value` | TEXT | NOT NULL | Cached value |
| `checked_at` | TEXT | NOT NULL | ISO 8601 timestamp, used for 24h TTL |

This table is created by `CacheSet()` on first write (not in migrations — created lazily).

**Query**: `SELECT value FROM update_cache WHERE key = ? AND checked_at > datetime('now', '-24 hours')`

---

## Seed Data

`SeedTargetCLIs()` inserts 5 default CLI rows via `INSERT OR IGNORE` (idempotent):

| name | config_path | env_vars | mutator | mutator_config |
|------|------------|----------|---------|---------------|
| `claude-code` | `~/.config/claude/settings.json` | `["ANTHROPIC_DEFAULT_HAIKU_MODEL","ANTHROPIC_DEFAULT_SONNET_MODEL","ANTHROPIC_DEFAULT_OPUS_MODEL","CLAUDE_CODE_SUBAGENT_MODEL"]` | `claude-settings-json` | `{}` |
| `opencode` | `~/.config/opencode/config.json` | `["OPENCODE_MODEL","OPENCODE_FAST_MODEL"]` | `opencode-provider-json` | `{"provider_id":"custom"}` |
| `codex` | `~/.codex/config.toml` | `["CODEX_MODEL"]` | `codex-config-toml` | `{"provider_id":"custom"}` |
| `github-copilot` | `""` (auto-detect shell profile) | `["COPILOT_MODEL"]` | `copilot-shell-profile` | `{}` |
| `pi-ai` | `~/.pi/agent/models.json` | `["PI_DEFAULT_MODEL","PI_FAST_MODEL"]` | `pi-dual-json` | `{"provider_id":"custom"}` |

---

## Migration History

Migrations are applied sequentially in `main.go`. Each is a Go function receiving `*sql.DB`. All are idempotent (check before alter).

| Order | Function | Description | Idempotency Check |
|-------|----------|-------------|-------------------|
| 1 | `RunMigrations` | Create 4 initial tables (providers, provider_models, target_clis, active_multiplex) | `IF NOT EXISTS` |
| 2 | `MigrationAddMutatorColumns` | Add `mutator`, `mutator_config` to target_clis | `columnExists(mutator)` |
| 3 | `MigrationAddDiscoveryURLColumn` | Add `discovery_url` to providers | `columnExists(discovery_url)` |
| 4 | `MigrationDropApiTypeColumn` | **No-op** — api_type was removed from code but column stays for backwards compat | Always passes |
| 5 | `MigrationAddModelMetadataColumn` | Add `metadata` JSON column to provider_models | `columnExists(metadata)` |
| 6 | `MigrationMultiProvider` | Rebuild active_multiplex with composite PK (target_cli_id, provider_id); migrate existing data | `columnExists(id)` on new schema |
| 7 | `MigrationRemoveOpenCodeNpm` | Remove hardcoded npm override from opencode's mutator_config | `WHERE mutator_config = old_value` |
| 8 | `MigrationAddDefaultContextWindow` | Add `default_context_window` to providers | `columnExists(default_context_window)` |
| 9 | `CreateIndexes` | Create `idx_provider_models_provider_id` | `IF NOT EXISTS` |
| 10 | `SeedTargetCLIs` | Insert 5 default CLI rows | `INSERT OR IGNORE` |
| 11 | `MigrationCopilotShellProfile` | Migrate copilot from `copilot-env-file` to `copilot-shell-profile` | `WHERE mutator = old_value` |

**Migration design rules**:

- Never drop columns (except in no-op migrations)
- Never modify existing data unless target is explicit (WHERE clause)
- Check existence before altering
- Use transactions for multi-step migrations (MigrationMultiProvider)

---

## Query Patterns

### Active multiplexes with joined data

```sql
SELECT am.target_cli_id, am.provider_id, am.model_mappings, am.activated_at,
       COALESCE(p.name, '') AS provider_name,
       COALESCE(tc.name, '') AS cli_name,
       COALESCE(p.status, '') AS provider_status
FROM active_multiplex am
JOIN providers p ON am.provider_id = p.id
JOIN target_clis tc ON am.target_cli_id = tc.id
```

### Upsert multiplex binding

```sql
INSERT INTO active_multiplex (target_cli_id, provider_id, model_mappings, activated_at)
VALUES (?, ?, ?, datetime('now'))
ON CONFLICT(target_cli_id, provider_id) DO UPDATE SET
  model_mappings = excluded.model_mappings,
  activated_at = datetime('now')
```

### Atomic model replace (delete + insert in transaction)

```sql
BEGIN TRANSACTION;
DELETE FROM provider_models WHERE provider_id = ?;
INSERT INTO provider_models (provider_id, model_name) VALUES (?, ?), (?, ?), ...;
COMMIT;
```

### 24-hour cache with expiry

```sql
SELECT value FROM update_cache
WHERE key = ?
  AND checked_at > datetime('now', '-24 hours');

INSERT INTO update_cache (key, value, checked_at)
VALUES (?, ?, datetime('now'))
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  checked_at = excluded.checked_at;
```

---

## Model Metadata JSON Structure

The `metadata` column in `provider_models` stores a JSON object. Supported keys are defined as constants in `domain/provider.go`.

```json
{
  "context_window": 1000000,
  "max_tokens": 384000,
  "reasoning": true,
  "input_modalities": ["text", "image"],
  "cost": {
    "input": 0.435,
    "output": 0.87,
    "cacheRead": 0.003625,
    "cacheWrite": 0.435
  },
  "compat": {
    "supportsDeveloperRole": false,
    "supportsReasoningEffort": false
  },
  "thinking_level_map": {
    "minimal": null,
    "low": null,
    "medium": null,
    "high": "high",
    "xhigh": "max"
  },
  "context_suffix": "[1m]",
  "headers": {},
  "extra_env": {}
}
```

**Cassette aliases** (for pi):

- `contextWindow` (camelCase) — same as `context_window`
- `maxTokens` (camelCase) — same as `max_tokens`
