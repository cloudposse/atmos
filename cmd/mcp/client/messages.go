package client

import (
	mcpconfig "github.com/cloudposse/atmos/pkg/mcp/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// noServersConfiguredMessage builds the "nothing configured yet" message
// shared by list/install/status/uninstall, pointing at the three on-ramps:
// adding an external server, adding the built-in self preset, or (if
// mcp.enabled is already true) installing the self preset directly.
func noServersConfiguredMessage(mcpEnabled bool) string {
	base := "No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`."
	if mcpEnabled {
		return base + " To use Atmos's own tools via MCP, run `atmos mcp add self`."
	}
	return base + " To use Atmos's own tools via MCP, run `atmos config set mcp.enabled true` " +
		"then `atmos mcp add self`."
}

// atmosProNudge returns a suggestion to add the atmos-pro preset when Atmos
// Pro is configured (settings.pro.workspace_id is set) but its MCP server
// hasn't been added yet. Matched by URL rather than conventional key name so
// a renamed entry (via --name) doesn't trigger a false nudge. Returns "" when
// there's nothing to suggest.
func atmosProNudge(atmosConfig *schema.AtmosConfiguration) string {
	if atmosConfig.Settings.Pro.WorkspaceID == "" {
		return ""
	}
	preset, ok := mcpconfig.ResolvePreset(mcpconfig.PresetAtmosPro)
	if !ok {
		return ""
	}
	url := preset.Resolve(atmosConfig).URL
	if mcpconfig.HasServerWithURL(atmosConfig.MCP.Servers, url) {
		return ""
	}
	return "Atmos Pro is configured — run `atmos mcp add atmos-pro` to add its MCP server."
}
