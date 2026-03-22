package schema

// MCPSettings contains configuration for the MCP (Model Context Protocol) server
// and external MCP integrations.
type MCPSettings struct {
	Enabled      bool                            `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Integrations map[string]MCPIntegrationConfig `yaml:"integrations,omitempty" json:"integrations,omitempty" mapstructure:"integrations"`
}

// MCPIntegrationConfig represents an external MCP server integration
// configured in atmos.yaml under mcp.integrations.
type MCPIntegrationConfig struct {
	Description string            `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	Command     string            `yaml:"command" json:"command" mapstructure:"command"`
	Args        []string          `yaml:"args,omitempty" json:"args,omitempty" mapstructure:"args"`
	Env         map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
	AutoStart   bool              `yaml:"auto_start,omitempty" json:"auto_start,omitempty" mapstructure:"auto_start"`
	Timeout     string            `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`
}
