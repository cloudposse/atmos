package schema

// Dependencies declares required tools and their versions.
type Dependencies struct {
	// Tools maps tool names to version constraints (e.g., "terraform": "1.5.0" or "latest").
	Tools map[string]string `yaml:"tools,omitempty" json:"tools,omitempty" mapstructure:"tools"`
}
