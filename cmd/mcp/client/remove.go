package client

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/atmos_mcp_remove.md
var removeLongMarkdown string

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an MCP server from atmos.yaml",
	Long:  removeLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.mcpRemove")()

		name := args[0]

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Check if server exists.
		if _, exists := atmosConfig.MCP.Servers[name]; !exists {
			return errUtils.Build(errUtils.ErrMCPServerNotFound).
				WithExplanationf("Server %q is not configured in atmos.yaml", name).
				WithHint("Use 'atmos mcp list' to see configured servers").
				Err()
		}

		configFile := findAtmosYAML(atmosConfig.CliConfigPath)

		if err := removeServerFromConfig(configFile, name); err != nil {
			return err
		}

		ui.Successf("Removed MCP server %q from %s", name, configFile)
		return nil
	},
}

func init() {
	mcpcmd.McpCmd.AddCommand(removeCmd)
}

// removeServerFromConfig removes an MCP server from the atmos.yaml file.
func removeServerFromConfig(configFile, name string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigReadFailed, configFile, err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigParseFailed, configFile, err)
	}

	mcpSection, ok := config["mcp"].(map[string]any)
	if !ok {
		return nil
	}

	servers, ok := mcpSection["servers"].(map[string]any)
	if !ok {
		return nil
	}

	delete(servers, name)

	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrMCPConfigWriteFailed, err)
	}

	const configFilePerms = 0o644
	return os.WriteFile(configFile, output, configFilePerms)
}
