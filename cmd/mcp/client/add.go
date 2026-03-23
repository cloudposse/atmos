package client

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var addParser *flags.StandardParser

//go:embed markdown/atmos_mcp_add.md
var addLongMarkdown string

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add an MCP server to atmos.yaml",
	Long:  addLongMarkdown,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.mcpAdd")()

		name := args[0]

		v := viper.GetViper()
		if err := addParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		command := v.GetString("command")
		if command == "" {
			return errUtils.Build(errUtils.ErrMCPServerCommandEmpty).
				WithHint("Use --command to specify the server command (e.g., --command uvx)").
				Err()
		}

		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}

		// Check if server already exists.
		if _, exists := atmosConfig.MCP.Servers[name]; exists {
			return errUtils.Build(errUtils.ErrMCPServerAlreadyExists).
				WithExplanationf("Server %q is already configured in atmos.yaml", name).
				WithHintf("Use 'atmos mcp remove %s' first, or edit atmos.yaml directly", name).
				Err()
		}

		// Build the server config.
		server := map[string]any{
			"command": command,
		}

		if cmdArgs := v.GetStringSlice("args"); len(cmdArgs) > 0 {
			server["args"] = cmdArgs
		}

		if desc := v.GetString("description"); desc != "" {
			server["description"] = desc
		}

		if envVars := v.GetStringSlice("env"); len(envVars) > 0 {
			envMap := make(map[string]string)
			for _, e := range envVars {
				for i, c := range e {
					if c == '=' {
						envMap[e[:i]] = e[i+1:]
						break
					}
				}
			}
			server["env"] = envMap
		}

		// Write to atmos.yaml.
		configPath := atmosConfig.CliConfigPath
		if configPath == "" {
			configPath = "atmos.yaml"
		}
		configFile := findAtmosYAML(configPath)

		if err := addServerToConfig(configFile, name, server); err != nil {
			return err
		}

		ui.Successf("Added MCP server %q to %s", name, configFile)
		return nil
	},
}

func init() {
	addParser = flags.NewStandardParser(
		flags.WithStringFlag("command", "c", "", "Command to run the MCP server (e.g., uvx, npx)"),
		flags.WithStringSliceFlag("args", "a", nil, "Arguments for the server command"),
		flags.WithStringFlag("description", "d", "", "Description of the server"),
		flags.WithStringSliceFlag("env", "e", nil, "Environment variables (KEY=VALUE format)"),
	)

	addParser.RegisterFlags(addCmd)

	if err := addParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	mcpcmd.McpCmd.AddCommand(addCmd)
}

// findAtmosYAML returns the path to the atmos.yaml config file.
func findAtmosYAML(configPath string) string {
	candidates := []string{
		filepath.Join(configPath, "atmos.yaml"),
		filepath.Join(configPath, "atmos.yml"),
		"atmos.yaml",
		"atmos.yml",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "atmos.yaml"
}

// addServerToConfig adds an MCP server to the atmos.yaml file.
func addServerToConfig(configFile, name string, server map[string]any) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigReadFailed, configFile, err)
	}

	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrMCPConfigParseFailed, configFile, err)
	}

	// Ensure mcp section exists.
	mcpSection, ok := config["mcp"].(map[string]any)
	if !ok {
		mcpSection = make(map[string]any)
		config["mcp"] = mcpSection
	}

	// Ensure servers section exists.
	servers, ok := mcpSection["servers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
		mcpSection["servers"] = servers
	}

	// Add the new server.
	servers[name] = server

	// Write back.
	output, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrMCPConfigWriteFailed, err)
	}

	const configFilePerms = 0o644
	return os.WriteFile(configFile, output, configFilePerms)
}
