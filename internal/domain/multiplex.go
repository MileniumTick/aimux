package domain

// ActiveMultiplex represents an active multiplex row with joined data.
type ActiveMultiplex struct {
	TargetCLIID    int64
	ProviderID     int64
	ModelMappings  string
	ActivatedAt    string
	ProviderName   string
	CLIName        string
	ProviderStatus string
}

// MultiplexRepository defines the interface for multiplex persistence.
type MultiplexRepository interface {
	GetActive(targetCLIID int64) (ActiveMultiplex, error)
	SetActive(targetCLIID, providerID int64, modelMappingsJSON string) error
	ClearActive(targetCLIID int64) error
	ListActive() ([]ActiveMultiplex, error)
}
