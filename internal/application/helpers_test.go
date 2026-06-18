package application

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/mutators"
	"github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
	if err := sqlite.RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	if err := sqlite.MigrationAddMutatorColumns(db); err != nil {
		t.Fatalf("failed to add mutator columns: %v", err)
	}
	if err := sqlite.MigrationAddModelMetadataColumn(db); err != nil {
		t.Fatalf("failed to add metadata column: %v", err)
	}
	if err := sqlite.CreateIndexes(db); err != nil {
		t.Fatalf("failed to create indexes: %v", err)
	}
	if err := sqlite.SeedTargetCLIs(db); err != nil {
		t.Fatalf("failed to seed target CLIs: %v", err)
	}
	return db
}

func setupProviderTest(t *testing.T) *ProviderUseCases {
	t.Helper()
	db := setupTestDB(t)
	providerRepo := &sqlite.ProviderRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}
	return NewProviderUseCases(providerRepo, multiplexRepo)
}

type switchTestHarness struct {
	uc  *SwitchUseCases
	db  *sql.DB
}

// setupSwitchTest returns a SwitchUseCases for tests that don't need direct DB access.
func setupSwitchTest(t *testing.T) *SwitchUseCases {
	t.Helper()
	return setupSwitchHarness(t).uc
}

func defaultMutatorRegistry() map[string]domain.ConfigMutator {
	return map[string]domain.ConfigMutator{
		"claude-settings-json": &mutators.ClaudeSettingsJSON{},
	}
}

func setupSwitchHarness(t *testing.T) *switchTestHarness {
	t.Helper()
	db := setupTestDB(t)
	providerRepo := &sqlite.ProviderRepository{DB: db}
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}
	return &switchTestHarness{
		uc:  NewSwitchUseCases(providerRepo, cliRepo, multiplexRepo, defaultMutatorRegistry()),
		db:  db,
	}
}

func addTestProvider(t *testing.T, uc *SwitchUseCases, name, baseURL string) int64 {
	t.Helper()
	id, err := uc.providerRepo.Add(name, baseURL, "test-api-key", "test-auth-token", domain.ApiTypeOpenAI)
	if err != nil {
		t.Fatalf("Add provider failed: %v", err)
	}
	uc.providerRepo.UpdateStatus(id, "active")
	return id
}

func insertTestModels(t *testing.T, uc *SwitchUseCases, providerID int64, models []string) {
	t.Helper()
	if err := uc.providerRepo.InsertModels(providerID, models); err != nil {
		t.Fatalf("InsertModels failed: %v", err)
	}
}
