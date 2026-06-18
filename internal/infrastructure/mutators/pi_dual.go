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
// full pi schema: baseUrl, apiKey, api, authHeader, headers, compat, and
// models as an array with all supported fields: id, name, contextWindow,
// maxTokens, reasoning, input, cost, compat, thinkingLevelMap, api.
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

	// When _clear_providers is set, clear the providers map before adding entries.
	// This ensures deleted bindings don't leave stale entries in the config.
	if clear, _ := mutatorConfig["_clear_providers"].(bool); clear {
		providers = make(map[string]any)
		root["providers"] = providers
	}

	// Build model list from _registered_models or modelMappings
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	modelList := buildModelList(mutatorConfig, modelMappings)

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
		if md, ok := modelMeta[modelName].(map[string]any); ok {
			fillPiModelEntry(entry, md)
		}
		modelsArr = append(modelsArr, entry)
	}

	// Build provider entry with all supported pi fields
	providerEntry := map[string]any{
		"baseUrl": provider.BaseURL,
		"apiKey":  provider.APIKey,
		"api":     apiVal,
		"models":  modelsArr,
	}

	// Provider-level compat from metadata or mutatorConfig
	providerCompat := extractProviderCompat(modelMeta, modelList, mutatorConfig)
	if len(providerCompat) > 0 {
		providerEntry["compat"] = providerCompat
	}

	// Provider-level headers
	if headers := extractProviderHeaders(mutatorConfig); len(headers) > 0 {
		providerEntry["headers"] = headers
	}

	// authHeader
	if ah, ok := mutatorConfig["auth_header"].(bool); ok {
		providerEntry["authHeader"] = ah
	}

	providers[providerID] = providerEntry

	if err := config.WriteAtomicJSON(modelsPath, root); err != nil {
		return nil, err
	}
	config.PruneBackups(modelsPath, 5)

	return backupResult, nil
}

// fillPiModelEntry copies known pi model fields from metadata into the entry.
// ponytail: known fields map; add new pi fields here as they appear.
func fillPiModelEntry(entry map[string]any, md map[string]any) {
	// Simple scalar fields
	copyField(entry, md, "name")
	copyField(entry, md, "reasoning")
	copyField(entry, md, "api")

	// Context window: prefer pi camelCase, fall back to snake_case
	if v, ok := md[domain.MetaCtxWindowPi]; ok {
		entry["contextWindow"] = v
	} else if v, ok := md[domain.MetaContextWindow]; ok {
		entry["contextWindow"] = v
	}

	// Max tokens: prefer pi camelCase
	if v, ok := md[domain.MetaMaxTokensPi]; ok {
		entry["maxTokens"] = v
	} else if v, ok := md[domain.MetaMaxTokens]; ok {
		entry["maxTokens"] = v
	}

	// Input modalities: pi uses "input" key
	if v, ok := md[domain.MetaInputModalities]; ok {
		entry["input"] = v
	}

	// Cost: pi expects {"input": N, "output": N, "cacheRead": N, "cacheWrite": N}
	if v, ok := md[domain.MetaCost]; ok {
		entry["cost"] = v
	}

	// Compat: pi supports per-model compat overrides
	if v, ok := md[domain.MetaCompat]; ok {
		entry["compat"] = v
	}

	// Thinking level map
	if v, ok := md[domain.MetaThinkingLevelMap]; ok {
		entry["thinkingLevelMap"] = v
	}
}

// copyField copies a field from src to dst if it exists in src.
func copyField(dst, src map[string]any, key string) {
	if v, ok := src[key]; ok {
		dst[key] = v
	}
}

// extractProviderCompat computes provider-level compat from model metadata.
// If all models share the same compat, promote it to provider level.
// If mutatorConfig provides explicit compat, use that instead.
// ponytail: simple majority promotion; complex per-model compat stays per-model.
func extractProviderCompat(modelMeta map[string]any, modelList []string, mutatorConfig map[string]any) map[string]any {
	// Explicit compat from mutatorConfig wins
	if cfg, ok := mutatorConfig["compat"].(map[string]any); ok && len(cfg) > 0 {
		return cfg
	}
	if len(modelList) == 0 || len(modelMeta) == 0 {
		return nil
	}
	// Promote shared compat fields from model metadata
	var shared map[string]any
	for _, name := range modelList {
		md, ok := modelMeta[name].(map[string]any)
		if !ok {
			return nil // can't promote if any model lacks metadata
		}
		c, ok := md[domain.MetaCompat].(map[string]any)
		if !ok {
			return nil // can't promote if any model lacks compat
		}
		if shared == nil {
			shared = c
		} else {
			// Check all models share same compat
			for k := range shared {
				if c[k] != shared[k] {
					return nil // mismatch, keep compat per-model
				}
			}
		}
	}
	return shared
}

// extractProviderHeaders extracts custom headers from mutatorConfig.
func extractProviderHeaders(mutatorConfig map[string]any) map[string]any {
	if h, ok := mutatorConfig["headers"].(map[string]any); ok && len(h) > 0 {
		return h
	}
	return nil
}

// buildModelList extracts model names from _registered_models or modelMappings.
// _registered_models can be []string (from Apply) or []any (from direct calls).
func buildModelList(mutatorConfig map[string]any, modelMappings map[string]string) []string {
	modelList := make([]string, 0)
	if registered, ok := mutatorConfig["_registered_models"].([]any); ok && len(registered) > 0 {
		for _, r := range registered {
			if name, ok := r.(string); ok && name != "" {
				modelList = append(modelList, name)
			}
		}
	} else if registered, ok := mutatorConfig["_registered_models"].([]string); ok && len(registered) > 0 {
		modelList = append(modelList, registered...)
	}
	if len(modelList) == 0 {
		for _, modelName := range modelMappings {
			if modelName != "" {
				modelList = append(modelList, modelName)
			}
		}
	}
	return modelList
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
