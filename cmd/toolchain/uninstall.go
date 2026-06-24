package toolchain

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

var uninstallParser *flags.StandardParser

var uninstallCmd = &cobra.Command{
	Use:   "uninstall [tool@version]",
	Short: "Uninstall a tool or all tools from .tool-versions",
	Long: `Uninstall a specific tool version or all tools listed in .tool-versions.

Use --all to uninstall every tool found in the install directory,
including tools installed via component dependencies.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUninstall,
}

func init() {
	// Create parser with uninstall-specific flags.
	uninstallParser = flags.NewStandardParser(
		flags.WithBoolFlag("all", "", false, "Uninstall all installed tools (not just .tool-versions)"),
		flags.WithEnvVars("all", "ATMOS_TOOLCHAIN_UNINSTALL_ALL"),
	)

	// Register flags.
	uninstallParser.RegisterFlags(uninstallCmd)

	// Bind flags to Viper.
	if err := uninstallParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func runUninstall(cmd *cobra.Command, args []string) error {
	// Bind flags to Viper for precedence handling.
	v := viper.GetViper()
	if err := uninstallParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	uninstallAll := v.GetBool("all")

	toolSpec := ""
	if len(args) > 0 {
		toolSpec = args[0]
	}

	return toolchain.RunUninstall(toolSpec, uninstallAll)
}

// UninstallCommandProvider implements the CommandProvider interface.
type UninstallCommandProvider struct{}

func (u *UninstallCommandProvider) GetCommand() *cobra.Command {
	return uninstallCmd
}

func (u *UninstallCommandProvider) GetName() string {
	return "uninstall"
}

func (u *UninstallCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (u *UninstallCommandProvider) GetFlagsBuilder() flags.Builder {
	return uninstallParser
}

func (u *UninstallCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (u *UninstallCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
