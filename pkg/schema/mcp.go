package schema

// MCPSettings contains configuration for the MCP (Model Context Protocol) server
// and external MCP server connections.
type MCPSettings struct {
	Enabled bool                       `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Servers map[string]MCPServerConfig `yaml:"servers,omitempty" json:"servers,omitempty" mapstructure:"servers"`
}

// MCPServerConfig represents an external MCP server configured in atmos.yaml
// under mcp.servers. The core fields (command, args, env) follow the standard
// MCP server configuration format used by Claude Code, Codex CLI, and Gemini CLI.
// Atmos-specific extensions (description, auto_start, timeout, auth_identity, read_only)
// provide additional functionality.
type MCPServerConfig struct {
	// Standard MCP server fields (compatible with mcpServers JSON format).
	Command string            `yaml:"command" json:"command" mapstructure:"command"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty" mapstructure:"args"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`

	// Atmos-specific extensions.
	Description  string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	AutoStart    bool   `yaml:"auto_start,omitempty" json:"auto_start,omitempty" mapstructure:"auto_start"`
	Timeout      string `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`
	AuthIdentity string `yaml:"auth_identity,omitempty" json:"auth_identity,omitempty" mapstructure:"auth_identity"`
	// ReadOnly marks a server as safe for non-interactive commands (atmos ai ask).
	// Read-only servers expose tools that only retrieve data (docs, pricing, etc.)
	// and are included in the read-only tool set alongside native Atmos tools.
	ReadOnly bool `yaml:"read_only,omitempty" json:"read_only,omitempty" mapstructure:"read_only"`
}
