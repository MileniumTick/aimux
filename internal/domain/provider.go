package domain

// Provider represents a provider row in the database.
type Provider struct {
	ID                   int64
	Name                 string
	BaseURL              string
	DiscoveryURL         string // optional, separate URL for model discovery; empty = use BaseURL
	DefaultContextWindow int64  // fallback context window when API/catalog don't provide it; 0 = not set
	LogoURL              string // URL to provider logo, e.g. https://models.dev/logos/anthropic.svg
	APIKey               string
	AuthToken            string
	Status               string
	CreatedAt            string
	UpdatedAt            string
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
// Known keys — exported for reference by mutators and catalog:
const (
	MetaContextWindow    = "context_window"     // int64 — max input tokens
	MetaMaxTokens        = "max_tokens"         // int64 — max output tokens
	MetaReasoning        = "reasoning"          // bool — supports extended thinking
	MetaInputModalities  = "input_modalities"   // []string — e.g. ["text"], ["text","image"]
	MetaCost             = "cost"               // map[string]float64 — {input, output, cacheRead, cacheWrite}
	MetaCompat           = "compat"             // map[string]any — provider/model compat overrides
	MetaThinkingLevelMap = "thinking_level_map" // map[string]any — pi thinking level mapping
	MetaHeaders          = "headers"            // map[string]string — custom HTTP headers
	MetaAuthHeader       = "auth_header"        // bool — add Authorization header automatically
	MetaName             = "name"               // string — human-readable model label

	// Pi-specific camelCase aliases (read by pi-dual-json mutator)
	MetaCtxWindowPi = "contextWindow" // int64 — pi camelCase context window
	MetaMaxTokensPi = "maxTokens"     // int64 — pi camelCase max tokens

	// OpenCode-specific
	MetaLimit    = "limit"    // map[string]any — {context, output} token limits
	MetaOptions  = "options"  // map[string]any — model options (thinking, reasoningEffort, etc.)
	MetaVariants = "variants" // map[string]any — model variants for OpenCode

	// Anthropic/Claude Code suffixes for context window indication
	MetaContextSuffix = "context_suffix" // string — e.g. "[1m]", "[200k]", "[32k]"

	// Claude Code extra env vars
	MetaExtraEnv = "extra_env" // map[string]string — extra env vars like CLAUDE_CODE_EFFORT_LEVEL
)

type ModelMetadata map[string]any

// ProviderRepository defines the interface for provider persistence.
type ProviderRepository interface {
	Add(name, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) (int64, error)
	Get(id int64) (Provider, error)
	List() ([]Provider, error)
	Update(id int64, baseURL, discoveryURL, apiKey, authToken string, defaultContextWindow ...int64) error
	UpdateStatus(id int64, status string) error
	Delete(id int64) error
	InsertModels(providerID int64, modelNames []string) error
	AddCustomModels(providerID int64, modelNames []string) error
	DeleteModelsByProvider(providerID int64) error
	ListModels(providerID int64) ([]ProviderModel, error)
	ListAllModels() ([]ProviderModel, error)
	UpdateModelMetadata(providerID int64, modelName string, metadata ModelMetadata) error
}
