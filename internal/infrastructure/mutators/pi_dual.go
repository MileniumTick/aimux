package mutators

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// PiDualJSON mutates Pi's single models.json config file.
// Pi keeps providers, credentials, and models together in one file.
// Registered as: "pi-dual-json"
type PiDualJSON struct{}

// Mutate reads models.json and writes/updates a provider entry with the
// correct schema: baseUrl, apiKey, api, and models as an array of {id, name}.
func (m *PiDualJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	providerID, ok := mutatorConfig["provider_id"].(string)
	if !ok || providerID == "" {
		return nil, fmt.Errorf("pi mutator requires provider_id in mutator_config")
	}

	// Derive pi api value from provider.ApiType. mutator_config "api" overrides.
	apiVal := piAPIFromProvider(provider)
	if cfgAPI, ok := mutatorConfig["api"].(string); ok && cfgAPI != "" {
		apiVal = cfgAPI
	}

	// Resolve models.json path
	piDir := configPath
	if filepath.Ext(configPath) != "" {
		piDir = filepath.Dir(configPath)
	}

	modelsPath := filepath.Join(piDir, "models.json")
	if mp, ok := mutatorConfig["models_path"].(string); ok && mp != "" {
		modelsPath = mp
	}

	if err := config.PrepareDir(modelsPath); err != nil {
		return nil, err
	}

	// Backup
	var backupResult *domain.BackupResult
	if fi, err := os.Stat(modelsPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(modelsPath)
		if err != nil {
			return nil, err
		}
		backupResult = &domain.BackupResult{BackupPath: bp}
	}

	root, err := config.ReadJSONWithLock(modelsPath)
	if err != nil {
		return nil, err
	}

	// Ensure providers key exists
	providers, ok := root["providers"].(map[string]any)
	if !ok {
		providers = make(map[string]any)
		root["providers"] = providers
	}

	// Build models array with metadata from catalog/API.
	// If _registered_models is provided, use that list (free selection);
	// otherwise fall back to mapped model values (backward compatible).
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	modelList := make([]string, 0)
	if registered, ok := mutatorConfig["_registered_models"].([]any); ok && len(registered) > 0 {
		for _, r := range registered {
			if name, ok := r.(string); ok && name != "" {
				modelList = append(modelList, name)
			}
		}
	}
	if len(modelList) == 0 {
		for _, modelName := range modelMappings {
			if modelName != "" {
				modelList = append(modelList, modelName)
			}
		}
	}

	modelsArr := make([]any, 0, len(modelList))
	for _, modelName := range modelList {
		if modelName == "" {
			continue
		}
		entry := map[string]any{
			"id":   modelName,
			"name": modelName,
		}
		// Enrich from catalog/API metadata if available
		if modelMeta != nil {
			if md, ok := modelMeta[modelName].(map[string]any); ok {
				if cw, ok := md["context_window"]; ok {
					entry["context_window"] = cw
				}
				if mt, ok := md["max_tokens"]; ok {
					entry["max_tokens"] = mt
				}
				if r, ok := md["reasoning"]; ok {
					entry["reasoning"] = r
				}
				if im, ok := md["input_modalities"]; ok {
					entry["input"] = im
				}
			}
		}
		modelsArr = append(modelsArr, entry)
	}

	providerEntry := map[string]any{
		"baseUrl": provider.BaseURL,
		"apiKey":  provider.APIKey,
		"api":     apiVal,
		"models":  modelsArr,
	}

	providers[providerID] = providerEntry

	if err := config.WriteAtomicJSON(modelsPath, root); err != nil {
		return nil, err
	}
	config.PruneBackups(modelsPath, 5)

	return backupResult, nil
}

// piAPIFromProvider maps domain.ApiType to pi's api field value.
// ponytail: simple mapping, safe "openai-completions" fallback for unknown types.
func piAPIFromProvider(p domain.Provider) string {
	switch p.ApiType {
	case domain.ApiTypeAnthropic:
		return "anthropic-messages"
	case domain.ApiTypeGoogle:
		return "google-generative-ai"
	case domain.ApiTypeOpenAI:
		return "openai-completions"
	default:
		return "openai-completions"
	}
}
