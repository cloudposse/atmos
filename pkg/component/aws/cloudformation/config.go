package cloudformation

import "github.com/cloudposse/atmos/pkg/perf"

// Config holds the resolved global configuration for aws/cloudformation components.
type Config struct {
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}

// DefaultConfig returns the built-in defaults for aws/cloudformation components.
func DefaultConfig() Config {
	defer perf.Track(nil, "cloudformation.DefaultConfig")()

	return Config{
		BasePath: "components/cloudformation",
	}
}
