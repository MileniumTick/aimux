package tui

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/charmbracelet/huh"
)

// AddProviderResult holds the values submitted from the Add Provider form.
type AddProviderResult struct {
	Name         string
	BaseURL      string
	DiscoveryURL string // optional, for model discovery
	APIKey       string
	AuthToken    string
	ApiType      string
}

// NewAddProviderForm creates a form for adding a new provider.
func NewAddProviderForm(result *AddProviderResult) *huh.Form {
	apiTypeOpts := []huh.Option[string]{
		huh.NewOption("OpenAI / OpenAI-compatible", "openai"),
		huh.NewOption("Anthropic", "anthropic"),
		huh.NewOption("Google AI (Gemini)", "google"),
	}
	result.ApiType = "openai"

	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Name").
				Placeholder("My OpenAI").
				Value(&result.Name).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("name is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Base URL").
				Placeholder("https://api.openai.com/v1").
				Value(&result.BaseURL).
				Validate(func(s string) error {
					u, err := url.ParseRequestURI(s)
					if err != nil {
						return fmt.Errorf("must be a valid URL")
					}
					if u.Scheme == "" || u.Host == "" {
						return fmt.Errorf("URL must include scheme and host")
					}
					return nil
				}),
			huh.NewInput().
				Title("API Key").
				Value(&result.APIKey).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Auth Token").
				Description("Optional if same as API Key").
				Value(&result.AuthToken).
				EchoMode(huh.EchoModePassword),
			huh.NewInput().
				Title("Discovery URL (optional)").
				Description("Separate URL for model discovery. Leave empty to use Base URL.").
				Placeholder("https://api.bifrost.local/v1").
				Value(&result.DiscoveryURL),
			huh.NewSelect[string]().
				Title("API Type").
				Description("Model discovery method").
				Options(apiTypeOpts...).
				Value(&result.ApiType),
		),
	).WithHeight(11)
}

// NewDeleteConfirmForm creates a confirmation dialog for deleting a provider.
func NewDeleteConfirmForm(name string, result *bool) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete %s?", name)).
				Description("This will remove all associated models and active mappings.").
				Affirmative("Yes").
				Negative("No").
				Value(result),
		),
	)
}

// NewSelectTargetCLIForm creates a form to select a target CLI.
func NewSelectTargetCLIForm(clis []domain.TargetCLI, result *int64) *huh.Form {
	opts := make([]huh.Option[int64], len(clis))
	for i, c := range clis {
		opts[i] = huh.NewOption(c.Name, c.ID)
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int64]().
				Title("Select Target CLI").
				Filtering(true).
				Options(opts...).
				Value(result),
		),
	)
}

// NewSelectProviderForm creates a form to select a provider.
// All providers are shown, with status label for non-active ones.
func NewSelectProviderForm(providers []domain.Provider, result *int64) *huh.Form {
	return newSelectProviderForm("Select Provider", providers, result)
}

// NewSelectProviderToRemoveForm creates a form to select a provider to remove.
func NewSelectProviderToRemoveForm(providers []domain.Provider, result *int64) *huh.Form {
	return newSelectProviderForm("Select provider to remove", providers, result)
}

func newSelectProviderForm(title string, providers []domain.Provider, result *int64) *huh.Form {
	opts := make([]huh.Option[int64], 0, len(providers))
	for _, p := range providers {
		label := p.Name
		if p.Status == "error" {
			label = p.Name + " [ERROR]"
		}
		opts = append(opts, huh.NewOption(label, p.ID))
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int64]().
				Title(title).
				Filtering(true).
				Options(opts...).
				Value(result),
		),
	)
}

// MapModelsResult holds the result of the model mapping form.
type MapModelsResult struct {
	Mappings map[string]string
}

// RegisterModelsResult holds the result of the model registration form.
type RegisterModelsResult struct {
	RegisteredModels []string
}

// NewRegisterModelsForm creates a multi-select form for choosing which models
// to register in the target CLI's config file. Pre-selects the currently mapped
// models as defaults.
func NewRegisterModelsForm(models []domain.ProviderModel, preSelected map[string]bool, result *RegisterModelsResult) *huh.Form {
	opts := make([]huh.Option[string], 0, len(models))
	for _, m := range models {
		opts = append(opts, huh.NewOption(m.ModelName, m.ModelName))
	}
	// Pre-populate with defaults so user can press Enter immediately
	result.RegisteredModels = make([]string, 0, len(models))
	for _, m := range models {
		if preSelected[m.ModelName] {
			result.RegisteredModels = append(result.RegisteredModels, m.ModelName)
		}
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Register Models").
				Description("Select which models to include in the config file.").
				Options(opts...).
				Value(&result.RegisteredModels),
		),
	).WithHeight(10)
}

// NewSelectModelsForm creates a multi-select to pick models for a CLI.
// All models are pre-selected by default. Uses RegisterModelsResult as bridge.
func NewSelectModelsForm(models []domain.ProviderModel, result *RegisterModelsResult) *huh.Form {
	preselected := make(map[string]bool, len(models))
	for _, m := range models {
		preselected[m.ModelName] = true
	}
	return NewRegisterModelsForm(models, preselected, result)
}

// EditModelsResult holds the result from the edit-models form.
type EditModelsResult struct {
	SelectedModels []string
	CustomModels   string // comma-separated custom model IDs
}

// NewEditModelsForm creates a form to edit which models are included for a
// binding. Shows multi-select of provider models (current ones pre-selected)
// plus a text input for custom model IDs (in case the fetch missed some).
func NewEditModelsForm(models []domain.ProviderModel, currentModels []string, result *EditModelsResult) *huh.Form {
	preselected := make(map[string]bool, len(currentModels))
	for _, m := range currentModels {
		preselected[m] = true
	}

	opts := make([]huh.Option[string], 0, len(models))
	for _, m := range models {
		opts = append(opts, huh.NewOption(m.ModelName, m.ModelName))
	}

	defaults := make([]string, 0, len(currentModels))
	for _, m := range models {
		if preselected[m.ModelName] {
			defaults = append(defaults, m.ModelName)
		}
	}

	result.SelectedModels = defaults

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select Models").
				Description("Pick models to include. Space to toggle.").
				Options(opts...).
				Value(&result.SelectedModels),
			huh.NewInput().
				Title("Custom Models").
				Description("Extra model IDs not in the list (comma-separated)").
				Placeholder("e.g. my-custom-model,other-model").
				Value(&result.CustomModels),
		),
	)
}

// EditCLIPathResult holds the values from the Edit CLI Path form.
type EditCLIPathResult struct {
	CLIID      int64
	ConfigPath string
}

// NewSelectCLIForm creates a form to select a CLI for management.
func NewSelectCLIForm(clis []domain.TargetCLI, result *int64) *huh.Form {
	opts := make([]huh.Option[int64], len(clis))
	for i, c := range clis {
		label := c.Name + "  (" + c.ConfigPath + ")"
		opts[i] = huh.NewOption(label, c.ID)
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int64]().
				Title("Select CLI to Edit").
				Filtering(true).
				Options(opts...).
				Value(result),
		),
	)
}

// NewEditCLIPathForm creates a form to edit a CLI's config path.
func NewEditCLIPathForm(cli *domain.TargetCLI, result *EditCLIPathResult) *huh.Form {
	result.CLIID = cli.ID
	result.ConfigPath = cli.ConfigPath
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Config Path for " + cli.Name).
				Placeholder("~/.config/claude/settings.json").
				Value(&result.ConfigPath).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("path is required")
					}
					return nil
				}),
		),
	)
}

// NewMapModelsForm creates a dynamic form with one Select per env var.
// Returns the form and a function to extract the mapping result after completion.
func NewMapModelsForm(envVars []string, models []domain.ProviderModel) (*huh.Form, func() MapModelsResult) {
	// Create value pointers for each env var
	type envVarBinding struct {
		name  string
		value string
	}
	bindings := make([]envVarBinding, len(envVars))

	// Build model options
	opts := make([]huh.Option[string], 0, len(models)+1)
	opts = append(opts, huh.NewOption("(Not Selected)", ""))
	for _, m := range models {
		opts = append(opts, huh.NewOption(m.ModelName, m.ModelName))
	}

	groups := make([]*huh.Group, len(envVars))
	for i, ev := range envVars {
		bindings[i].name = ev
		// Add "(Apply to all)" option for env vars 2+
		itemOpts := opts
		if i > 0 {
			extra := make([]huh.Option[string], 0, len(opts)+1)
			extra = append(extra, huh.NewOption("(Apply to all)", "__apply_all__"))
			extra = append(extra, opts...)
			itemOpts = extra
		}
		groups[i] = huh.NewGroup(
			huh.NewSelect[string]().
				Title(ev).
				Description(fmt.Sprintf("Select model for %s", ev)).
				Filtering(true).
				Options(itemOpts...).
				Value(&bindings[i].value),
		)
	}

	extract := func() MapModelsResult {
		result := MapModelsResult{
			Mappings: make(map[string]string, len(bindings)),
		}
		firstModel := ""
		for _, b := range bindings {
			if b.value == "__apply_all__" {
				// Fill remaining with first env var's model
				for j := range bindings {
					if bindings[j].value != "__apply_all__" && bindings[j].value != "" {
						firstModel = bindings[j].value
						break
					}
				}
				continue
			}
			result.Mappings[b.name] = b.value
		}
		// If any "__apply_all__" was selected, propagate first model
		for _, b := range bindings {
			if b.value == "__apply_all__" && firstModel != "" {
				result.Mappings[b.name] = firstModel
			}
		}
		return result
	}

	return huh.NewForm(groups...), extract
}

// EditProviderResult holds the values submitted from the Edit Provider form.
type EditProviderResult struct {
	Name         string
	BaseURL      string
	DiscoveryURL string // optional, for model discovery
	APIKey       string
	AuthToken    string
	ApiType      string
}

// NewEditProviderForm creates a pre-filled form for editing an existing provider.
func NewEditProviderForm(provider domain.Provider, result *EditProviderResult) *huh.Form {
	apiTypeOpts := []huh.Option[string]{
		huh.NewOption("OpenAI / OpenAI-compatible", "openai"),
		huh.NewOption("Anthropic", "anthropic"),
		huh.NewOption("Google AI (Gemini)", "google"),
	}
	result.Name = provider.Name
	result.BaseURL = provider.BaseURL
	result.DiscoveryURL = provider.DiscoveryURL
	result.APIKey = provider.APIKey
	result.AuthToken = provider.AuthToken
	result.ApiType = string(provider.ApiType)

	return huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("Editing: "+provider.Name).
				Description("Name is read-only. Update fields below."),
			huh.NewInput().
				Title("Base URL").
				Placeholder("https://api.openai.com/v1").
				Value(&result.BaseURL).
				Validate(func(s string) error {
					u, err := url.ParseRequestURI(s)
					if err != nil {
						return fmt.Errorf("must be a valid URL")
					}
					if u.Scheme == "" || u.Host == "" {
						return fmt.Errorf("URL must include scheme and host")
					}
					return nil
				}),
			huh.NewInput().
				Title("API Key").
				Value(&result.APIKey).
				EchoMode(huh.EchoModePassword).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("API key is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("Auth Token").
				Description("Optional if same as API Key").
				Value(&result.AuthToken).
				EchoMode(huh.EchoModePassword),
			huh.NewInput().
				Title("Discovery URL (optional)").
				Description("Separate URL for model discovery. Leave empty to use Base URL.").
				Placeholder("https://api.bifrost.local/v1").
				Value(&result.DiscoveryURL),
			huh.NewSelect[string]().
				Title("API Type").
				Description("Model discovery method").
				Options(apiTypeOpts...).
				Value(&result.ApiType),
		),
	).WithHeight(11)
}

// NewRestoreBackupForm creates a form to select a backup to restore.
func NewRestoreBackupForm(backups []application.BackupOption, result *string) *huh.Form {
	opts := make([]huh.Option[string], 0, len(backups))
	for _, b := range backups {
		opts = append(opts, huh.NewOption(b.Label, b.Path))
	}
	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select Backup to Restore").
				Description("Newest first. Restore overwrites current config.").
				Filtering(true).
				Options(opts...).
				Value(result),
		),
	)
}
