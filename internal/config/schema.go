package config

type Terraform struct {
	BasePath string `yaml:"base_path" mapstructure:"base_path"`
}

type Helmfile struct {
	BasePath string `yaml:"base_path" mapstructure:"base_path"`
}

type Components struct {
	Terraform Terraform
	Helmfile  Helmfile
}

type Stacks struct {
	BasePath      string   `yaml:"base_path" mapstructure:"base_path"`
	IncludedPaths []string `yaml:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" mapstructure:"excluded_paths"`
	NamePattern   string   `yaml:"name_pattern" mapstructure:"name_pattern"`
}

type Configuration struct {
	Components Components
	Stacks     Stacks
}

type ProcessedConfiguration struct {
	StacksBaseAbsolutePath    string   `yaml:"StacksBaseAbsolutePath"`
	IncludeStackAbsolutePaths []string `yaml:"IncludeStackAbsolutePaths"`
	ExcludeStackAbsolutePaths []string `yaml:"ExcludeStackAbsolutePaths"`
	TerraformDirAbsolutePath  string   `yaml:"TerraformDirAbsolutePath"`
	HelmfileDirAbsolutePath   string   `yaml:"HelmfileDirAbsolutePath"`
	StackConfigFiles          []string `yaml:"StackConfigFiles"`
	StackType                 string   `yaml:"StackType"`
}
