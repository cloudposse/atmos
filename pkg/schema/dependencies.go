package schema

// Dependencies declares required tools and their versions.
type Dependencies struct {
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
}
