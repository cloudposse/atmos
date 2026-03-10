package schema

// MCPSettings contains configuration for the MCP (Model Context Protocol) server.
type MCPSettings struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"` // Enable MCP server (default: false)
}
