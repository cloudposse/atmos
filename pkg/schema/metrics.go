package schema

// MetricsSettings contains configuration for local command-execution metrics display.
type MetricsSettings struct {
	// Enabled controls the local per-command / final-summary resource-usage display (ui.Info). Defaults to true when unset. Never gates the Atmos Pro exec-metadata upload — only local display.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}
