package toolchain

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/toolchain"
)

var supportedFormats = []string{"bash", "json", "dotenv", "fish", "powershell"}

var envParser *flags.StandardParser

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Export PATH entries for installed tools in shell-specific format",
	Long:  `Export PATH environment variable for all tools configured in .tool-versions, formatted for different shells.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Bind flags to Viper for precedence handling.
		v := viper.GetViper()
		if err := envParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		format := v.GetString("format")
		if !slices.Contains(supportedFormats, format) {
			return fmt.Errorf("%w: invalid format: %s (supported: %v)", errUtils.ErrInvalidArgumentError, format, supportedFormats)
		}

		relativeFlag := v.GetBool("relative")

		return toolchain.EmitEnv(format, relativeFlag)
	},
}

func init() {
	// Create parser with env-specific flags.
	envParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "bash", fmt.Sprintf("Output format: %v", supportedFormats)),
		flags.WithBoolFlag("relative", "", false, "Use relative paths instead of absolute"),
		flags.WithEnvVars("format", "ATMOS_TOOLCHAIN_ENV_FORMAT"),
		flags.WithEnvVars("relative", "ATMOS_TOOLCHAIN_RELATIVE"),
	)

	// Register flags.
	envParser.RegisterFlags(envCmd)

	// Bind flags to Viper.
	if err := envParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register shell completion for format flag.
	if err := envCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return supportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		panic(err)
	}
}

// EnvCommandProvider implements the CommandProvider interface.
type EnvCommandProvider struct{}

func (e *EnvCommandProvider) GetCommand() *cobra.Command {
	return envCmd
}

func (e *EnvCommandProvider) GetName() string {
	return "env"
}

func (e *EnvCommandProvider) GetGroup() string {
	return "Toolchain Commands"
}

func (e *EnvCommandProvider) GetFlagsBuilder() flags.Builder {
	return envParser
}

func (e *EnvCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (e *EnvCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}
