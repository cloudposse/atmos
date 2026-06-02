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
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
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
	Args:  cobra.NoArgs,
	RunE:  executeMCPExport,
}

func init() {
	exportCmd.Flags().StringP("output", "o", ".mcp.json", "Output file path")
	mcpcmd.McpCmd.AddCommand(exportCmd)
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

	// Delegate to the shared package builder so the exported .mcp.json
	// matches what the in-process MCP client uses (env-key normalization,
	// `atmos auth exec` wrapping for identity-having servers) and — most
	// importantly — carries the toolchain PATH so IDE-spawned subprocesses
	// can find `uvx` / `npx` from the Atmos toolchain. The cmd-local
	// implementation that previously lived here silently dropped this.
	toolchainPATH := buildToolchainPATH(&atmosConfig)
	config := mcpclient.GenerateMCPConfig(atmosConfig.MCP.Servers, toolchainPATH)

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
