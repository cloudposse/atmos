package client

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_export.md
var exportLongMarkdown string

const configFilePermissions = 0o600

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export .mcp.json from atmos.yaml MCP server configuration",
	Long:  exportLongMarkdown,
	RunE:  executeMCPExport,
}

func init() {
	exportCmd.Flags().StringP("output", "o", ".mcp.json", "Output file path")
	mcpcmd.McpCmd.AddCommand(exportCmd)
}

// mcpJSONConfig represents the .mcp.json file format used by Claude Code and other IDEs.
type mcpJSONConfig struct {
	MCPServers map[string]mcpJSONServer `json:"mcpServers"`
}

// mcpJSONServer represents a single MCP server entry in .mcp.json.
type mcpJSONServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

func executeMCPExport(cmd *cobra.Command, _ []string) error {
	defer perf.Track(nil, "cmd.mcpExport")()
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	if len(atmosConfig.MCP.Servers) == 0 {
		ui.Info("No MCP servers configured. Add servers under `mcp.servers` in `atmos.yaml`.")
		return nil
	}

	outputFile, _ := cmd.Flags().GetString("output")

	config := mcpJSONConfig{
		MCPServers: make(map[string]mcpJSONServer),
	}

	for name, serverCfg := range atmosConfig.MCP.Servers {
		config.MCPServers[name] = buildMCPJSONEntry(name, &serverCfg)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrMCPConfigMarshalFailed, err)
	}

	if err := os.WriteFile(outputFile, append(data, '\n'), configFilePermissions); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigWriteFailed, outputFile, err)
	}
	// Enforce permissions on existing files (WriteFile only sets perms on new files).
	if err := os.Chmod(outputFile, configFilePermissions); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigPermsFailed, outputFile, err)
	}

	ui.Success(fmt.Sprintf("Generated %s with %d server(s)", outputFile, len(config.MCPServers)))
	return nil
}

// buildMCPJSONEntry creates a .mcp.json entry for a server.
// Servers with identity are wrapped with 'atmos auth exec' for credential injection.
func buildMCPJSONEntry(_ string, serverCfg *schema.MCPServerConfig) mcpJSONServer {
	if serverCfg.Identity != "" {
		// Wrap with atmos auth exec for credential injection.
		args := []string{"auth", "exec", "-i", serverCfg.Identity, "--", serverCfg.Command}
		args = append(args, serverCfg.Args...)
		return mcpJSONServer{
			Command: "atmos",
			Args:    args,
			Env:     serverCfg.Env,
		}
	}

	// No auth — use command directly.
	return mcpJSONServer{
		Command: serverCfg.Command,
		Args:    serverCfg.Args,
		Env:     serverCfg.Env,
	}
}
