package profile

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/profile"
	profileShow "github.com/cloudposse/atmos/pkg/profile/show"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:embed markdown/atmos_profile_show_long.md
var profileShowLongMarkdown string

//go:embed markdown/atmos_profile_show_usage.md
var profileShowUsageMarkdown string

// profileShowCmd shows detailed information about a specific profile.
var profileShowCmd = &cobra.Command{
	Use:                "show <profile-name>",
	Short:              "Show detailed information about a profile",
	Long:               profileShowLongMarkdown,
	Example:            profileShowUsageMarkdown,
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  profileNameCompletion,
	RunE:               executeProfileShowCommand,
}

func init() {
	defer perf.Track(nil, "profile.init.profileShowCmd")()

	// Format flag.
	profileShowCmd.Flags().StringP("format", "f", "text", "Output format: text, json, yaml")

	// Register flag completion functions.
	if err := profileShowCmd.RegisterFlagCompletionFunc("format", profileShowFormatFlagCompletion); err != nil {
		log.Trace("Failed to register format flag completion", "error", err)
	}

	profileCmd.AddCommand(profileShowCmd)
}

// profileShowFormatFlagCompletion provides shell completion for the format flag.
func profileShowFormatFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "profile.profileShowFormatFlagCompletion")()

	return []string{"text", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
}

// profileNameCompletion provides shell completion for profile names.
func profileNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "profile.profileNameCompletion")()

	// Don't complete if we already have a profile name.
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	manager := profile.NewProfileManager()
	profiles, err := manager.ListProfiles(&atmosConfig)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, p := range profiles {
		names = append(names, p.Name)
	}

	return names, cobra.ShellCompDirectiveNoFileComp
}

// executeProfileShowCommand handles the profile show command execution.
func executeProfileShowCommand(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "profile.executeProfileShowCommand")()

	profileName := args[0]

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

	// Get profile details.
	profileInfo, err := getProfileInfo(&atmosConfig, profileName)
	if err != nil {
		return err
	}

	// Render and print output.
	output, err := renderProfileOutput(profileInfo, format)
	if err != nil {
		return err
	}

	fmt.Print(output)

	return nil
}

// getProfileInfo retrieves profile information with enhanced error handling.
func getProfileInfo(atmosConfig *schema.AtmosConfiguration, profileName string) (*profile.ProfileInfo, error) {
	defer perf.Track(nil, "profile.getProfileInfo")()

	manager := profile.NewProfileManager()
	profileInfo, err := manager.GetProfile(atmosConfig, profileName)
	if err != nil {
		if errors.Is(err, errUtils.ErrProfileNotFound) {
			return nil, buildProfileNotFoundError(profileName)
		}
		return nil, err
	}

	return profileInfo, nil
}

// buildProfileNotFoundError creates a detailed error for profile not found scenarios.
func buildProfileNotFoundError(profileName string) error {
	return errUtils.Build(errUtils.ErrProfileNotFound).
		WithExplanationf("Profile `%s` does not exist in any configured location", profileName).
		WithExplanation("The profile directory was not found in any of the search locations").
		WithHint("Run `atmos profile list` to see all available profiles").
		WithHint("Check the spelling of the profile name").
		WithHint("Verify `profiles.base_path` in `atmos.yaml` if using custom location").
		WithContext("profile", profileName).
		WithContext("command", "profile show").
		WithExitCode(2).
		Err()
}

// renderProfileOutput renders profile information in the specified format.
func renderProfileOutput(profileInfo *profile.ProfileInfo, format string) (string, error) {
	defer perf.Track(nil, "profile.renderProfileOutput")()

	switch format {
	case "json":
		return renderProfileJSON(profileInfo)
	case "yaml":
		return renderProfileYAML(profileInfo)
	case "text":
		return profileShow.RenderProfile(profileInfo)
	default:
		return "", buildInvalidFormatError(format)
	}
}

// buildInvalidFormatError creates a detailed error for invalid format scenarios.
func buildInvalidFormatError(format string) error {
	return errUtils.Build(errUtils.ErrInvalidFormat).
		WithExplanationf("The format `%s` is not supported for this command", format).
		WithExplanation("Only `text`, `json`, and `yaml` formats are available").
		WithHint("Use `--format text`, `--format json`, or `--format yaml`").
		WithHint("Example: `atmos profile show dev --format text`").
		WithContext("format", format).
		WithContext("command", "profile show").
		WithContext("supported_formats", "text, json, yaml").
		WithExitCode(2).
		Err()
}

// renderProfileJSON renders a profile as JSON.
func renderProfileJSON(p *profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.renderProfileJSON")()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", errUtils.Build(errUtils.ErrOutputFormat).
			WithExplanationf("Failed to generate JSON output: `%s`", err).
			WithExplanation("This is likely an internal error").
			WithHint("Try using `--format yaml` or `--format text` as a workaround").
			WithHint("Please report this issue if it persists").
			WithContext("format", "json").
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	return string(data) + "\n", nil
}

// renderProfileYAML renders a profile as YAML.
func renderProfileYAML(p *profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "profile.renderProfileYAML")()

	data, err := yaml.Marshal(p)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrOutputFormat).
			WithExplanationf("Failed to generate YAML output: `%s`", err).
			WithExplanation("This is likely an internal error").
			WithHint("Try using `--format json` or `--format text` as a workaround").
			WithHint("Please report this issue if it persists").
			WithContext("format", "yaml").
			WithContext("error", err.Error()).
			WithExitCode(1).
			Err()
	}
	return string(data), nil
}
