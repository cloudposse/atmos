package schema

import "strings"

const (
	// MCPTransportStdio runs an MCP server as a local subprocess over stdio.
	MCPTransportStdio = "stdio"
	// MCPTransportHTTP connects to a remote MCP server over streamable HTTP.
	MCPTransportHTTP = "http"
)

// MCPSettings contains configuration for the MCP (Model Context Protocol) server
// and external MCP server connections.
type MCPSettings struct {
	Enabled bool                       `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
	Servers map[string]MCPServerConfig `yaml:"servers,omitempty" json:"servers,omitempty" mapstructure:"servers"`
	Routing MCPRoutingConfig           `yaml:"routing,omitempty" json:"routing,omitempty" mapstructure:"routing"`
}

// MCPRoutingConfig configures the two-pass routing that selects which MCP servers
// are relevant to a user's question before starting them.
// Routing uses the same AI provider and model configured under ai.default_provider.
type MCPRoutingConfig struct {
	// Enabled controls whether automatic server routing is active (default: true).
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" mapstructure:"enabled"`
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
// Atmos-specific extensions (description, auto_start, timeout, identity)
// provide additional functionality.
type MCPServerConfig struct {
	// Standard MCP server fields (compatible with mcpServers JSON format).
	Command string            `yaml:"command" json:"command" mapstructure:"command"`
	Args    []string          `yaml:"args,omitempty" json:"args,omitempty" mapstructure:"args"`
	Env     map[string]string `yaml:"env,omitempty" json:"env,omitempty" mapstructure:"env"`
	Type    string            `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	URL     string            `yaml:"url,omitempty" json:"url,omitempty" mapstructure:"url"`
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty" mapstructure:"headers"`

	// Atmos-specific extensions.
	Description string `yaml:"description,omitempty" json:"description,omitempty" mapstructure:"description"`
	AutoStart   bool   `yaml:"auto_start,omitempty" json:"auto_start,omitempty" mapstructure:"auto_start"`
	Timeout     string `yaml:"timeout,omitempty" json:"timeout,omitempty" mapstructure:"timeout"`
	// Identity is the Atmos Auth identity (from the auth section) for credential injection.
	Identity string `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
}

// TransportType returns the configured MCP transport, inferring a default when
// omitted for backward compatibility with existing stdio-only configurations.
func (c MCPServerConfig) TransportType() string { //nolint:gocritic // This value receiver keeps literal calls ergonomic in config tests.
	t := strings.ToLower(strings.TrimSpace(c.Type))
	if t != "" {
		return t
	}
	if c.URL != "" {
		return MCPTransportHTTP
	}
	return MCPTransportStdio
}
