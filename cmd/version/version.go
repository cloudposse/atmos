package version

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
	atmosConfigPtr *schema.AtmosConfiguration
	// VersionParser handles flag parsing with Viper precedence.
	versionParser *flags.StandardParser
)

// VersionOptions contains parsed flags for the version command.
type VersionOptions struct {
	global.Flags
	Check  bool
	Format string
}

// SetAtmosConfig sets the Atmos configuration for the version command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Display the version of Atmos you are running and check for updates",
	Long:    `This command shows the version of the Atmos CLI you are currently running and checks if a newer version is available. Use this command to verify your installation and ensure you are up to date.`,
	Example: "atmos version",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "version.RunE")()

		// Parse flags using new options pattern.
		// Reuse global Viper to preserve env/config precedence
		v := viper.GetViper()
		if err := versionParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts, err := parseVersionOptions(cmd, v, args)
		if err != nil {
			return err
		}

		return exec.NewVersionExec(atmosConfigPtr).Execute(opts.Check, opts.Format)
	},
}

// parseVersionOptions parses command flags into VersionOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parseVersionOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*VersionOptions, error) {
	return &VersionOptions{
		Flags:  flags.ParseGlobalFlags(cmd, v),
		Check:  v.GetBool("check"),
		Format: v.GetString("format"),
	}, nil
}

func init() {
	// Create parser with version-specific flags using functional options.
	versionParser = flags.NewStandardParser(
		flags.WithBoolFlag("check", "c", false, "Run additional checks after displaying version info"),
		flags.WithStringFlag("format", "", "", "Specify the output format"),
		flags.WithEnvVars("check", "ATMOS_VERSION_CHECK"),
		flags.WithEnvVars("format", "ATMOS_VERSION_FORMAT"),
	)

	// Register flags using the standard RegisterFlags method.
	// RegisterFlags() does NOT set DisableFlagParsing, allowing Cobra to
	// validate flags normally and reject unknown flags like --non-existent.
	versionParser.RegisterFlags(versionCmd)

	// Bind flags to Viper for environment variable support.
	if err := versionParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&VersionCommandProvider{})
}

// VersionCommandProvider implements the CommandProvider interface.
type VersionCommandProvider struct{}

// GetCommand returns the version command.
func (v *VersionCommandProvider) GetCommand() *cobra.Command {
	return versionCmd
}

// GetName returns the command name.
func (v *VersionCommandProvider) GetName() string {
	return "version"
}

// GetGroup returns the command group for help organization.
func (v *VersionCommandProvider) GetGroup() string {
	return "Other Commands"
}

// GetFlagsBuilder returns the flags builder for this command.
// Version command uses StandardParser for its flags.
func (v *VersionCommandProvider) GetFlagsBuilder() flags.Builder {
	return versionParser
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
// Version command has no positional arguments.
func (v *VersionCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
// Version command has no compatibility flags (uses native Cobra flags only).
func (v *VersionCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
// Version command has no aliases.
func (v *VersionCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (v *VersionCommandProvider) IsExperimental() bool {
	return false
}
