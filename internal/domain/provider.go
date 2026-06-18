package domain

// ApiType represents the provider's API type for model discovery.
type ApiType string

const (
	ApiTypeOpenAI    ApiType = "openai"
	ApiTypeAnthropic ApiType = "anthropic"
	ApiTypeGoogle    ApiType = "google"
)

// Provider represents a provider row in the database.
type Provider struct {
	ID           int64
	Name         string
	BaseURL      string
	DiscoveryURL string // optional, separate URL for model discovery; empty = use BaseURL
	APIKey       string
	AuthToken    string
	ApiType      ApiType
	Status       string
	CreatedAt    string
	UpdatedAt    string
}

// ProviderModel represents a model row associated with a provider.
type ProviderModel struct {
	ID           int64
	ProviderID   int64
	ModelName    string
	ProviderName string        // populated by joins, empty otherwise
	Metadata     ModelMetadata // flexible JSON with model capabilities
}

// ModelMetadata holds model capabilities as a flexible JSON object.
// Known keys: context_window, max_tokens, reasoning, thinking_level_map,
// cost, compat, input_modalities.
type ModelMetadata map[string]any

// ProviderRepository defines the interface for provider persistence.
type ProviderRepository interface {
	Add(name, baseURL, discoveryURL, apiKey, authToken string, apiType ApiType) (int64, error)
	Get(id int64) (Provider, error)
	List() ([]Provider, error)
	Update(id int64, baseURL, discoveryURL, apiKey, authToken string, apiType ApiType) error
	UpdateStatus(id int64, status string) error
	Delete(id int64) error
	InsertModels(providerID int64, modelNames []string) error
	DeleteModelsByProvider(providerID int64) error
	ListModels(providerID int64) ([]ProviderModel, error)
	ListAllModels() ([]ProviderModel, error)
	UpdateModelMetadata(providerID int64, modelName string, metadata ModelMetadata) error
}
