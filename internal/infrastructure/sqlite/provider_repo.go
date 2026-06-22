package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// ProviderRepository implements domain.ProviderRepository.
type ProviderRepository struct {
	DB *sql.DB
}

// Add inserts a new provider and returns its ID.
func (r *ProviderRepository) Add(name, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) (int64, error) {
	dcw := int64(0)
	if len(defaultContextWindow) > 0 {
		dcw = defaultContextWindow[0]
	}
	result, err := r.DB.Exec(
		`INSERT INTO providers (name, base_url, discovery_url, default_context_window, api_key, auth_token, status) VALUES (?, ?, ?, ?, ?, ?, 'active')`,
		name, baseURL, discoveryURL, dcw, apiKey, authToken,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return 0, fmt.Errorf("provider '%s' already exists", name)
		}
		return 0, fmt.Errorf("add provider: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get last insert id: %w", err)
	}
	return id, nil
}

// Get returns a single provider by ID.
func (r *ProviderRepository) Get(id int64) (domain.Provider, error) {
	var p domain.Provider
	err := r.DB.QueryRow(
		`SELECT id, name, base_url, discovery_url, default_context_window, logo_url, api_key, auth_token, status, created_at, updated_at, COALESCE(custom_models, '') FROM providers WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Name, &p.BaseURL, &p.DiscoveryURL, &p.DefaultContextWindow, &p.LogoURL, &p.APIKey, &p.AuthToken, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.CustomModels)
	if err != nil {
		return p, fmt.Errorf("get provider %d: %w", id, err)
	}
	return p, nil
}

// List returns all providers ordered by name ascending.
func (r *ProviderRepository) List() ([]domain.Provider, error) {
	rows, err := r.DB.Query(
		`SELECT id, name, base_url, discovery_url, default_context_window, logo_url, api_key, auth_token, status, created_at, updated_at, COALESCE(custom_models, '') FROM providers ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	var providers []domain.Provider
	for rows.Next() {
		var p domain.Provider
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.DiscoveryURL, &p.DefaultContextWindow, &p.LogoURL, &p.APIKey, &p.AuthToken, &p.Status, &p.CreatedAt, &p.UpdatedAt, &p.CustomModels); err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// Update updates a provider's base_url, discovery_url, default_context_window, api_key, and auth_token.
func (r *ProviderRepository) Update(id int64, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) error {
	dcw := int64(0)
	if len(defaultContextWindow) > 0 {
		dcw = defaultContextWindow[0]
	}
	_, err := r.DB.Exec(
		`UPDATE providers SET base_url = ?, discovery_url = ?, default_context_window = ?, api_key = ?, auth_token = ?, updated_at = datetime('now') WHERE id = ?`,
		baseURL, discoveryURL, dcw, apiKey, authToken, id,
	)
	if err != nil {
		return fmt.Errorf("update provider %d: %w", id, err)
	}
	return nil
}

// UpdateStatus updates the status and updated_at timestamp.
func (r *ProviderRepository) UpdateStatus(id int64, status string) error {
	_, err := r.DB.Exec(
		`UPDATE providers SET status = ?, updated_at = datetime('now') WHERE id = ?`,
		status, id,
	)
	if err != nil {
		return fmt.Errorf("update provider status: %w", err)
	}
	return nil
}

// Delete deletes a provider by ID (CASCADE deletes models and active_multiplex).
func (r *ProviderRepository) Delete(id int64) error {
	_, err := r.DB.Exec(`DELETE FROM providers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete provider %d: %w", id, err)
	}
	return nil
}

// InsertModels clears existing models for a provider and inserts new ones.
// Deduplicates model names to avoid UNIQUE constraint violations (Bifrost may
// return the same model from multiple upstreams).
func (r *ProviderRepository) InsertModels(providerID int64, modelNames []string) error {
	tx, err := r.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Clear existing models
	if _, err := tx.Exec(`DELETE FROM provider_models WHERE provider_id = ?`, providerID); err != nil {
		return fmt.Errorf("clear models: %w", err)
	}

	// Deduplicate before insert — Bifrost may return the same model from
	// multiple upstreams.
	seen := make(map[string]struct{}, len(modelNames))
	unique := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unique = append(unique, name)
	}

	// Insert new models
	if len(unique) > 0 {
		stmt := `INSERT INTO provider_models (provider_id, model_name) VALUES `
		var args []any
		placeholders := make([]string, 0, len(unique))
		for _, name := range unique {
			placeholders = append(placeholders, "(?, ?)")
			args = append(args, providerID, name)
		}
		stmt += strings.Join(placeholders, ", ")
		if _, err := tx.Exec(stmt, args...); err != nil {
			return fmt.Errorf("insert models: %w", err)
		}
	}

	return tx.Commit()
}

// DeleteModelsByProvider deletes all models for a given provider.
func (r *ProviderRepository) DeleteModelsByProvider(providerID int64) error {
	_, err := r.DB.Exec(`DELETE FROM provider_models WHERE provider_id = ?`, providerID)
	if err != nil {
		return fmt.Errorf("delete models for provider %d: %w", providerID, err)
	}
	return nil
}

// AddCustomModels inserts custom model names for a provider, skipping duplicates.
// Also saves the comma-separated list to the custom_models column for form pre-fill.
// Uses INSERT OR IGNORE so existing models (from API fetch) are preserved.
func (r *ProviderRepository) AddCustomModels(providerID int64, modelNames []string) error {
	if len(modelNames) == 0 {
		return nil
	}
	// Deduplicate
	seen := make(map[string]struct{}, len(modelNames))
	unique := make([]string, 0, len(modelNames))
	for _, name := range modelNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unique = append(unique, name)
	}
	if len(unique) == 0 {
		return nil
	}

	// Save to provider_models
	stmt := `INSERT OR IGNORE INTO provider_models (provider_id, model_name) VALUES `
	var args []any
	placeholders := make([]string, 0, len(unique))
	for _, name := range unique {
		placeholders = append(placeholders, "(?, ?)")
		args = append(args, providerID, name)
	}
	stmt += strings.Join(placeholders, ", ")
	if _, err := r.DB.Exec(stmt, args...); err != nil {
		return fmt.Errorf("add custom models: %w", err)
	}

	// Save raw list to providers.custom_models for form pre-fill
	raw := strings.Join(unique, ",")
	if _, err := r.DB.Exec(`UPDATE providers SET custom_models = ? WHERE id = ?`, raw, providerID); err != nil {
		return fmt.Errorf("save custom_models column: %w", err)
	}

	return nil
}

// ListModels returns all models for a given provider ordered by model_name ASC.
func (r *ProviderRepository) ListModels(providerID int64) ([]domain.ProviderModel, error) {
	rows, err := r.DB.Query(
		`SELECT id, provider_id, model_name, COALESCE(metadata, '{}') FROM provider_models WHERE provider_id = ? ORDER BY model_name ASC`,
		providerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()

	var models []domain.ProviderModel
	for rows.Next() {
		var m domain.ProviderModel
		var metaStr string
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelName, &metaStr); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		m.Metadata = parseModelMetadata(metaStr)
		models = append(models, m)
	}
	return models, rows.Err()
}

// ListAllModels returns all models across all providers with provider name joined.
func (r *ProviderRepository) ListAllModels() ([]domain.ProviderModel, error) {
	rows, err := r.DB.Query(
		`SELECT pm.id, pm.provider_id, pm.model_name, p.name AS provider_name, COALESCE(pm.metadata, '{}')
		 FROM provider_models pm
		 JOIN providers p ON pm.provider_id = p.id
		 ORDER BY pm.model_name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all models: %w", err)
	}
	defer rows.Close()

	var models []domain.ProviderModel
	for rows.Next() {
		var m domain.ProviderModel
		var metaStr string
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelName, &m.ProviderName, &metaStr); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		m.Metadata = parseModelMetadata(metaStr)
		models = append(models, m)
	}
	return models, rows.Err()
}

// UpdateModelMetadata sets metadata for a specific model.
func (r *ProviderRepository) UpdateModelMetadata(providerID int64, modelName string, metadata domain.ModelMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	_, err = r.DB.Exec(
		`UPDATE provider_models SET metadata = ? WHERE provider_id = ? AND model_name = ?`,
		string(data), providerID, modelName,
	)
	if err != nil {
		return fmt.Errorf("update model metadata: %w", err)
	}
	return nil
}

func parseModelMetadata(raw string) domain.ModelMetadata {
	if raw == "" || raw == "{}" {
		return domain.ModelMetadata{}
	}
	var m domain.ModelMetadata
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return domain.ModelMetadata{}
	}
	return m
}
