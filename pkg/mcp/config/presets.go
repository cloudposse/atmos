package config

import (
	"github.com/cloudposse/atmos/pkg/schema"
)

// atmosProMCPURL is the hosted Atmos Pro MCP server endpoint.
const atmosProMCPURL = "https://atmos-pro.com/mcp"

// PresetSelf and PresetAtmosPro are the built-in preset names recognized by
// `atmos mcp add`.
const (
	PresetSelf     = "self"
	PresetAtmosPro = "atmos-pro"
)

// Preset is a named, built-in server definition `atmos mcp add` can resolve
// without the user typing out a URL or command.
type Preset struct {
	Name string
	// DefaultServerName is the mcp.servers key used unless the user passes
	// --name explicitly.
	DefaultServerName string
	Description       string
	// RequiresMCPEnabled marks presets that only work when mcp.enabled is
	// true (i.e. Atmos itself needs to be able to run as an MCP server).
	RequiresMCPEnabled bool
	Resolve            func(atmosConfig *schema.AtmosConfiguration) schema.MCPServerConfig
}

var presets = []Preset{
	{
		Name:               PresetSelf,
		DefaultServerName:  "atmos",
		Description:        "Atmos's own AI tools (describe, list, validate, ...) exposed via MCP",
		RequiresMCPEnabled: true,
		Resolve: func(_ *schema.AtmosConfiguration) schema.MCPServerConfig {
			return schema.MCPServerConfig{
				Command:     "atmos",
				Args:        []string{"mcp", "start"},
				Description: "Atmos's own AI tools (describe, list, validate, ...) exposed via MCP",
			}
		},
	},
	{
		Name:               PresetAtmosPro,
		DefaultServerName:  PresetAtmosPro,
		Description:        "Atmos Pro — deployments, drift, approvals, and audit history via MCP",
		RequiresMCPEnabled: false,
		Resolve: func(_ *schema.AtmosConfiguration) schema.MCPServerConfig {
			return schema.MCPServerConfig{
				Type:        schema.MCPTransportHTTP,
				URL:         atmosProMCPURL,
				Description: "Atmos Pro — deployments, drift, approvals, and audit history via MCP",
			}
		},
	},
}

// ResolvePreset looks up a built-in preset by name.
func ResolvePreset(name string) (Preset, bool) {
	for _, preset := range presets {
		if preset.Name == name {
			return preset, true
		}
	}
	return Preset{}, false
}

// Presets returns all built-in presets, for --help/listing.
func Presets() []Preset {
	return append([]Preset(nil), presets...)
}
