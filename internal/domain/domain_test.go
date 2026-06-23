package domain

import (
	"strings"
	"testing"
)

// ── Compile-time interface checks ─────────────────────────────────────────

var _ ConfigMutator = (ConfigMutator)(nil)

// ── Struct zero values ────────────────────────────────────────────────────

func TestProviderZeroValue(t *testing.T) {
	var p Provider
	if p.ID != 0 {
		t.Errorf("Provider.ID zero value: expected 0, got %d", p.ID)
	}
	if p.Name != "" {
		t.Errorf("Provider.Name zero value: expected empty, got %q", p.Name)
	}
	if p.BaseURL != "" {
		t.Errorf("Provider.BaseURL zero value: expected empty, got %q", p.BaseURL)
	}
	if p.DiscoveryURL != "" {
		t.Errorf("Provider.DiscoveryURL zero value: expected empty, got %q", p.DiscoveryURL)
	}
	if p.DefaultContextWindow != 0 {
		t.Errorf("Provider.DefaultContextWindow zero value: expected 0, got %d", p.DefaultContextWindow)
	}
	if p.LogoURL != "" {
		t.Errorf("Provider.LogoURL zero value: expected empty, got %q", p.LogoURL)
	}
	if p.APIKey != "" {
		t.Errorf("Provider.APIKey zero value: expected empty, got %q", p.APIKey)
	}
	if p.AuthToken != "" {
		t.Errorf("Provider.AuthToken zero value: expected empty, got %q", p.AuthToken)
	}
	if p.Status != "" {
		t.Errorf("Provider.Status zero value: expected empty, got %q", p.Status)
	}
	if p.CreatedAt != "" {
		t.Errorf("Provider.CreatedAt zero value: expected empty, got %q", p.CreatedAt)
	}
	if p.UpdatedAt != "" {
		t.Errorf("Provider.UpdatedAt zero value: expected empty, got %q", p.UpdatedAt)
	}
	if p.CustomModels != "" {
		t.Errorf("Provider.CustomModels zero value: expected empty, got %q", p.CustomModels)
	}
}

func TestProviderModelZeroValue(t *testing.T) {
	var pm ProviderModel
	if pm.ID != 0 {
		t.Errorf("ProviderModel.ID zero value: expected 0, got %d", pm.ID)
	}
	if pm.ProviderID != 0 {
		t.Errorf("ProviderModel.ProviderID zero value: expected 0, got %d", pm.ProviderID)
	}
	if pm.ModelName != "" {
		t.Errorf("ProviderModel.ModelName zero value: expected empty, got %q", pm.ModelName)
	}
	if pm.ProviderName != "" {
		t.Errorf("ProviderModel.ProviderName zero value: expected empty, got %q", pm.ProviderName)
	}
	if pm.Metadata != nil {
		t.Errorf("ProviderModel.Metadata zero value: expected nil, got %v", pm.Metadata)
	}
}

func TestTargetCLIZeroValue(t *testing.T) {
	var tc TargetCLI
	if tc.ID != 0 {
		t.Errorf("TargetCLI.ID zero value: expected 0, got %d", tc.ID)
	}
	if tc.Name != "" {
		t.Errorf("TargetCLI.Name zero value: expected empty, got %q", tc.Name)
	}
	if tc.ConfigPath != "" {
		t.Errorf("TargetCLI.ConfigPath zero value: expected empty, got %q", tc.ConfigPath)
	}
	if tc.EnvVars != "" {
		t.Errorf("TargetCLI.EnvVars zero value: expected empty, got %q", tc.EnvVars)
	}
	if tc.Mutator != "" {
		t.Errorf("TargetCLI.Mutator zero value: expected empty, got %q", tc.Mutator)
	}
	if tc.MutatorConfig != "" {
		t.Errorf("TargetCLI.MutatorConfig zero value: expected empty, got %q", tc.MutatorConfig)
	}
}

func TestActiveMultiplexZeroValue(t *testing.T) {
	var am ActiveMultiplex
	if am.TargetCLIID != 0 {
		t.Errorf("ActiveMultiplex.TargetCLIID zero value: expected 0, got %d", am.TargetCLIID)
	}
	if am.ProviderID != 0 {
		t.Errorf("ActiveMultiplex.ProviderID zero value: expected 0, got %d", am.ProviderID)
	}
	if am.ModelMappings != "" {
		t.Errorf("ActiveMultiplex.ModelMappings zero value: expected empty, got %q", am.ModelMappings)
	}
	if am.ActivatedAt != "" {
		t.Errorf("ActiveMultiplex.ActivatedAt zero value: expected empty, got %q", am.ActivatedAt)
	}
	if am.ProviderName != "" {
		t.Errorf("ActiveMultiplex.ProviderName zero value: expected empty, got %q", am.ProviderName)
	}
	if am.CLIName != "" {
		t.Errorf("ActiveMultiplex.CLIName zero value: expected empty, got %q", am.CLIName)
	}
	if am.ProviderStatus != "" {
		t.Errorf("ActiveMultiplex.ProviderStatus zero value: expected empty, got %q", am.ProviderStatus)
	}
}

// ── ModelMetadata key constants ───────────────────────────────────────────

func TestMetaKeyConstants_NonEmpty(t *testing.T) {
	keys := []struct {
		name  string
		value string
	}{
		{"MetaContextWindow", MetaContextWindow},
		{"MetaMaxTokens", MetaMaxTokens},
		{"MetaReasoning", MetaReasoning},
		{"MetaInputModalities", MetaInputModalities},
		{"MetaCost", MetaCost},
		{"MetaCompat", MetaCompat},
		{"MetaThinkingLevelMap", MetaThinkingLevelMap},
		{"MetaHeaders", MetaHeaders},
		{"MetaAuthHeader", MetaAuthHeader},
		{"MetaName", MetaName},
		{"MetaCtxWindowPi", MetaCtxWindowPi},
		{"MetaMaxTokensPi", MetaMaxTokensPi},
		{"MetaLimit", MetaLimit},
		{"MetaOptions", MetaOptions},
		{"MetaVariants", MetaVariants},
		{"MetaContextSuffix", MetaContextSuffix},
		{"MetaExtraEnv", MetaExtraEnv},
	}
	for _, k := range keys {
		t.Run(k.name, func(t *testing.T) {
			if k.value == "" {
				t.Errorf("constant %s is empty", k.name)
			}
			if strings.TrimSpace(k.value) == "" {
				t.Errorf("constant %s is only whitespace", k.name)
			}
		})
	}
}

// ── BackupResult struct fields ────────────────────────────────────────────

func TestBackupResult_Fields(t *testing.T) {
	br := BackupResult{BackupPath: "/tmp/backup.json.12345678"}
	if br.BackupPath != "/tmp/backup.json.12345678" {
		t.Errorf("BackupResult.BackupPath: expected %q, got %q", "/tmp/backup.json.12345678", br.BackupPath)
	}
}

func TestBackupResult_ZeroValue(t *testing.T) {
	var br BackupResult
	if br.BackupPath != "" {
		t.Errorf("BackupResult.BackupPath zero value: expected empty, got %q", br.BackupPath)
	}
}
