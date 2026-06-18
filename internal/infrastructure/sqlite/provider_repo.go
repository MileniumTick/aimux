package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/MileniumTick/aimux/internal/domain"
)

// ProviderRepository implements domain.ProviderRepository.
type ProviderRepository struct {
	DB *sql.DB
}

// Add inserts a new provider and returns its ID.
func (r *ProviderRepository) Add(name, baseURL, apiKey, authToken string, apiType domain.ApiType) (int64, error) {
	result, err := r.DB.Exec(
		`INSERT INTO providers (name, base_url, api_key, auth_token, api_type, status) VALUES (?, ?, ?, ?, ?, 'active')`,
		name, baseURL, apiKey, authToken, string(apiType),
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
		`SELECT id, name, base_url, api_key, auth_token, api_type, status, created_at, updated_at FROM providers WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.Name, &p.BaseURL, &p.APIKey, &p.AuthToken, &p.ApiType, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, fmt.Errorf("get provider %d: %w", id, err)
	}
	return p, nil
}

// List returns all providers ordered by name ascending.
func (r *ProviderRepository) List() ([]domain.Provider, error) {
	rows, err := r.DB.Query(
		`SELECT id, name, base_url, api_key, auth_token, api_type, status, created_at, updated_at FROM providers ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list providers: %w", err)
	}
	defer rows.Close()

	var providers []domain.Provider
	for rows.Next() {
		var p domain.Provider
		if err := rows.Scan(&p.ID, &p.Name, &p.BaseURL, &p.APIKey, &p.AuthToken, &p.ApiType, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan provider: %w", err)
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

// Update updates a provider's base_url, api_key, auth_token, and api_type.
func (r *ProviderRepository) Update(id int64, baseURL, apiKey, authToken string, apiType domain.ApiType) error {
	_, err := r.DB.Exec(
		`UPDATE providers SET base_url = ?, api_key = ?, auth_token = ?, api_type = ?, updated_at = datetime('now') WHERE id = ?`,
		baseURL, apiKey, authToken, string(apiType), id,
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

	// Insert new models
	if len(modelNames) > 0 {
		stmt := `INSERT INTO provider_models (provider_id, model_name) VALUES `
		var args []any
		placeholders := make([]string, 0, len(modelNames))
		for _, name := range modelNames {
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

// ListModels returns all models for a given provider ordered by model_name ASC.
func (r *ProviderRepository) ListModels(providerID int64) ([]domain.ProviderModel, error) {
	rows, err := r.DB.Query(
		`SELECT id, provider_id, model_name FROM provider_models WHERE provider_id = ? ORDER BY model_name ASC`,
		providerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()

	var models []domain.ProviderModel
	for rows.Next() {
		var m domain.ProviderModel
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelName); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		models = append(models, m)
	}
	return models, rows.Err()
}

// ListAllModels returns all models across all providers with provider name joined.
func (r *ProviderRepository) ListAllModels() ([]domain.ProviderModel, error) {
	rows, err := r.DB.Query(
		`SELECT pm.id, pm.provider_id, pm.model_name, p.name AS provider_name
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
		if err := rows.Scan(&m.ID, &m.ProviderID, &m.ModelName, &m.ProviderName); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		models = append(models, m)
	}
	return models, rows.Err()
}
