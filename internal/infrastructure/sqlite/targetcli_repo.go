package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/jchavarriam/aimux/internal/domain"
)

// TargetCLIRepository implements domain.TargetCLIRepository.
type TargetCLIRepository struct {
	DB *sql.DB
}

// List returns all target CLIs ordered by name ASC.
func (r *TargetCLIRepository) List() ([]domain.TargetCLI, error) {
	rows, err := r.DB.Query(
		`SELECT id, name, config_path, env_vars, mutator, mutator_config FROM target_clis ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list target clis: %w", err)
	}
	defer rows.Close()

	var clis []domain.TargetCLI
	for rows.Next() {
		var c domain.TargetCLI
		if err := rows.Scan(&c.ID, &c.Name, &c.ConfigPath, &c.EnvVars, &c.Mutator, &c.MutatorConfig); err != nil {
			return nil, fmt.Errorf("scan target cli: %w", err)
		}
		clis = append(clis, c)
	}
	return clis, rows.Err()
}

// Get returns a single target CLI by ID.
func (r *TargetCLIRepository) Get(id int64) (domain.TargetCLI, error) {
	var c domain.TargetCLI
	err := r.DB.QueryRow(
		`SELECT id, name, config_path, env_vars, mutator, mutator_config FROM target_clis WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.ConfigPath, &c.EnvVars, &c.Mutator, &c.MutatorConfig)
	if err != nil {
		return c, fmt.Errorf("get target cli %d: %w", id, err)
	}
	return c, nil
}

// Update updates a target CLI's config_path.
func (r *TargetCLIRepository) Update(c domain.TargetCLI) error {
	_, err := r.DB.Exec(
		`UPDATE target_clis SET config_path = ? WHERE id = ?`,
		c.ConfigPath, c.ID,
	)
	if err != nil {
		return fmt.Errorf("update target cli %d: %w", c.ID, err)
	}
	return nil
}
