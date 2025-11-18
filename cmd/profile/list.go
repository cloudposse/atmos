package profile

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

//go:embed markdown/atmos_profile_list_long.md
var profileListLongMarkdown string

//go:embed markdown/atmos_profile_list_usage.md
var profileListUsageMarkdown string

// profileListCmd lists available configuration profiles.
var profileListCmd = &cobra.Command{
	Use:                "list",
	Short:              "List available configuration profiles",
	Long:               profileListLongMarkdown,
	Example:            profileListUsageMarkdown,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  cobra.NoFileCompletions,
	RunE:               executeProfileListCommand,
}

func init() {
	defer perf.Track(nil, "profile.init.profileListCmd")()

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
	defer perf.Track(nil, "profile.profileFormatFlagCompletion")()

	return []string{"table", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
}

// executeProfileListCommand handles the profile list command execution.
func executeProfileListCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "profile.executeProfileListCommand")()

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
		return buildProfileDiscoveryError(err, &atmosConfig)
	}

	// Render output based on format.
	output, err := renderProfileListOutput(&atmosConfig, profiles, format)
	if err != nil {
		return err
	}

	fmt.Print(output)

	return nil
}

// renderProfilesJSON renders profiles as JSON.
func renderProfilesJSON(profiles []profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.renderProfilesJSON")()

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return "", errUtils.Build(errUtils.ErrOutputFormat).
			WithExplanationf("Failed to generate JSON output: `%s`", err).
			WithExplanation("This is likely an internal error").
			WithHint("Try using `--format yaml` or `--format table` as a workaround").
			WithHint("Please report this issue if it persists").
			WithContext("format", "json").
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	return string(data) + "\n", nil
}

// renderProfilesYAML renders profiles as YAML.
func renderProfilesYAML(profiles []profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.renderProfilesYAML")()

	data, err := yaml.Marshal(profiles)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrOutputFormat).
			WithExplanationf("Failed to generate YAML output: `%s`", err).
			WithExplanation("This is likely an internal error").
			WithHint("Try using `--format json` or `--format table` as a workaround").
			WithHint("Please report this issue if it persists").
			WithContext("format", "yaml").
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	return string(data), nil
}

// buildProfileDiscoveryError creates a detailed error for profile discovery failures.
func buildProfileDiscoveryError(err error, atmosConfig *schema.AtmosConfiguration) error {
	return errUtils.Build(errUtils.ErrProfileDiscovery).
		WithExplanationf("Failed to discover profiles: `%s`", err).
		WithExplanation("Could not read profile directories or determine profile locations").
		WithHint("Check `profiles.base_path` configuration in `atmos.yaml`").
		WithHint("Verify profile directories exist and are accessible").
		WithHint("Run `atmos describe config` to view profile configuration").
		WithContext("config_path", atmosConfig.CliConfigPath).
		WithContext("base_path", atmosConfig.Profiles.BasePath).
		WithExitCode(2).
		Err()
}

// renderProfileListOutput renders profiles in the requested format.
func renderProfileListOutput(atmosConfig *schema.AtmosConfiguration, profiles []profile.ProfileInfo, format string) (string, error) {
	defer perf.Track(atmosConfig, "profile.renderProfileListOutput")()

	var output string
	var err error

	switch format {
	case "json":
		output, err = renderProfilesJSON(profiles)
	case "yaml":
		output, err = renderProfilesYAML(profiles)
	case "table":
		output, err = profileList.RenderTable(profiles)
	default:
		return "", errUtils.Build(errUtils.ErrInvalidFormat).
			WithExplanationf("The format `%s` is not supported for this command", format).
			WithExplanation("Only `table`, `json`, and `yaml` formats are available").
			WithHint("Use `--format table`, `--format json`, or `--format yaml`").
			WithHint("Example: `atmos profile list --format table`").
			WithContext("format", format).
			WithContext("command", "profile list").
			WithContext("supported_formats", "table, json, yaml").
			WithExitCode(2).
			Err()
	}

	return output, err
}
