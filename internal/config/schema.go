package config

type Terraform struct {
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}

type Helmfile struct {
	BasePath string `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
}

type Components struct {
	Terraform Terraform
	Helmfile  Helmfile
}

type Stacks struct {
	BasePath      string   `yaml:"base_path" json:"base_path" mapstructure:"base_path"`
	IncludedPaths []string `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
	NamePattern   string   `yaml:"name_pattern" json:"name_pattern" mapstructure:"name_pattern"`
}

type Configuration struct {
	Components Components
	Stacks     Stacks
}

type ProcessedConfiguration struct {
	StacksBaseAbsolutePath        string   `yaml:"StacksBaseAbsolutePath" json:"StacksBaseAbsolutePath"`
	IncludeStackAbsolutePaths     []string `yaml:"IncludeStackAbsolutePaths" json:"IncludeStackAbsolutePaths"`
	ExcludeStackAbsolutePaths     []string `yaml:"ExcludeStackAbsolutePaths" json:"ExcludeStackAbsolutePaths"`
	TerraformDirAbsolutePath      string   `yaml:"TerraformDirAbsolutePath" json:"TerraformDirAbsolutePath"`
	HelmfileDirAbsolutePath       string   `yaml:"HelmfileDirAbsolutePath" json:"HelmfileDirAbsolutePath"`
	StackConfigFilesRelativePaths []string `yaml:"StackConfigFilesRelativePaths" json:"StackConfigFilesRelativePaths"`
	StackConfigFilesAbsolutePaths []string `yaml:"StackConfigFilesAbsolutePaths" json:"StackConfigFilesAbsolutePaths"`
	StackType                     string   `yaml:"StackType" json:"StackType"`
}
