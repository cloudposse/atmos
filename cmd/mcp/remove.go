package mcp

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an MCP integration from atmos.yaml",
	Long:  "Remove an external MCP server integration from the mcp.integrations section of atmos.yaml.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.mcpRemove")()

		name := args[0]

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Check if integration exists.
		if _, exists := atmosConfig.MCP.Integrations[name]; !exists {
			return errUtils.Build(errUtils.ErrMCPIntegrationNotFound).
				WithExplanationf("Integration %q is not configured in atmos.yaml", name).
				WithHint("Use 'atmos mcp list' to see configured integrations").
				Err()
		}

		configFile := findAtmosYAML(atmosConfig.CliConfigPath)

		if err := removeIntegrationFromConfig(configFile, name); err != nil {
			return err
		}

		ui.Successf("Removed MCP integration %q from %s", name, configFile)
		return nil
	},
}

func init() {
	mcpCmd.AddCommand(removeCmd)
}

// removeIntegrationFromConfig removes an MCP integration from the atmos.yaml file.
func removeIntegrationFromConfig(configFile, name string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", configFile, err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse %s: %w", configFile, err)
	}

	mcpSection, ok := config["mcp"].(map[string]any)
	if !ok {
		return nil
	}

	integrations, ok := mcpSection["integrations"].(map[string]any)
	if !ok {
		return nil
	}

	delete(integrations, name)

	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	const configFilePerms = 0o644
	return os.WriteFile(configFile, output, configFilePerms)
}
