package schema

// MCPSettings contains configuration for the MCP (Model Context Protocol) server
// and external MCP server connections.
type MCPSettings struct {
	Enabled bool                       `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Servers map[string]MCPServerConfig `yaml:"servers,omitempty" json:"servers,omitempty" mapstructure:"servers"`
	Routing MCPRoutingConfig           `yaml:"routing,omitempty" json:"routing,omitempty" mapstructure:"routing"`
}

// MCPRoutingConfig configures the two-pass routing that selects which MCP servers
// are relevant to a user's question before starting them.
type MCPRoutingConfig struct {
	// Enabled controls whether automatic server routing is active (default: true).
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	// Model is the fast model used for routing (default: claude-haiku-4-5-20251001).
	Model string `yaml:"model,omitempty" json:"model,omitempty" mapstructure:"model"`
	// Provider overrides the AI provider for routing (default: same as ai.default_provider).
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty" mapstructure:"provider"`
}

// IsEnabled returns true if routing is enabled (defaults to true when not explicitly set).
func (r *MCPRoutingConfig) IsEnabled() bool {
	if r.Enabled == nil {
		return true
	}
	return *r.Enabled
}

// MCPServerConfig represents an external MCP server configured in atmos.yaml
// under mcp.servers. The core fields (command, args, env) follow the standard
// MCP server configuration format used by Claude Code, Codex CLI, and Gemini CLI.
// Atmos-specific extensions (description, auto_start, timeout, auth_identity)
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
}
