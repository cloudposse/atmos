package env

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	// DefaultFileMode is the file mode for output files.
	defaultFileMode = 0o644
)

// SupportedFormats lists all supported output formats.
var SupportedFormats = []string{"bash", "json", "dotenv", "github"}

// envCmd outputs environment variables from atmos.yaml.
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Output environment variables configured in atmos.yaml",
	Long:  `Outputs environment variables from the 'env' section of atmos.yaml in various formats suitable for shell evaluation, .env files, JSON consumption, or GitHub Actions workflows.`,
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get output format.
		format, _ := cmd.Flags().GetString("format")
		if !slices.Contains(SupportedFormats, format) {
			return fmt.Errorf("%w: invalid format '%s', supported formats: %s",
				errUtils.ErrInvalidArgumentError, format, strings.Join(SupportedFormats, ", "))
		}

		// Get output file path.
		output, _ := cmd.Flags().GetString("output")

		// Build ConfigAndStacksInfo with CLI overrides (--config, --config-path, --base-path).
		// These are persistent flags inherited from the root command.
		configAndStacksInfo := schema.ConfigAndStacksInfo{}
		if bp, _ := cmd.Flags().GetString("base-path"); bp != "" {
			configAndStacksInfo.AtmosBasePath = bp
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
			return fmt.Errorf("failed to load atmos config: %w", err)
		}

		// Get env vars with original case preserved (Viper lowercases all YAML map keys).
		envVars := atmosConfig.GetCaseSensitiveMap("env")
		if envVars == nil {
			envVars = make(map[string]string)
		}

		// Handle GitHub format special case.
		if format == "github" {
			if output == "" {
				// GITHUB_ENV is an external CI environment variable set by GitHub Actions,
				// not an Atmos configuration variable, so os.Getenv is appropriate here.
				//nolint:forbidigo // GITHUB_ENV is an external CI env var, not Atmos config
				output = os.Getenv("GITHUB_ENV")
				if output == "" {
					return fmt.Errorf("%w: --format=github requires GITHUB_ENV environment variable to be set, or use --output to specify a file path",
						errUtils.ErrRequiredFlagNotProvided)
				}
			}
			return writeEnvToFile(envVars, output, formatGitHub)
		}

		// Handle file output for other formats.
		if output != "" {
			var formatter func(map[string]string) string
			switch format {
			case "bash":
				formatter = formatBash
			case "dotenv":
				formatter = formatDotenv
			case "json":
				// For JSON file output, use the utility function.
				return u.WriteToFileAsJSON(output, envVars, defaultFileMode)
			default:
				formatter = formatBash
			}
			return writeEnvToFile(envVars, output, formatter)
		}

		// Output to stdout.
		switch format {
		case "json":
			return outputEnvAsJSON(&atmosConfig, envVars)
		case "bash":
			return outputEnvAsBash(envVars)
		case "dotenv":
			return outputEnvAsDotenv(envVars)
		default:
			return outputEnvAsBash(envVars)
		}
	},
}

// outputEnvAsJSON outputs environment variables as JSON.
func outputEnvAsJSON(atmosConfig *schema.AtmosConfiguration, envVars map[string]string) error {
	return u.PrintAsJSON(atmosConfig, envVars)
}

// outputEnvAsBash outputs environment variables as shell export statements.
func outputEnvAsBash(envVars map[string]string) error {
	fmt.Print(formatBash(envVars))
	return nil
}

// outputEnvAsDotenv outputs environment variables in .env format.
func outputEnvAsDotenv(envVars map[string]string) error {
	fmt.Print(formatDotenv(envVars))
	return nil
}

// formatBash formats environment variables as shell export statements.
func formatBash(envVars map[string]string) string {
	keys := sortedKeys(envVars)
	var sb strings.Builder
	for _, key := range keys {
		value := envVars[key]
		// Escape single quotes for safe single-quoted shell literals: ' -> '\''.
		safe := strings.ReplaceAll(value, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("export %s='%s'\n", key, safe))
	}
	return sb.String()
}

// formatDotenv formats environment variables in .env format.
func formatDotenv(envVars map[string]string) string {
	keys := sortedKeys(envVars)
	var sb strings.Builder
	for _, key := range keys {
		value := envVars[key]
		// Use the same safe single-quoted escaping as bash output.
		safe := strings.ReplaceAll(value, "'", "'\\''")
		sb.WriteString(fmt.Sprintf("%s='%s'\n", key, safe))
	}
	return sb.String()
}

// formatGitHub formats environment variables for GitHub Actions $GITHUB_ENV file.
// Uses KEY=value format without quoting. For multiline values, GitHub uses heredoc syntax.
func formatGitHub(envVars map[string]string) string {
	keys := sortedKeys(envVars)
	var sb strings.Builder
	for _, key := range keys {
		value := envVars[key]
		// Check if value contains newlines - use heredoc syntax.
		// Use ATMOS_EOF_ prefix to avoid collision with values containing "EOF".
		if strings.Contains(value, "\n") {
			sb.WriteString(fmt.Sprintf("%s<<ATMOS_EOF_%s\n%s\nATMOS_EOF_%s\n", key, key, value, key))
		} else {
			sb.WriteString(fmt.Sprintf("%s=%s\n", key, value))
		}
	}
	return sb.String()
}

// writeEnvToFile writes formatted environment variables to a file (append mode).
func writeEnvToFile(envVars map[string]string, filePath string, formatter func(map[string]string) string) error {
	// Open file in append mode, create if doesn't exist.
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, defaultFileMode)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", filePath, err)
	}
	defer f.Close()

	content := formatter(envVars)
	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to write to file '%s': %w", filePath, err)
	}
	return nil
}

// sortedKeys returns the keys of a map sorted alphabetically.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func init() {
	envCmd.Flags().StringP("format", "f", "bash", "Output format: bash, json, dotenv, github")
	envCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout, or $GITHUB_ENV for github format)")

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
	return nil
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
