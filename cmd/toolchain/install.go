package toolchain

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	maxConcurrencyFlagName = "max-concurrency"
	maxConcurrencyEnvVar   = "ATMOS_TOOLCHAIN_MAX_CONCURRENCY"
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
		flags.WithIntFlag(maxConcurrencyFlagName, "", 0, "Maximum number of tools to install concurrently (default 4)"),
		flags.WithEnvVars("reinstall", "ATMOS_TOOLCHAIN_REINSTALL"),
		flags.WithEnvVars("default", "ATMOS_TOOLCHAIN_DEFAULT"),
		flags.WithEnvVars(maxConcurrencyFlagName, maxConcurrencyEnvVar),
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
	maxConcurrency, err := resolveInstallMaxConcurrency(cmd)
	if err != nil {
		return err
	}

	// No args: install from .tool-versions file.
	if len(args) == 0 {
		if defaultVersion {
			ui.Warning("--default flag is ignored when installing multiple tools")
		}
		return toolchain.RunInstallFromToolVersions(reinstall, true, maxConcurrency)
	}

	// Single tool: use single-tool flow.
	if len(args) == 1 {
		return toolchain.RunInstall(args[0], defaultVersion, reinstall, true, true)
	}

	// Multiple tools: use batch install.
	// Note: --default flag is ignored for batch installs (only applies to single-tool installs).
	if defaultVersion {
		ui.Warning("--default flag is ignored when installing multiple tools")
	}
	return toolchain.RunInstallBatchWithOptions(args, toolchain.BatchInstallOptions{
		Reinstall:      reinstall,
		MaxConcurrency: maxConcurrency,
	})
}

func resolveInstallMaxConcurrency(cmd *cobra.Command) (int, error) {
	_, envSet := os.LookupEnv(maxConcurrencyEnvVar)
	return resolveInstallMaxConcurrencyFromSources(cmd, viper.GetViper(), toolchain.GetAtmosConfig(), envSet)
}

// resolveInstallMaxConcurrencyFromSources resolves the configured worker count.
// The CLI flag wins over the environment variable, which wins over atmos.yaml.
func resolveInstallMaxConcurrencyFromSources(cmd *cobra.Command, v *viper.Viper, config *schema.AtmosConfiguration, envSet bool) (int, error) {
	maxConcurrency := toolchain.DefaultInstallMaxConcurrency
	if config != nil && v.IsSet("toolchain.max_concurrency") {
		maxConcurrency = config.Toolchain.MaxConcurrency
	}
	if envSet {
		maxConcurrency = v.GetInt(maxConcurrencyFlagName)
	}
	if cmd != nil && cmd.Flags().Changed(maxConcurrencyFlagName) {
		maxConcurrency, _ = cmd.Flags().GetInt(maxConcurrencyFlagName)
	}
	if maxConcurrency < 1 {
		return 0, fmt.Errorf("%w: --max-concurrency, %s, and toolchain.max_concurrency must be at least 1", errUtils.ErrInvalidFlagValue, maxConcurrencyEnvVar)
	}
	return maxConcurrency, nil
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
