package env

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	envfmt "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
		output := v.GetString("output")
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

		// Handle JSON format separately (not supported by pkg/env).
		if formatStr == "json" {
			if output != "" {
				return u.WriteToFileAsJSON(output, envVars, 0o644)
			}
			return outputEnvAsJSON(&atmosConfig, envVars)
		}

		// Handle GitHub format special case (requires output path).
		if formatStr == "github" {
			path := output
			if path == "" {
				path = ghactions.GetEnvPath()
				if path == "" {
					return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
						WithExplanation("--format=github requires GITHUB_ENV environment variable to be set, or use --output to specify a file path.").
						Err()
				}
			}
			dataMap := convertToAnyMap(envVars)
			formatted, err := envfmt.FormatData(dataMap, envfmt.FormatGitHub)
			if err != nil {
				return errUtils.Build(errUtils.ErrInvalidArgumentError).
					WithCause(err).
					WithExplanation("Failed to format environment variables for GitHub output.").
					Err()
			}
			return envfmt.WriteToFile(path, formatted)
		}

		// Parse format string to Format type.
		format, err := envfmt.ParseFormat(formatStr)
		if err != nil {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithCause(err).
				WithExplanationf("Invalid --format value %q.", formatStr).
				Err()
		}

		// Format the environment variables with export option.
		dataMap := convertToAnyMap(envVars)
		formatted, err := envfmt.FormatData(dataMap, format, envfmt.WithExport(exportPrefix))
		if err != nil {
			return errUtils.Build(errUtils.ErrInvalidArgumentError).
				WithCause(err).
				WithExplanation("Failed to format environment variables.").
				Err()
		}

		// Output to file or stdout.
		if output != "" {
			return envfmt.WriteToFile(output, formatted)
		}
		return data.Write(formatted)
	},
}

// outputEnvAsJSON outputs environment variables as JSON.
func outputEnvAsJSON(atmosConfig *schema.AtmosConfiguration, envVars map[string]string) error {
	return u.PrintAsJSON(atmosConfig, envVars)
}

// convertToAnyMap converts a map[string]string to map[string]any for use with env formatters.
func convertToAnyMap(envVars map[string]string) map[string]any {
	result := make(map[string]any, len(envVars))
	for k, v := range envVars {
		result[k] = v
	}
	return result
}

func init() {
	// Create parser with env-specific flags using functional options.
	envParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "bash", "Output format: bash, json, dotenv, github"),
		flags.WithStringFlag("output", "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)"),
		flags.WithBoolFlag("export", "", true, "Include 'export' prefix in bash format (default: true)"),
		flags.WithEnvVars("format", "ATMOS_ENV_FORMAT"),
		flags.WithEnvVars("output", "ATMOS_ENV_OUTPUT"),
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
