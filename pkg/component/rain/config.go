package rain

import "github.com/cloudposse/atmos/pkg/perf"

// Config represents global Rain component configuration.
type Config struct {
	BasePath          string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Command           string `yaml:"command" json:"command" mapstructure:"command"`
	AutoGenerateFiles bool   `yaml:"auto_generate_files" json:"auto_generate_files" mapstructure:"auto_generate_files"`
}

// DefaultConfig returns default Rain component configuration.
func DefaultConfig() Config {
	defer perf.Track(nil, "rain.DefaultConfig")()

	return Config{
		BasePath:          "components/rain",
		Command:           "rain",
		AutoGenerateFiles: false,
	}
}
