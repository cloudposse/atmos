package env

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	envfmt "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/schema"
)

// SupportedFormats lists all supported output formats.
var SupportedFormats = []string{"bash", "json", "dotenv", "github"}

// envParser handles flag parsing with Viper precedence.
var envParser *flags.StandardParser

// envCmd outputs environment variables from atmos.yaml.
// Args validator is auto-applied by the command registry for commands without PositionalArgsBuilder.
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output environment variables configured in atmos.yaml",
	Long:  `Outputs environment variables from the 'env' section of atmos.yaml in various formats suitable for shell evaluation, .env files, JSON consumption, or GitHub Actions workflows.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Parse flags using Viper (respects precedence: flags > env > config > defaults).
		v := viper.GetViper()
		if err := envParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Get output format.
		formatStr := v.GetString("format")
		if !slices.Contains(SupportedFormats, formatStr) {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithExplanationf("Invalid --format value %q.", formatStr).
				WithHintf("Supported formats: %s.", strings.Join(SupportedFormats, ", ")).
				Err()
		}

		// Get output file path and export flag.
		output := v.GetString("output-file")
		exportPrefix := v.GetBool("export")

		// Build ConfigAndStacksInfo with CLI overrides (--config, --config-path, --base-path).
		// These are persistent flags inherited from the root command.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		if bp, _ := cmd.Flags().GetString("base-path"); bp != "" {
			configAndStacksInfo.BasePath = bp
		}
		if cfgFiles, _ := cmd.Flags().GetStringSlice("config"); len(cfgFiles) > 0 {
			configAndStacksInfo.AtmosConfigFilesFromArg = cfgFiles
		}
		if cfgDirs, _ := cmd.Flags().GetStringSlice("config-path"); len(cfgDirs) > 0 {
			configAndStacksInfo.AtmosConfigDirsFromArg = cfgDirs
		}

		// Load atmos configuration (processStacks=false since env command doesn't require stack manifests).
		atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
		if err != nil {
			return errUtils.Build(errUtils.ErrMissingAtmosConfig).
				WithCause(err).
				WithExplanation("Failed to load atmos configuration.").
				WithHint("Ensure atmos.yaml exists and is properly formatted.").
				Err()
		}

		// Get env vars with original case preserved (Viper lowercases all YAML map keys).
		envVars := atmosConfig.GetCaseSensitiveMap("env")
		if envVars == nil {
			envVars = make(map[string]string)
		}

		// Handle GitHub format special case (requires output path).
		if formatStr == "github" && output == "" {
			output = ghactions.GetEnvPath()
			if output == "" {
				return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
					WithExplanation("--format=github requires GITHUB_ENV environment variable to be set, or use --output-file to specify a file path.").
					Err()
			}
		}

		// Use unified env.Output() for all format/output combinations.
		return envfmt.Output(envVars, formatStr, output,
			envfmt.WithAtmosConfig(&atmosConfig),
			envfmt.WithFormatOptions(envfmt.WithExport(exportPrefix)),
		)
	},
}

func init() {
	// Create parser with env-specific flags using functional options.
	envParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "bash", "Output format: bash, json, dotenv, github"),
		flags.WithStringFlag("output-file", "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)"),
		flags.WithBoolFlag("export", "", true, "Include 'export' prefix in bash format (default: true)"),
		flags.WithEnvVars("format", "ATMOS_ENV_FORMAT"),
		flags.WithEnvVars("output-file", "ATMOS_ENV_OUTPUT_FILE"),
		flags.WithEnvVars("export", "ATMOS_ENV_EXPORT"),
	)

	// Register flags using the standard RegisterFlags method.
	envParser.RegisterFlags(envCmd)

	// Bind flags to Viper for environment variable support.
	if err := envParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Register format flag completion.
	if err := envCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return SupportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		// Silently ignore completion registration errors.
		_ = err
	}

	// Register this command with the registry.
	internal.Register(&EnvCommandProvider{})
}

// EnvCommandProvider implements the CommandProvider interface.
type EnvCommandProvider struct{}

// GetCommand returns the env command.
func (e *EnvCommandProvider) GetCommand() *cobra.Command {
	return envCmd
}

// GetName returns the command name.
func (e *EnvCommandProvider) GetName() string {
	return "env"
}

// GetGroup returns the command group for help organization.
func (e *EnvCommandProvider) GetGroup() string {
	return "Configuration Management"
}

// GetFlagsBuilder returns the flags builder for this command.
func (e *EnvCommandProvider) GetFlagsBuilder() flags.Builder {
	return envParser
}

// GetPositionalArgsBuilder returns the positional args builder for this command.
func (e *EnvCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

// GetCompatibilityFlags returns compatibility flags for this command.
func (e *EnvCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

// GetAliases returns command aliases.
func (e *EnvCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

// IsExperimental returns whether this command is experimental.
func (e *EnvCommandProvider) IsExperimental() bool {
	return false
}
