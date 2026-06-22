package sqlite

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func enableForeignKeys(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}
	enableForeignKeys(t, db)
	if err := RunMigrations(db); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	if err := MigrationAddMutatorColumns(db); err != nil {
		t.Fatalf("failed to add mutator columns: %v", err)
	}
	if err := MigrationAddModelMetadataColumn(db); err != nil {
		t.Fatalf("failed to add metadata column: %v", err)
	}
	if err := MigrationMultiProvider(db); err != nil {
		t.Fatalf("failed to migrate multi-provider: %v", err)
	}
	if err := MigrationRemoveOpenCodeNpm(db); err != nil {
		t.Fatalf("failed to migrate remove opencode npm: %v", err)
	}
	if err := MigrationAddDefaultContextWindow(db); err != nil {
		t.Fatalf("failed to add default_context_window: %v", err)
	}
	if err := MigrationAddLogoURL(db); err != nil {
		t.Fatalf("failed to add logo_url: %v", err)
	}
	if err := MigrationAddCustomModelsColumn(db); err != nil {
		t.Fatalf("failed to add custom_models column: %v", err)
	}
	if err := CreateIndexes(db); err != nil {
		t.Fatalf("failed to create indexes: %v", err)
	}
	if err := SeedTargetCLIs(db); err != nil {
		t.Fatalf("failed to seed target CLIs: %v", err)
	}
	return db
}

func TestAddProvider_Success(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	id, err := repo.Add("Test Provider", "https://api.test.com/v1", "", "key123", "token456")
	if err != nil {
		t.Fatalf("AddProvider failed: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero provider ID")
	}

	provider, err := repo.Get(id)
	if err != nil {
		t.Fatalf("GetProvider failed: %v", err)
	}
	if provider.Name != "Test Provider" {
		t.Errorf("expected name 'Test Provider', got %q", provider.Name)
	}
	if provider.Status != "active" {
		t.Errorf("expected status 'active', got %q", provider.Status)
	}
}

func TestAddProvider_DuplicateName(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}

	_, err := repo.Add("Duplicate", "https://api.test.com", "", "key1", "token1")
	if err != nil {
		t.Fatalf("first AddProvider failed: %v", err)
	}

	_, err = repo.Add("Duplicate", "https://api.test.com", "", "key2", "token2")
	if err == nil {
		t.Fatal("expected error for duplicate provider name, got nil")
	}
}

func TestListProviders_Sorted(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	repo.Add("Zeta", "https://zeta.test", "", "k1", "t1")
	repo.Add("Alpha", "https://alpha.test", "", "k2", "t2")
	repo.Add("Beta", "https://beta.test", "", "k3", "t3")

	providers, err := repo.List()
	if err != nil {
		t.Fatalf("ListProviders failed: %v", err)
	}
	if len(providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(providers))
	}
	if providers[0].Name != "Alpha" {
		t.Errorf("expected first provider 'Alpha', got %q", providers[0].Name)
	}
	if providers[1].Name != "Beta" {
		t.Errorf("expected second provider 'Beta', got %q", providers[1].Name)
	}
	if providers[2].Name != "Zeta" {
		t.Errorf("expected third provider 'Zeta', got %q", providers[2].Name)
	}
}

func TestUpdateProviderStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	id, _ := repo.Add("StatusTest", "https://test.com", "", "k", "t")

	if err := repo.UpdateStatus(id, "error"); err != nil {
		t.Fatalf("UpdateProviderStatus failed: %v", err)
	}

	provider, _ := repo.Get(id)
	if provider.Status != "error" {
		t.Errorf("expected status 'error', got %q", provider.Status)
	}
}

func TestDeleteProvider_Cascade(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	id, _ := repo.Add("CascadeTest", "https://test.com", "", "k", "t")
	repo.InsertModels(id, []string{"model-a", "model-b"})

	models, err := repo.ListModels(id)
	if err != nil {
		t.Fatalf("ListModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	if err := repo.Delete(id); err != nil {
		t.Fatalf("DeleteProvider failed: %v", err)
	}

	models, err = repo.ListModels(id)
	if err != nil {
		t.Fatalf("ListModels after delete failed: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models after cascade delete, got %d", len(models))
	}

	_, err = repo.Get(id)
	if err == nil {
		t.Error("expected error for deleted provider, got nil")
	}
}

func TestInsertModels_ClearAndReInsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	providerID, _ := repo.Add("ModelTest", "https://test.com", "", "k", "t")

	if err := repo.InsertModels(providerID, []string{"old-model-1", "old-model-2"}); err != nil {
		t.Fatalf("first InsertModels failed: %v", err)
	}

	if err := repo.InsertModels(providerID, []string{"new-model-1", "new-model-2", "new-model-3"}); err != nil {
		t.Fatalf("second InsertModels failed: %v", err)
	}

	models, _ := repo.ListModels(providerID)
	if len(models) != 3 {
		t.Errorf("expected 3 models after re-insert, got %d", len(models))
	}
	if models[0].ModelName != "new-model-1" {
		t.Errorf("expected first model 'new-model-1', got %q", models[0].ModelName)
	}
}

func TestInsertModels_EmptyList(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	providerID, _ := repo.Add("EmptyModelTest", "https://test.com", "", "k", "t")

	repo.InsertModels(providerID, []string{"model-1"})
	if err := repo.InsertModels(providerID, []string{}); err != nil {
		t.Fatalf("InsertModels with empty list failed: %v", err)
	}

	models, _ := repo.ListModels(providerID)
	if len(models) != 0 {
		t.Errorf("expected 0 models after empty insert, got %d", len(models))
	}
}

func TestSetAndGetActiveMultiplex(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	providerRepo := &ProviderRepository{DB: db}
	muxRepo := &MultiplexRepository{DB: db}

	providerID, _ := providerRepo.Add("ActiveTest", "https://test.com", "", "k", "t")

	mappings := `{"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4"}`

	if err := muxRepo.SetActive(1, providerID, mappings); err != nil {
		t.Fatalf("SetActiveMultiplex failed: %v", err)
	}

	am, err := muxRepo.GetActive(1)
	if err != nil {
		t.Fatalf("GetActiveMultiplex failed: %v", err)
	}
	if am == nil {
		t.Fatal("expected non-nil ActiveMultiplex")
	}
	if am.TargetCLIID != 1 {
		t.Errorf("expected target_cli_id 1, got %d", am.TargetCLIID)
	}
	if am.ProviderID != providerID {
		t.Errorf("expected provider_id %d, got %d", providerID, am.ProviderID)
	}
}

func TestGetActiveMultiplex_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	muxRepo := &MultiplexRepository{DB: db}
	am, err := muxRepo.GetActive(999)
	if err != nil {
		t.Fatalf("GetActiveMultiplex for non-existent CLI should not error: %v", err)
	}
	if am != nil {
		t.Errorf("expected nil for non-existent ActiveMultiplex, got target_cli_id=%d", am.TargetCLIID)
	}
}

func TestSetActiveMultiplex_MultiProvider(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	providerRepo := &ProviderRepository{DB: db}
	muxRepo := &MultiplexRepository{DB: db}

	pid1, _ := providerRepo.Add("Prov1", "https://p1.test", "", "k1", "t1")
	pid2, _ := providerRepo.Add("Prov2", "https://p2.test", "", "k2", "t2")

	// Set two different providers for the same CLI
	muxRepo.SetActive(1, pid1, `{"var": "model1"}`)
	muxRepo.SetActive(1, pid2, `{"var": "model2"}`)

	// ListForCLI should return both
	all, _ := muxRepo.ListForCLI(1)
	if len(all) != 2 {
		t.Fatalf("expected 2 multiplexes, got %d", len(all))
	}

	// Update existing binding (same CLI + provider) should replace
	muxRepo.SetActive(1, pid1, `{"var": "updated"}`)
	all, _ = muxRepo.ListForCLI(1)
	if len(all) != 2 {
		t.Fatalf("expected 2 multiplexes after update, got %d", len(all))
	}
	// Find pid1 and check mappings
	for _, am := range all {
		if am.ProviderID == pid1 {
			if !strings.Contains(am.ModelMappings, "updated") {
				t.Errorf("expected updated mappings for pid1, got %s", am.ModelMappings)
			}
		}
	}
}

func TestClearActiveMultiplex(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	providerRepo := &ProviderRepository{DB: db}
	muxRepo := &MultiplexRepository{DB: db}

	pid, _ := providerRepo.Add("ClearTest", "https://test.com", "", "k", "t")
	muxRepo.SetActive(1, pid, `{"var": "model"}`)

	if err := muxRepo.ClearActive(1); err != nil {
		t.Fatalf("ClearActiveMultiplex failed: %v", err)
	}

	am, _ := muxRepo.GetActive(1)
	if am != nil {
		t.Errorf("expected nil after clear, got target_cli_id=%d", am.TargetCLIID)
	}
}

func TestListActiveMultiplexes_Join(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	providerRepo := &ProviderRepository{DB: db}
	muxRepo := &MultiplexRepository{DB: db}

	pid, _ := providerRepo.Add("JoinTest", "https://test.com", "", "k", "t")
	mappings := `{"model": "claude-sonnet-4"}`
	muxRepo.SetActive(1, pid, mappings)

	multiplexes, err := muxRepo.ListActive()
	if err != nil {
		t.Fatalf("ListActiveMultiplexes failed: %v", err)
	}
	if len(multiplexes) != 1 {
		t.Fatalf("expected 1 active multiplex, got %d", len(multiplexes))
	}
	if multiplexes[0].ProviderName != "JoinTest" {
		t.Errorf("expected provider_name 'JoinTest', got %q", multiplexes[0].ProviderName)
	}
	if multiplexes[0].CLIName != "claude-code" {
		t.Errorf("expected cli_name 'claude-code', got %q", multiplexes[0].CLIName)
	}
}

func TestSeedTargetCLIs_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	if err := SeedTargetCLIs(db); err != nil {
		t.Fatalf("second SeedTargetCLIs failed: %v", err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM target_clis WHERE name = 'claude-code'").Scan(&count)
	if count != 1 {
		t.Errorf("expected exactly 1 claude-code row, got %d", count)
	}

	var envVars string
	db.QueryRow("SELECT env_vars FROM target_clis WHERE name = 'claude-code'").Scan(&envVars)
	var vars []string
	if err := json.Unmarshal([]byte(envVars), &vars); err != nil {
		t.Fatalf("failed to parse env_vars JSON: %v", err)
	}
	if len(vars) != 4 {
		t.Errorf("expected 4 env vars, got %d", len(vars))
	}
}

func TestListAllModels_Join(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := &ProviderRepository{DB: db}
	pid, _ := repo.Add("ModelProvider", "https://test.com", "", "k", "t")
	repo.InsertModels(pid, []string{"claude-sonnet-4", "claude-haiku-3"})

	models, err := repo.ListAllModels()
	if err != nil {
		t.Fatalf("ListAllModels failed: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ProviderName != "ModelProvider" {
		t.Errorf("expected provider_name 'ModelProvider', got %q", models[0].ProviderName)
	}
}

func TestDeleteProvider_ActiveMultiplexCascade(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	providerRepo := &ProviderRepository{DB: db}
	muxRepo := &MultiplexRepository{DB: db}

	pid, _ := providerRepo.Add("CascadeMX", "https://test.com", "", "k", "t")
	muxRepo.SetActive(1, pid, `{"model": "m1"}`)

	if err := providerRepo.Delete(pid); err != nil {
		t.Fatalf("DeleteProvider failed: %v", err)
	}

	am, _ := muxRepo.GetActive(1)
	if am != nil {
		t.Errorf("expected nil after cascade delete, got target_cli_id=%d", am.TargetCLIID)
	}
}

func TestTargetCLIRepository_List(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cliRepo := &TargetCLIRepository{DB: db}
	clis, err := cliRepo.List()
	if err != nil {
		t.Fatalf("ListTargetCLIs failed: %v", err)
	}
	if len(clis) != 5 {
		t.Fatalf("expected 5 target CLIs, got %d", len(clis))
	}
	if clis[0].Name != "claude-code" {
		t.Errorf("expected 'claude-code', got %q", clis[0].Name)
	}
	if clis[0].Mutator != "claude-settings-json" {
		t.Errorf("expected mutator 'claude-settings-json', got %q", clis[0].Mutator)
	}
	if clis[0].MutatorConfig != "{}" {
		t.Errorf("expected mutator_config '{}', got %q", clis[0].MutatorConfig)
	}
}
