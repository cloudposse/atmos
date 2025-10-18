package mock

// Config represents the configuration structure for mock components.
// This configuration is stored in the Components.Plugins map in atmos.yaml.
// It's designed to test inheritance and merging behavior across global, stack,
// and component configuration levels.
type Config struct {
	// BasePath is the filesystem path to mock components.
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`

	// Enabled determines if the mock component is active.
	// Can be overridden at stack or component level.
	Enabled bool `yaml:"enabled" json:"enabled" mapstructure:"enabled"`

	// DryRun prevents actual execution when true.
	DryRun bool `yaml:"dry_run" json:"dry_run" mapstructure:"dry_run"`

	// Tags are labels that can be merged across inheritance levels.
	Tags []string `yaml:"tags" json:"tags" mapstructure:"tags"`

	// Metadata contains nested configuration for testing deep merging.
	Metadata map[string]interface{} `yaml:"metadata" json:"metadata" mapstructure:"metadata"`

	// Dependencies lists component dependencies for DAG testing.
	Dependencies []string `yaml:"dependencies" json:"dependencies" mapstructure:"dependencies"`
}

// DefaultConfig returns the default configuration for mock components.
func DefaultConfig() Config {
	return Config{
		BasePath:     "components/mock",
		Enabled:      true,
		DryRun:       false,
		Tags:         []string{},
		Metadata:     make(map[string]interface{}),
		Dependencies: []string{},
	}
}
