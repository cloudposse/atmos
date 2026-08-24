package helm

import "github.com/cloudposse/atmos/pkg/perf"

// Config holds the resolved global configuration for Helm components.
type Config struct {
	BasePath          string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	AutoGenerateFiles bool   `yaml:"auto_generate_files" json:"auto_generate_files" mapstructure:"auto_generate_files"`
}

// DefaultConfig returns the built-in defaults for Helm components.
func DefaultConfig() Config {
	defer perf.Track(nil, "helm.DefaultConfig")()

	return Config{
		BasePath:          "components/helm",
		AutoGenerateFiles: false,
	}
}
