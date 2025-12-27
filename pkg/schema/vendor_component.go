package schema

// Component vendoring (`component.yaml` file)

type VendorComponentSource struct {
	Type          string       `yaml:"type" json:"type" mapstructure:"type"`
	Uri           string       `yaml:"uri" json:"uri" mapstructure:"uri"`
	Version       string       `yaml:"version" json:"version" mapstructure:"version"`
	IncludedPaths []string     `yaml:"included_paths" json:"included_paths" mapstructure:"included_paths"`
	ExcludedPaths []string     `yaml:"excluded_paths" json:"excluded_paths" mapstructure:"excluded_paths"`
	Retry         *RetryConfig `yaml:"retry,omitempty" json:"retry,omitempty" mapstructure:"retry"`
}

type VendorComponentMixins struct {
	Type     string `yaml:"type" json:"type" mapstructure:"type"`
	Uri      string `yaml:"uri" json:"uri" mapstructure:"uri"`
	Version  string `yaml:"version" json:"version" mapstructure:"version"`
	Filename string `yaml:"filename" json:"filename" mapstructure:"filename"`
}

type VendorComponentSpec struct {
	Source VendorComponentSource   `yaml:"source" json:"source" mapstructure:"source"`
	Mixins []VendorComponentMixins `yaml:"mixins" json:"mixins" mapstructure:"mixins"`
}

type VendorComponentMetadata struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
}

type VendorComponentConfig struct {
	ApiVersion string                  `yaml:"apiVersion" json:"apiVersion" mapstructure:"apiVersion"`
	Kind       string                  `yaml:"kind" json:"kind" mapstructure:"kind"`
	Metadata   VendorComponentMetadata `yaml:"metadata" json:"metadata" mapstructure:"metadata"`
	Spec       VendorComponentSpec     `yaml:"spec" json:"spec" mapstructure:"spec"`
}
