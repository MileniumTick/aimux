package application

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
)

const (
	fetchTimeout   = 5 * time.Second
	fetchUserAgent = "aimux/0.1.0"
)

// ProviderUseCases handles provider business logic.
type ProviderUseCases struct {
	providerRepo  *sqlite.ProviderRepository
	multiplexRepo *sqlite.MultiplexRepository
}

// NewProviderUseCases creates a new ProviderUseCases.
func NewProviderUseCases(providerRepo *sqlite.ProviderRepository, multiplexRepo *sqlite.MultiplexRepository) *ProviderUseCases {
	return &ProviderUseCases{
		providerRepo:  providerRepo,
		multiplexRepo: multiplexRepo,
	}
}

// Add creates a new provider and fetches its models.
// customModels are always inserted (supplement fetched models or serve as fallback).
func (uc *ProviderUseCases) Add(name, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) (int64, error) {
	id, err := uc.providerRepo.Add(name, baseURL, discoveryURL, apiKey, authToken, defaultContextWindow...)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return 0, err
		}
		return 0, fmt.Errorf("add provider: %w", err)
	}

	// Trigger model fetch
	fetchErr := uc.FetchModels(id, baseURL, discoveryURL, authToken)

	if fetchErr != nil {
		_ = uc.providerRepo.UpdateStatus(id, "error")
		return id, fmt.Errorf("provider created but model fetch failed: %w", fetchErr)
	}

	return id, nil
}

// List returns all providers.
func (uc *ProviderUseCases) List() ([]domain.Provider, error) {
	return uc.providerRepo.List()
}

// Get returns a single provider by ID.
func (uc *ProviderUseCases) Get(id int64) (domain.Provider, error) {
	return uc.providerRepo.Get(id)
}

// Update updates a provider's credentials.
func (uc *ProviderUseCases) Update(id int64, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) error {
	if err := uc.providerRepo.Update(id, baseURL, discoveryURL, apiKey, authToken, defaultContextWindow...); err != nil {
		return err
	}
	// FetchModels -> InsertModels performs an atomic delete+insert inside a single
	// transaction, so we no longer pre-delete here. The old pre-delete left the
	// provider with an empty model list whenever the re-fetch failed.
	if fetchErr := uc.FetchModels(id, baseURL, discoveryURL, authToken); fetchErr != nil {
		_ = uc.providerRepo.UpdateStatus(id, "error")
		return fmt.Errorf("provider updated but model fetch failed: %w", fetchErr)
	}
	return nil
}

// Delete removes a provider by ID.
func (uc *ProviderUseCases) Delete(id int64) error {
	return uc.providerRepo.Delete(id)
}

// AddWithCustomModels creates a provider and inserts custom fallback models
// regardless of whether the API model fetch succeeds or fails.
// customModels are comma-separated model IDs.
func (uc *ProviderUseCases) AddWithCustomModels(name, baseURL, discoveryURL, apiKey, authToken string, customModels string, defaultContextWindow ...int64) (int64, error) {
	id, err := uc.providerRepo.Add(name, baseURL, discoveryURL, apiKey, authToken, defaultContextWindow...)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return 0, err
		}
		return 0, fmt.Errorf("add provider: %w", err)
	}

	// Parse custom models
	var fallbackNames []string
	if customModels != "" {
		for _, p := range strings.Split(customModels, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				fallbackNames = append(fallbackNames, p)
			}
		}
	}

	// Try API fetch
	fetchErr := uc.FetchModels(id, baseURL, discoveryURL, authToken)

	// Always insert custom models (supplement fetched or serve as fallback)
	if len(fallbackNames) > 0 {
		if cmErr := uc.providerRepo.AddCustomModels(id, fallbackNames); cmErr != nil {
			// Non-fatal
			_ = cmErr
		}
	}

	if fetchErr != nil {
		if len(fallbackNames) > 0 {
			// Has custom models — keep active
			_ = uc.providerRepo.UpdateStatus(id, "active")
			return id, fmt.Errorf("provider created with fallback models, but model fetch failed: %w", fetchErr)
		}
		_ = uc.providerRepo.UpdateStatus(id, "error")
		return id, fmt.Errorf("provider created but model fetch failed: %w", fetchErr)
	}

	return id, nil
}

// AddCustomModels inserts custom model names for an existing provider.
func (uc *ProviderUseCases) AddCustomModels(providerID int64, modelNames []string) error {
	if len(modelNames) == 0 {
		return nil
	}
	if err := uc.providerRepo.AddCustomModels(providerID, modelNames); err != nil {
		return fmt.Errorf("add custom models: %w", err)
	}
	// If provider was in error state and now has models, reactivate
	provider, err := uc.providerRepo.Get(providerID)
	if err == nil && provider.Status == "error" {
		// Check if it has any models now
		models, listErr := uc.providerRepo.ListModels(providerID)
		if listErr == nil && len(models) > 0 {
			_ = uc.providerRepo.UpdateStatus(providerID, "active")
		}
	}
	return nil
}

// FetchModels fetches models from the provider's API.
// discoveryURL is the full URL to the models endpoint (no path appended).
// Without discoveryURL, /v1/models is appended to baseURL.
func (uc *ProviderUseCases) FetchModels(providerID int64, baseURL, discoveryURL, authToken string) error {
	fetchURL := baseURL
	if discoveryURL != "" {
		fetchURL = discoveryURL
	} else {
		fetchURL = resolveBaseURL(baseURL) + "/v1/models"
	}

	req, err := http.NewRequest("GET", fetchURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fetchUserAgent)

	modelNames, body, err := uc.doModelFetch(req)
	if err != nil {
		return err
	}

	if err := uc.providerRepo.InsertModels(providerID, modelNames); err != nil {
		return fmt.Errorf("store models: %w", err)
	}
	uc.saveModelMetadata(providerID, modelNames, body)
	if err := uc.providerRepo.UpdateStatus(providerID, "active"); err != nil {
		return fmt.Errorf("update provider status: %w", err)
	}
	return nil
}

// doModelFetch executes the HTTP request and returns model names and raw response body.
func (uc *ProviderUseCases) doModelFetch(req *http.Request) ([]string, []byte, error) {
	client := &http.Client{Timeout: fetchTimeout}

	resp, err := client.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "Timeout") {
			return nil, nil, fmt.Errorf("request timed out after %d seconds", int(fetchTimeout.Seconds()))
		}
		return nil, nil, fmt.Errorf("network error: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// continue
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, nil, fmt.Errorf("authentication failed: check auth token")
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			return nil, nil, fmt.Errorf("rate limited by provider, retry after %s seconds", retryAfter)
		}
		return nil, nil, fmt.Errorf("rate limited by provider")
	default:
		if resp.StatusCode >= 500 {
			return nil, nil, fmt.Errorf("provider returned server error: %d", resp.StatusCode)
		}
		return nil, nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}

	// If response is HTML (starts with <), the URL is probably wrong
	if len(body) > 0 && body[0] == '<' {
		return nil, nil, fmt.Errorf("endpoint returned HTML instead of JSON — check the URL")
	}

	modelNames, err := parseModelsResponse(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid response format: %w", err)
	}

	return modelNames, body, nil
}

// saveModelMetadata stores catalog metadata for each model.
// Strips Bifrost's Provider/ prefix before catalog lookup so
// "Opencode/deepseek-v4-flash" still finds "deepseek-v4-flash" in the catalog.
// If the catalog has no entry and the provider has default_context_window set,
// uses that as fallback so the CLI doesn't get an empty context window.
func (uc *ProviderUseCases) saveModelMetadata(providerID int64, modelNames []string, body []byte) {
	// Read provider's default context window (0 = not set)
	p, err := uc.providerRepo.Get(providerID)
	dcw := int64(0)
	if err == nil {
		dcw = p.DefaultContextWindow
	}

	for _, name := range modelNames {
		baseName := config.StripProviderPrefix(name)
		md := config.LookupModelMetadata(baseName)
		if len(md) > 0 {
			_ = uc.providerRepo.UpdateModelMetadata(providerID, name, md)
		} else if dcw > 0 {
			// Fallback: provider default context window
			meta := domain.ModelMetadata{
				domain.MetaContextWindow: dcw,
				domain.MetaContextSuffix: config.ContextSuffixForWindow(dcw),
			}
			_ = uc.providerRepo.UpdateModelMetadata(providerID, name, meta)
		}
	}
}

// resolveBaseURL normalizes a base URL.
func resolveBaseURL(baseURL string) string {
	url := strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return url
}

// FetchDiff describes model changes after a fetch operation.
type FetchDiff struct {
	Added   int
	Removed int
	Total   int
	Error   string
}

// TestConnectivity pings the provider's /v1/models endpoint without storing results.
func (uc *ProviderUseCases) TestConnectivity(providerID int64) error {
	provider, err := uc.providerRepo.Get(providerID)
	if err != nil {
		return fmt.Errorf("get provider: %w", err)
	}

	url := resolveBaseURL(provider.BaseURL) + "/v1/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+provider.AuthToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fetchUserAgent)

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connectivity test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			return fmt.Errorf("rate limited by provider, retry after %s seconds", retryAfter)
		}
		return fmt.Errorf("rate limited by provider")
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("authentication failed: check auth token")
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("provider returned server error: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	names, err := parseModelsResponse(body)
	if err != nil {
		return fmt.Errorf("connectivity test: %w", err)
	}
	if len(names) == 0 {
		return fmt.Errorf("connectivity test returned no models")
	}
	return nil
}

// RetryFetch re-fetches models for a provider using stored credentials and returns a diff.
func (uc *ProviderUseCases) RetryFetch(providerID int64) (*FetchDiff, error) {
	provider, err := uc.providerRepo.Get(providerID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Get old model count for diff
	oldModels, _ := uc.providerRepo.ListModels(providerID)
	oldSet := make(map[string]bool, len(oldModels))
	for _, m := range oldModels {
		oldSet[m.ModelName] = true
	}

	if err := uc.FetchModels(providerID, provider.BaseURL, provider.DiscoveryURL, provider.AuthToken); err != nil {
		if strings.Contains(err.Error(), "rate limited") {
			return &FetchDiff{Error: err.Error()}, nil
		}
		return nil, err
	}

	// Get new models and compute diff
	newModels, _ := uc.providerRepo.ListModels(providerID)
	diff := &FetchDiff{Total: len(newModels)}
	for _, m := range newModels {
		if !oldSet[m.ModelName] {
			diff.Added++
		}
	}
	// Removed = old count that aren't in new set
	newSet := make(map[string]bool, len(newModels))
	for _, m := range newModels {
		newSet[m.ModelName] = true
	}
	for _, m := range oldModels {
		if !newSet[m.ModelName] {
			diff.Removed++
		}
	}

	return diff, nil
}

// modelsResponse represents the OpenAI-compatible /v1/models response.
type modelsResponse struct {
	Data   []modelEntry `json:"data,omitempty"`
	Models []modelEntry `json:"models,omitempty"`
}

type modelEntry struct {
	ID string `json:"id"`
}

func parseModelsResponse(body []byte) ([]string, error) {
	var resp modelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	if len(resp.Data) > 0 {
		names := make([]string, 0, len(resp.Data))
		for _, m := range resp.Data {
			if m.ID != "" {
				names = append(names, m.ID)
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no model IDs found in response data array")
		}
		return names, nil
	}

	if len(resp.Models) > 0 {
		names := make([]string, 0, len(resp.Models))
		for _, m := range resp.Models {
			if m.ID != "" {
				names = append(names, m.ID)
			}
		}
		if len(names) == 0 {
			return nil, fmt.Errorf("no model IDs found in response models array")
		}
		return names, nil
	}

	return nil, fmt.Errorf("no recognizable model list in response (expected 'data' or 'models' array)")
}
