package update

import (
	"database/sql"
)

// CacheKeyLatestVersion is the cache key used to store the latest released version.
const CacheKeyLatestVersion = "latest_version"

// CacheGet retrieves a cached value for the given key if it was stored
// within the last 24 hours. Returns ("", nil) on cache miss (expired or missing).
// On any DB error, returns ("", nil) — the caller treats this as a cache miss.
func CacheGet(db *sql.DB, key string) (string, error) {
	if db == nil {
		return "", nil
	}

	var value string
	err := db.QueryRow(
		`SELECT value FROM update_cache
		 WHERE key = ?
		   AND checked_at > datetime('now', '-24 hours')`,
		key,
	).Scan(&value)
	if err != nil {
		// Cache miss or DB error — treat both as miss
		return "", nil
	}
	return value, nil
}

// CacheSet stores a key-value pair with the current timestamp.
// Uses INSERT ... ON CONFLICT DO UPDATE for idempotency.
func CacheSet(db *sql.DB, key, value string) error {
	if db == nil {
		return nil
	}

	_, err := db.Exec(
		`INSERT INTO update_cache (key, value, checked_at)
		 VALUES (?, ?, datetime('now'))
		 ON CONFLICT(key) DO UPDATE SET
		   value = excluded.value,
		   checked_at = excluded.checked_at`,
		key, value,
	)
	return err
}
