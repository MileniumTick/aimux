package sqlite

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

// Open opens or creates a SQLite database at the given path, enables WAL mode,
// and sets file permissions to 0600.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign key enforcement
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Set busy timeout to avoid "database is locked" errors
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy timeout: %w", err)
	}

	// Set file permissions
	if err := os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, fmt.Errorf("set database permissions: %w", err)
	}

	return db, nil
}

// RunMigrations creates all tables if they do not exist.
func RunMigrations(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			base_url TEXT NOT NULL,
			discovery_url TEXT NOT NULL DEFAULT '',
			api_key TEXT NOT NULL DEFAULT '',
			auth_token TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'error')),
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS provider_models (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
			model_name TEXT NOT NULL,
			UNIQUE(provider_id, model_name)
		)`,
		`CREATE TABLE IF NOT EXISTS target_clis (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			config_path TEXT NOT NULL,
			env_vars TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS active_multiplex (
			target_cli_id INTEGER PRIMARY KEY REFERENCES target_clis(id) ON DELETE CASCADE,
			provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
			model_mappings TEXT NOT NULL,
			activated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}
	return nil
}

// MigrationAddMutatorColumns adds mutator and mutator_config columns to target_clis.
// Idempotent: checks if columns exist before altering.
func MigrationAddMutatorColumns(db *sql.DB) error {
	type col struct {
		name, def string
	}
	columns := []col{
		{"mutator", `TEXT NOT NULL DEFAULT 'claude-settings-json'`},
		{"mutator_config", `TEXT NOT NULL DEFAULT '{}'`},
	}
	for _, c := range columns {
		exists, err := columnExists(db, "target_clis", c.name)
		if err != nil {
			return fmt.Errorf("check column %s: %w", c.name, err)
		}
		if exists {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE target_clis ADD COLUMN %s %s", c.name, c.def)); err != nil {
			return fmt.Errorf("add mutator columns migration: %w", err)
		}
	}
	return nil
}

// MigrationAddDiscoveryURLColumn adds the discovery_url column to providers.
// Idempotent: checks if column exists before altering.
func MigrationAddDiscoveryURLColumn(db *sql.DB) error {
	exists, err := columnExists(db, "providers", "discovery_url")
	if err != nil {
		return fmt.Errorf("check discovery_url column: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE providers ADD COLUMN discovery_url TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add discovery_url column migration: %w", err)
	}
	return nil
}

// MigrationDropApiTypeColumn is a no-op that keeps existing databases working.
// api_type was removed from the schema and code — old databases still have the
// column but it's never read or written.
func MigrationDropApiTypeColumn(db *sql.DB) error {
	return nil
}

// MigrationAddModelMetadataColumn adds the metadata JSON column to provider_models.
// Idempotent: checks if column exists before altering.
func MigrationAddModelMetadataColumn(db *sql.DB) error {
	exists, err := columnExists(db, "provider_models", "metadata")
	if err != nil {
		return fmt.Errorf("check metadata column: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE provider_models ADD COLUMN metadata TEXT NOT NULL DEFAULT '{}'`); err != nil {
		return fmt.Errorf("add metadata column migration: %w", err)
	}
	return nil
}

// columnExists checks whether a column exists in a table.
func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk bool
		var defaultVal *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// MigrationRemoveOpenCodeNpm removes the hardcoded npm override from the
// opencode CLI's mutator_config. The npm package is now derived dynamically
// from the provider's API type (openai-compatible).
// Idempotent: only affects rows that still have the old npm value.
func MigrationRemoveOpenCodeNpm(db *sql.DB) error {
	_, err := db.Exec(`
		UPDATE target_clis
		SET mutator_config = '{"provider_id":"custom"}'
		WHERE name = 'opencode'
		AND mutator_config = '{"provider_id":"custom","npm":"@ai-sdk/openai-compatible"}'
	`)
	if err != nil {
		return fmt.Errorf("remove opencode npm migration: %w", err)
	}
	return nil
}

// MigrationMultiProvider changes active_multiplex from single-row PK to
// composite (target_cli_id, provider_id) PK so a CLI can bind multiple providers.
// Idempotent: checks if the table already uses the new schema.
func MigrationMultiProvider(db *sql.DB) error {
	hasID, err := columnExists(db, "active_multiplex", "id")
	if err != nil {
		return err
	}
	if hasID {
		return nil // already migrated
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create new table with composite PK
	if _, err := tx.Exec(`
		CREATE TABLE active_multiplex_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_cli_id INTEGER NOT NULL REFERENCES target_clis(id) ON DELETE CASCADE,
			provider_id INTEGER NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
			model_mappings TEXT NOT NULL,
			activated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE(target_cli_id, provider_id)
		)
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO active_multiplex_new (target_cli_id, provider_id, model_mappings, activated_at)
		SELECT target_cli_id, provider_id, model_mappings, activated_at FROM active_multiplex
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE active_multiplex`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE active_multiplex_new RENAME TO active_multiplex`); err != nil {
		return err
	}

	return tx.Commit()
}

// MigrationAddDefaultContextWindow adds default_context_window column to providers.
// Idempotent: checks if column exists before altering.
func MigrationAddDefaultContextWindow(db *sql.DB) error {
	exists, err := columnExists(db, "providers", "default_context_window")
	if err != nil {
		return fmt.Errorf("check default_context_window column: %w", err)
	}
	if exists {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE providers ADD COLUMN default_context_window INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add default_context_window column migration: %w", err)
	}
	return nil
}

// MigrationCopilotShellProfile updates the github-copilot target CLI row to use
// the new shell-profile mutator instead of the deprecated copilot-env-file.
func MigrationCopilotShellProfile(db *sql.DB) error {
	_, err := db.Exec(`UPDATE target_clis SET mutator = 'copilot-shell-profile', config_path = '' WHERE name = 'github-copilot' AND mutator = 'copilot-env-file'`)
	if err != nil {
		return fmt.Errorf("migrate copilot to shell profile: %w", err)
	}
	return nil
}

// CreateIndexes creates indexes if they do not exist.
func CreateIndexes(db *sql.DB) error {
	statements := []string{
		"CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON provider_models(provider_id)",
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}
	return nil
}

// SeedTargetCLIs inserts the default target CLI rows.
// Idempotent via INSERT OR IGNORE.
func SeedTargetCLIs(db *sql.DB) error {
	stmt := `INSERT OR IGNORE INTO target_clis (name, config_path, env_vars, mutator, mutator_config)
		VALUES (?, ?, ?, ?, ?)`

	seeds := []struct {
		name, configPath, envVars, mutator, mutatorConfig string
	}{
		{
			"claude-code",
			"~/.config/claude/settings.json",
			`["ANTHROPIC_DEFAULT_HAIKU_MODEL","ANTHROPIC_DEFAULT_SONNET_MODEL","ANTHROPIC_DEFAULT_OPUS_MODEL","CLAUDE_CODE_SUBAGENT_MODEL"]`,
			"claude-settings-json",
			"{}",
		},
		{
			"opencode",
			"~/.config/opencode/config.json",
			`["OPENCODE_MODEL","OPENCODE_FAST_MODEL"]`,
			"opencode-provider-json",
			`{"provider_id":"custom"}`,
		},
		{
			"codex",
			"~/.codex/config.toml",
			`["CODEX_MODEL"]`,
			"codex-config-toml",
			`{"provider_id":"custom"}`,
		},
		{
			"github-copilot",
			"", // shell profile auto-detected at runtime — no config path
			`["COPILOT_MODEL"]`,
			"copilot-shell-profile",
			"{}",
		},
		{
			"pi-ai",
			"~/.pi/agent/models.json",
			`["PI_DEFAULT_MODEL","PI_FAST_MODEL"]`,
			"pi-dual-json",
			`{"provider_id":"custom"}`,
		},
	}

	for _, s := range seeds {
		if _, err := db.Exec(stmt, s.name, s.configPath, s.envVars, s.mutator, s.mutatorConfig); err != nil {
			return fmt.Errorf("seed target clis: %w", err)
		}
	}
	return nil
}
