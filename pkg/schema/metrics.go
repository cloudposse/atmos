package schema

// MetricsSettings contains configuration for local command-execution metrics display.
type MetricsSettings struct {
	// Enabled controls the local per-command / final-summary resource-usage display (ui.Info). Defaults to true when unset, except in standalone Starlark scripts (interpreter mode), where the summary is shown only when explicitly set to true. Never gates the Atmos Pro exec-metadata upload — only local display.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
}
