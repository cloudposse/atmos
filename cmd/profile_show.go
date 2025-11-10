package cmd

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

//go:embed markdown/atmos_profile_show_usage.md
var profileShowUsageMarkdown string

// profileShowCmd shows detailed information about a specific profile.
var profileShowCmd = &cobra.Command{
	Use:   "show <profile-name>",
	Short: "Show detailed information about a profile",
	Long: `Show detailed information about a specific configuration profile.

Displays:
- Profile location and type
- Metadata (name, description, version, tags)
- List of configuration files
- Usage instructions

Supports multiple output formats:
- **text** (default): human-readable formatted output
- **json**/**yaml**: structured data for programmatic access`,
	Example:            profileShowUsageMarkdown,
	Args:               cobra.ExactArgs(1),
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	ValidArgsFunction:  profileNameCompletion,
	RunE:               executeProfileShowCommand,
}

func init() {
	defer perf.Track(nil, "cmd.init.profileShowCmd")()

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
	defer perf.Track(nil, "cmd.profileShowFormatFlagCompletion")()

	return []string{"text", "json", "yaml"}, cobra.ShellCompDirectiveNoFileComp
}

// profileNameCompletion provides shell completion for profile names.
func profileNameCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	defer perf.Track(nil, "cmd.profileNameCompletion")()

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
	defer perf.Track(nil, "cmd.executeProfileShowCommand")()

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

	// Create profile manager.
	manager := profile.NewProfileManager()

	// Get profile details.
	profileInfo, err := manager.GetProfile(&atmosConfig, profileName)
	if err != nil {
		// Only add "not found" hint if this is specifically a ProfileNotFound error.
		if errors.Is(err, errUtils.ErrProfileNotFound) {
			return fmt.Errorf("%w: profile '%s' not found (run 'atmos profile list' to see available profiles)", errUtils.ErrProfileNotFound, profileName)
		}
		// Return other errors unchanged to preserve their context.
		return err
	}

	// Render output based on format.
	var output string
	switch format {
	case "json":
		output, err = renderProfileJSON(profileInfo)
	case "yaml":
		output, err = renderProfileYAML(profileInfo)
	case "text":
		output, err = profileShow.RenderProfile(profileInfo)
	default:
		return fmt.Errorf("%w: unsupported format '%s' (supported: text, json, yaml)", errUtils.ErrInvalidFormat, format)
	}

	if err != nil {
		return err
	}

	fmt.Print(output)

	return nil
}

// renderProfileJSON renders a profile as JSON.
func renderProfileJSON(p *profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "cmd.renderProfileJSON")()

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal profile to JSON: %s", errUtils.ErrOutputFormat, err)
	}
	return string(data) + "\n", nil
}

// renderProfileYAML renders a profile as YAML.
func renderProfileYAML(p *profile.ProfileInfo) (string, error) {
	defer perf.Track(nil, "cmd.renderProfileYAML")()

	data, err := yaml.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("%w: failed to marshal profile to YAML: %s", errUtils.ErrOutputFormat, err)
	}
	return string(data), nil
}
