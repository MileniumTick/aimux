package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/MileniumTick/aimux/internal/domain"
)

// MultiplexRepository implements domain.MultiplexRepository.
type MultiplexRepository struct {
	DB *sql.DB
}

// GetActive returns the active multiplex for a given target CLI.
// Returns empty struct if no row exists (no error).
func (r *MultiplexRepository) GetActive(targetCLIID int64) (domain.ActiveMultiplex, error) {
	var am domain.ActiveMultiplex
	err := r.DB.QueryRow(
		`SELECT target_cli_id, provider_id, model_mappings, activated_at,
		        '' AS provider_name, '' AS cli_name
		 FROM active_multiplex WHERE target_cli_id = ?`,
		targetCLIID,
	).Scan(&am.TargetCLIID, &am.ProviderID, &am.ModelMappings, &am.ActivatedAt, &am.ProviderName, &am.CLIName)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.ActiveMultiplex{}, nil
		}
		return am, fmt.Errorf("get active multiplex: %w", err)
	}
	return am, nil
}

// SetActive inserts or updates an active multiplex row for a (CLI, provider) pair.
func (r *MultiplexRepository) SetActive(targetCLIID, providerID int64, modelMappingsJSON string) error {
	_, err := r.DB.Exec(
		`INSERT INTO active_multiplex (target_cli_id, provider_id, model_mappings, activated_at)
		 VALUES (?, ?, ?, datetime('now'))
		 ON CONFLICT(target_cli_id, provider_id) DO UPDATE SET
		   model_mappings = excluded.model_mappings,
		   activated_at = datetime('now')`,
		targetCLIID, providerID, modelMappingsJSON,
	)
	if err != nil {
		return fmt.Errorf("set active multiplex: %w", err)
	}
	return nil
}

// ListForCLI returns all active multiplex rows for a given CLI.
func (r *MultiplexRepository) ListForCLI(targetCLIID int64) ([]domain.ActiveMultiplex, error) {
	rows, err := r.DB.Query(
		`SELECT am.target_cli_id, am.provider_id, am.model_mappings, am.activated_at,
		        COALESCE(p.name, '') AS provider_name,
		        COALESCE(tc.name, '') AS cli_name,
		        COALESCE(p.status, '') AS provider_status
		 FROM active_multiplex am
		 JOIN providers p ON am.provider_id = p.id
		 JOIN target_clis tc ON am.target_cli_id = tc.id
		 WHERE am.target_cli_id = ?`,
		targetCLIID,
	)
	if err != nil {
		return nil, fmt.Errorf("list multiplexes for CLI: %w", err)
	}
	defer rows.Close()

	var multiplexes []domain.ActiveMultiplex
	for rows.Next() {
		var am domain.ActiveMultiplex
		if err := rows.Scan(&am.TargetCLIID, &am.ProviderID, &am.ModelMappings, &am.ActivatedAt,
			&am.ProviderName, &am.CLIName, &am.ProviderStatus); err != nil {
			return nil, fmt.Errorf("scan active multiplex: %w", err)
		}
		multiplexes = append(multiplexes, am)
	}
	return multiplexes, rows.Err()
}

// ClearActive deletes all active multiplex rows for a given target CLI.
func (r *MultiplexRepository) ClearActive(targetCLIID int64) error {
	_, err := r.DB.Exec(`DELETE FROM active_multiplex WHERE target_cli_id = ?`, targetCLIID)
	if err != nil {
		return fmt.Errorf("clear active multiplex: %w", err)
	}
	return nil
}

// ClearBinding deletes a specific (CLI, provider) binding.
func (r *MultiplexRepository) ClearBinding(targetCLIID, providerID int64) error {
	_, err := r.DB.Exec(`DELETE FROM active_multiplex WHERE target_cli_id = ? AND provider_id = ?`, targetCLIID, providerID)
	if err != nil {
		return fmt.Errorf("clear binding: %w", err)
	}
	return nil
}

// ListActive returns all active multiplex rows with joined provider and CLI names.
func (r *MultiplexRepository) ListActive() ([]domain.ActiveMultiplex, error) {
	rows, err := r.DB.Query(
		`SELECT am.target_cli_id, am.provider_id, am.model_mappings, am.activated_at,
		        COALESCE(p.name, '') AS provider_name,
		        COALESCE(tc.name, '') AS cli_name,
		        COALESCE(p.status, '') AS provider_status
		 FROM active_multiplex am
		 JOIN providers p ON am.provider_id = p.id
		 JOIN target_clis tc ON am.target_cli_id = tc.id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active multiplexes: %w", err)
	}
	defer rows.Close()

	var multiplexes []domain.ActiveMultiplex
	for rows.Next() {
		var am domain.ActiveMultiplex
		if err := rows.Scan(&am.TargetCLIID, &am.ProviderID, &am.ModelMappings, &am.ActivatedAt,
			&am.ProviderName, &am.CLIName, &am.ProviderStatus); err != nil {
			return nil, fmt.Errorf("scan active multiplex: %w", err)
		}
		multiplexes = append(multiplexes, am)
	}
	return multiplexes, rows.Err()
}
