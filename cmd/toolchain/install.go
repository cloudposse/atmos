package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var installParser *flags.StandardParser

var installCmd = &cobra.Command{
	Use:   "install [tool]",
	Short: "Install a CLI binary from the registry",
	Long: `Install a CLI binary using metadata from the registry.

The tool should be specified in the format: owner/repo@version
`,
	Args:          cobra.MaximumNArgs(1),
	RunE:          runInstall,
	SilenceUsage:  true, // Don't show usage on error.
	SilenceErrors: true, // Don't show errors twice.
}

func init() {
	// Create parser with install-specific flags.
	installParser = flags.NewStandardParser(
		flags.WithBoolFlag("reinstall", "", false, "Reinstall even if already installed"),
		flags.WithBoolFlag("default", "", false, "Set as default version in .tool-versions"),
		flags.WithEnvVars("reinstall", "ATMOS_TOOLCHAIN_REINSTALL"),
		flags.WithEnvVars("default", "ATMOS_TOOLCHAIN_DEFAULT"),
	)

	// Register flags.
	installParser.RegisterFlags(installCmd)

	// Bind flags to Viper.
	if err := installParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runInstall(cmd *cobra.Command, args []string) error {
	// Bind flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := installParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	toolSpec := ""
	if len(args) > 0 {
		toolSpec = args[0]
	}

	reinstall := v.GetBool("reinstall")
	defaultVersion := v.GetBool("default")

	return toolchain.RunInstall(toolSpec, defaultVersion, reinstall)
}

// InstallCommandProvider implements the CommandProvider interface.
type InstallCommandProvider struct{}

func (i *InstallCommandProvider) GetCommand() *cobra.Command {
	return installCmd
}

func (i *InstallCommandProvider) GetName() string {
	return "install"
}

func (i *InstallCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (i *InstallCommandProvider) GetFlagsBuilder() flags.Builder {
	return installParser
}

func (i *InstallCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (i *InstallCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
