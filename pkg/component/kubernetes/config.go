package kubernetes

import "github.com/cloudposse/atmos/pkg/perf"

const (
	ProviderKubectl   = "kubectl"
	ProviderKustomize = "kustomize"
)

type Config struct {
	BasePath          string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	Provider          string `yaml:"provider" json:"provider" mapstructure:"provider"`
	AutoGenerateFiles bool   `yaml:"auto_generate_files" json:"auto_generate_files" mapstructure:"auto_generate_files"`
}

func DefaultConfig() Config {
	defer perf.Track(nil, "kubernetes.DefaultConfig")()

	return Config{
		BasePath:          "components/kubernetes",
		Provider:          ProviderKubectl,
		AutoGenerateFiles: false,
	}
}
