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
)

const (
	fetchTimeout   = 5 * time.Second
	fetchUserAgent = "aimux/0.1.0"
)

// ProviderUseCases handles provider business logic.
type ProviderUseCases struct {
	providerRepo  domain.ProviderRepository
	multiplexRepo domain.MultiplexRepository
}

// NewProviderUseCases creates a new ProviderUseCases.
func NewProviderUseCases(providerRepo domain.ProviderRepository, multiplexRepo domain.MultiplexRepository) *ProviderUseCases {
	return &ProviderUseCases{
		providerRepo:  providerRepo,
		multiplexRepo: multiplexRepo,
	}
}

// Add creates a new provider and fetches its models.
func (uc *ProviderUseCases) Add(name, baseURL, discoveryURL, apiKey, authToken string, apiType domain.ApiType) (int64, error) {
	id, err := uc.providerRepo.Add(name, baseURL, discoveryURL, apiKey, authToken, apiType)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return 0, err
		}
		return 0, fmt.Errorf("add provider: %w", err)
	}

	// Trigger model fetch
	if fetchErr := uc.FetchModels(id, baseURL, discoveryURL, authToken, apiType); fetchErr != nil {
		// Fetch failure is non-fatal — provider is saved with error status
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

// Update updates a provider's credentials and API type.
func (uc *ProviderUseCases) Update(id int64, baseURL, discoveryURL, apiKey, authToken string, apiType domain.ApiType) error {
	if err := uc.providerRepo.Update(id, baseURL, discoveryURL, apiKey, authToken, apiType); err != nil {
		return err
	}
	// FetchModels -> InsertModels performs an atomic delete+insert inside a single
	// transaction, so we no longer pre-delete here. The old pre-delete left the
	// provider with an empty model list whenever the re-fetch failed.
	if fetchErr := uc.FetchModels(id, baseURL, discoveryURL, authToken, apiType); fetchErr != nil {
		_ = uc.providerRepo.UpdateStatus(id, "error")
		return fmt.Errorf("provider updated but model fetch failed: %w", fetchErr)
	}
	return nil
}

// Delete removes a provider by ID.
func (uc *ProviderUseCases) Delete(id int64) error {
	return uc.providerRepo.Delete(id)
}

// FetchModels fetches models from the provider's API, branching on apiType.
// Uses discoveryURL for model list endpoint when set, baseURL otherwise.
func (uc *ProviderUseCases) FetchModels(providerID int64, baseURL, discoveryURL, authToken string, apiType domain.ApiType) error {
	fetchURL := baseURL
	if discoveryURL != "" {
		fetchURL = discoveryURL
	}
	switch apiType {
	case domain.ApiTypeAnthropic:
		return uc.fetchAnthropicModels(providerID, fetchURL, authToken)
	case domain.ApiTypeGoogle:
		return uc.fetchGoogleModels(providerID, fetchURL, authToken)
	default:
		return uc.fetchOpenAIModels(providerID, fetchURL, authToken)
	}
}

func (uc *ProviderUseCases) fetchOpenAIModels(providerID int64, baseURL, authToken string) error {
	url := resolveBaseURL(baseURL) + "/v1/models"
	req, err := http.NewRequest("GET", url, nil)
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
	uc.saveModelMetadata(providerID, modelNames, body, domain.ApiTypeOpenAI)
	if err := uc.providerRepo.UpdateStatus(providerID, "active"); err != nil {
		return fmt.Errorf("update provider status: %w", err)
	}
	return nil
}

func (uc *ProviderUseCases) fetchAnthropicModels(providerID int64, baseURL, authToken string) error {
	url := resolveBaseURL(baseURL) + "/v1/models"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-api-key", authToken)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fetchUserAgent)

	modelNames, body, err := uc.doModelFetch(req)
	if err != nil {
		return err
	}

	if err := uc.providerRepo.InsertModels(providerID, modelNames); err != nil {
		return fmt.Errorf("store models: %w", err)
	}
	uc.saveModelMetadata(providerID, modelNames, body, domain.ApiTypeAnthropic)
	if err := uc.providerRepo.UpdateStatus(providerID, "active"); err != nil {
		return fmt.Errorf("update provider status: %w", err)
	}
	return nil
}

func (uc *ProviderUseCases) fetchGoogleModels(providerID int64, baseURL, authToken string) error {
	url := resolveBaseURL(baseURL) + "/v1beta/models?key=" + authToken
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fetchUserAgent)

	modelNames, body, err := uc.doModelFetch(req)
	if err != nil {
		return err
	}

	if err := uc.providerRepo.InsertModels(providerID, modelNames); err != nil {
		return fmt.Errorf("store models: %w", err)
	}
	uc.saveModelMetadata(providerID, modelNames, body, domain.ApiTypeGoogle)
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

	modelNames, err := parseModelsResponse(body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid response format: %w", err)
	}

	return modelNames, body, nil
}

// saveModelMetadata parses metadata from the API response body, enriches with the
// known model catalog, and stores it per model.
func (uc *ProviderUseCases) saveModelMetadata(providerID int64, modelNames []string, body []byte, apiType domain.ApiType) {
	for _, name := range modelNames {
		md := uc.parseModelCapabilities(name, body, apiType)
		if len(md) == 0 {
			continue // nothing to save
		}
		_ = uc.providerRepo.UpdateModelMetadata(providerID, name, md)
	}
}

// parseModelCapabilities extracts metadata for a specific model from the raw
// API response, then enriches with the known catalog.
// Strips Bifrost's Provider/ prefix before catalog lookup so
// "Opencode/deepseek-v4-flash" still finds "deepseek-v4-flash" in the catalog.
func (uc *ProviderUseCases) parseModelCapabilities(modelName string, body []byte, apiType domain.ApiType) domain.ModelMetadata {
	// Strip Bifrost's Provider/ prefix for catalog lookup
	baseName := config.StripProviderPrefix(modelName)
	md := config.LookupModelMetadata(baseName) // catalog first as base

	// Merge API-provided metadata on top
	switch apiType {
	case domain.ApiTypeGoogle:
		md = mergeMetadata(md, parseGoogleModelMetadata(baseName, body))
		// Anthropic and OpenAI APIs don't return per-model capabilities beyond the ID
	}

	return md
}

// googleModelsList represents a Gemini /v1beta/models response.
type googleModelsList struct {
	Models []googleModelEntry `json:"models"`
}

type googleModelEntry struct {
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	InputTokenLimit  int64  `json:"inputTokenLimit"`
	OutputTokenLimit int64  `json:"outputTokenLimit"`
}

// parseGoogleModelMetadata extracts capabilities from a Google API response.
func parseGoogleModelMetadata(modelName string, body []byte) domain.ModelMetadata {
	var list googleModelsList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil
	}
	for _, m := range list.Models {
		// Google returns model names like "models/gemini-2.5-pro" — strip prefix.
		id := strings.TrimPrefix(m.Name, "models/")
		if id == modelName || strings.EqualFold(id, modelName) {
			md := domain.ModelMetadata{}
			if m.InputTokenLimit > 0 {
				md["context_window"] = m.InputTokenLimit
			}
			if m.OutputTokenLimit > 0 {
				md["max_tokens"] = m.OutputTokenLimit
			}
			return md
		}
	}
	return nil
}

// mergeMetadata merges apiMeta on top of base, returning a new map.
func mergeMetadata(base, apiMeta domain.ModelMetadata) domain.ModelMetadata {
	result := make(domain.ModelMetadata, len(base)+len(apiMeta))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range apiMeta {
		result[k] = v
	}
	return result
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

	switch provider.ApiType {
	case domain.ApiTypeAnthropic:
		req.Header.Set("x-api-key", provider.AuthToken)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+provider.AuthToken)
	}
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

	if err := uc.FetchModels(providerID, provider.BaseURL, provider.DiscoveryURL, provider.AuthToken, provider.ApiType); err != nil {
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
