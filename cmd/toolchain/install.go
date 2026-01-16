package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/toolchain"
)

var installParser *flags.StandardParser

var installCmd = &cobra.Command{
	Use:   "install [tool...]",
	Short: "Install CLI binaries from the registry",
	Long: `Install one or more CLI binaries using metadata from the registry.

The tool(s) should be specified in the format: owner/repo@version

Examples:
  atmos toolchain install hashicorp/terraform@1.5.0
  atmos toolchain install opentofu@1.6.0 tflint@0.50.0 kubectl@1.29.0
`,
	Args:          cobra.ArbitraryArgs,
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

	reinstall := v.GetBool("reinstall")
	defaultVersion := v.GetBool("default")

	// No args: install from .tool-versions file.
	if len(args) == 0 {
		return toolchain.RunInstall("", defaultVersion, reinstall, true, true)
	}

	// Single tool: use single-tool flow.
	if len(args) == 1 {
		return toolchain.RunInstall(args[0], defaultVersion, reinstall, true, true)
	}

	// Multiple tools: use batch install.
	// Note: --default flag is ignored for batch installs (only applies to single-tool installs).
	if defaultVersion {
		_ = ui.Warning("--default flag is ignored when installing multiple tools")
	}
	return toolchain.RunInstallBatch(args, reinstall)
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
