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


