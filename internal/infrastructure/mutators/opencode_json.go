package mutators

import (
	"fmt"
	"os"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
)

// OpenCodeProviderJSON mutates OpenCode's opencode.json by building a provider
// entry under the provider key with npm package name, base URL, API key, custom
// headers, and models with full options (reasoningEffort, thinking, limit, etc.).
// Registered as: "opencode-provider-json"
type OpenCodeProviderJSON struct{}

// Mutate reads the JSON config, builds a provider entry with models, and
// writes atomically with backup. Uses model metadata to populate:
//   - Provider-level: npm, name, options.baseURL, options.headers, options.apiKey
//   - Model-level: name, limit.{context,output}, options.{reasoningEffort, thinking, ...}
func (m *OpenCodeProviderJSON) Mutate(
	configPath string,
	modelMappings map[string]string,
	provider domain.Provider,
	mutatorConfig map[string]any,
) (*domain.BackupResult, error) {
	providerID, ok := mutatorConfig["provider_id"].(string)
	if !ok || providerID == "" {
		return nil, fmt.Errorf("opencode mutator requires provider_id in mutator_config")
	}

	if err := config.PrepareDir(configPath); err != nil {
		return nil, err
	}

	// Create backup BEFORE reading — file existence on disk, not parse success
	var backupResult *domain.BackupResult
	if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
		bp, err := config.CreateBackup(configPath)
		if err != nil {
			return nil, err
		}
		backupResult = &domain.BackupResult{BackupPath: bp}
	}

	root, err := config.ReadJSONWithLock(configPath)
	if err != nil {
		return nil, err
	}

	// Ensure provider key exists
	providers, ok := root["provider"].(map[string]any)
	if !ok {
		providers = make(map[string]any)
		root["provider"] = providers
	}

	// When _clear_providers is set, clear the provider map before adding entries.
	// This ensures deleted bindings don't leave stale entries in the config.
	if clear, _ := mutatorConfig["_clear_providers"].(bool); clear {
		providers = make(map[string]any)
		root["provider"] = providers
	}

	// Build model list from _registered_models or modelMappings
	modelMeta, _ := mutatorConfig["_model_metadata"].(map[string]any)
	modelList := buildModelList(mutatorConfig, modelMappings)

	// Build models map with enriched options
	models := make(map[string]any, len(modelList))
	for _, name := range modelList {
		entry := map[string]any{"name": name}
		if md, ok := modelMeta[name].(map[string]any); ok {
			fillOpenCodeModelEntry(entry, md)
		}
		models[name] = entry
	}

	// Build provider entry
	providerEntry := map[string]any{
		"name": provider.Name,
		"options": map[string]any{
			"baseURL": provider.BaseURL,
			"apiKey":  provider.APIKey,
		},
		"models": models,
	}

	// NPM package: derive from provider API type, allow mutatorConfig override
	npm := openCodeNPMForProvider(provider.ApiType)
	if override, ok := mutatorConfig["npm"].(string); ok && override != "" {
		npm = override
	}
	if npm != "" {
		providerEntry["npm"] = npm
	}

	// Custom headers
	if headers := extractProviderHeaders(mutatorConfig); len(headers) > 0 {
		opts := providerEntry["options"].(map[string]any)
		opts["headers"] = headers
	}

	providers[providerID] = providerEntry

	if err := config.WriteAtomicJSON(configPath, root); err != nil {
		return nil, err
	}

	// Clean up old backups
	config.PruneBackups(configPath, 5)

	return backupResult, nil
}

// fillOpenCodeModelEntry populates an OpenCode model entry with metadata-derived fields.
// ponytail: known OpenCode model fields; add new fields here as they appear in upstream docs.
func fillOpenCodeModelEntry(entry map[string]any, md map[string]any) {
	copyField(entry, md, "name")

	// Build options sub-object
	opts := make(map[string]any)

	// OpenCode thinking config: {type: "enabled", budgetTokens: N} or
	// reasoningEffort: "high"|"medium"|"low"
	if reasoningVal, ok := md[domain.MetaReasoning].(bool); ok && reasoningVal {
		// For OpenAI-compatible models, use reasoningEffort
		opts["reasoningEffort"] = "medium"
	}

	// Copy known OpenCode options from metadata
	if ro, ok := md["reasoningEffort"]; ok {
		opts["reasoningEffort"] = ro
	}
	if tv, ok := md["textVerbosity"]; ok {
		opts["textVerbosity"] = tv
	}
	if rs, ok := md["reasoningSummary"]; ok {
		opts["reasoningSummary"] = rs
	}
	if incl, ok := md["include"]; ok {
		opts["include"] = incl
	}

	if len(opts) > 0 {
		entry["options"] = opts
	}

	// Build limit sub-object
	limit := make(map[string]any)
	if cw, ok := md[domain.MetaContextWindow]; ok {
		limit["context"] = cw
	}
	if mt, ok := md[domain.MetaMaxTokens]; ok {
		limit["output"] = mt
	}
	if len(limit) > 0 {
		entry["limit"] = limit
	}

	// Variants from metadata
	if vars, ok := md[domain.MetaVariants]; ok {
		entry["variants"] = vars
	}
}

// openCodeNPMForProvider maps domain API types to OpenCode npm packages.
func openCodeNPMForProvider(apiType domain.ApiType) string {
	switch apiType {
	case domain.ApiTypeAnthropic:
		return "@ai-sdk/anthropic"
	case domain.ApiTypeGoogle:
		return "@ai-sdk/google"
	case domain.ApiTypeOpenAI:
		return "@ai-sdk/openai-compatible"
	default:
		return "@ai-sdk/openai-compatible"
	}
}
