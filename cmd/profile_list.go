package cmd

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profile"
	profileList "github.com/cloudposse/atmos/pkg/profile/list"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed markdown/atmos_profile_list_usage.md
var profileListUsageMarkdown string

// profileListCmd lists available configuration profiles.
var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available configuration profiles",
	Long: `List all configured profiles across all locations with their details.

Profiles are discovered from multiple locations in precedence order:
1. Configurable (profiles.base_path in atmos.yaml)
2. Project-hidden (.atmos/profiles/)
3. XDG user (~/.config/atmos/profiles/ or $XDG_CONFIG_HOME/atmos/profiles/)
4. Project (profiles/)

Supports multiple output formats:
- **table** (default): tabular view with profile details
- **json**/**yaml**: structured data for programmatic access`,
	Example:            profileListUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeProfileListCommand,
}

func init() {
	defer perf.Track(nil, "cmd.init.profileListCmd")()

	// Format flag.
	profileListCmd.Flags().StringP("format", "f", "table", "Output format: table, json, yaml")

	// Register flag completion functions.
	if err := profileListCmd.RegisterFlagCompletionFunc("format", profileFormatFlagCompletion); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	profileCmd.AddCommand(profileListCmd)
}

// profileFormatFlagCompletion provides shell completion for the format flag.
func profileFormatFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"table", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
}

// executeProfileListCommand handles the profile list command execution.
func executeProfileListCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.executeProfileListCommand")()

	// Get format flag.
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}

	// Initialize config.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	// Create profile manager.
	manager := profile.NewProfileManager()

	// List all profiles.
	profiles, err := manager.ListProfiles(&atmosConfig)
	if err != nil {
		return fmt.Errorf("%w: failed to list profiles: %s", errUtils.ErrProfileDiscovery, err)
	}

	// Render output based on format.
	var output string
	switch format {
	case "json":
		output, err = renderProfilesJSON(profiles)
	case "yaml":
		output, err = renderProfilesYAML(profiles)
	case "table":
		output, err = profileList.RenderTable(profiles)
	default:
		return fmt.Errorf("%w: unsupported format '%s' (supported: table, json, yaml)", errUtils.ErrInvalidFormat, format)
	}

	if err != nil {
		return err
	}

	fmt.Print(output)

	return nil
}

// renderProfilesJSON renders profiles as JSON.
func renderProfilesJSON(profiles []profile.ProfileInfo) (string, error) {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal profiles to JSON: %s", errUtils.ErrOutputFormat, err)
	}
	return string(data) + "\n", nil
}

// renderProfilesYAML renders profiles as YAML.
func renderProfilesYAML(profiles []profile.ProfileInfo) (string, error) {
	data, err := yaml.Marshal(profiles)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal profiles to YAML: %s", errUtils.ErrOutputFormat, err)
	}
	return string(data), nil
}
