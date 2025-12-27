package terraform

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// outputParser handles flag parsing for output command.
var outputParser *flags.StandardParser

// outputCmd represents the terraform output command.
var outputCmd = &cobra.Command{
	Use:   "output",
	Short: "Show output values from your root module",
	Long: `Read output variables from the state file.

When --format is specified, retrieves all outputs and formats them in the specified format.
Without --format, passes through to native terraform/tofu output command.

For complete Terraform/OpenTofu documentation, see:
  https://developer.hashicorp.com/terraform/cli/commands/output
  https://opentofu.org/docs/cli/commands/output`,
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()
		if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := outputParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		format := v.GetString("format")
		if format == "" {
			return terraformRun(terraformCmd, cmd, args)
		}
		return outputRunWithFormat(cmd, args, format)
	},
}

// outputRunWithFormat executes terraform output with atmos formatting.
func outputRunWithFormat(cmd *cobra.Command, args []string, format string) error {
	defer perf.Track(nil, "terraform.outputRunWithFormat")()

	if err := validateOutputFormat(format); err != nil {
		return err
	}
	info, atmosConfig, err := prepareOutputContext(cmd, args)
	if err != nil {
		return err
	}
	return executeOutputWithFormat(atmosConfig, info, format)
}

// validateOutputFormat checks if the format is supported.
func validateOutputFormat(format string) error {
	if !slices.Contains(tfoutput.SupportedFormats, format) {
		return errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanationf("Invalid --format value %q.", format).
			WithHintf("Supported formats: %s.", strings.Join(tfoutput.SupportedFormats, ", ")).
			Err()
	}
	return nil
}

// prepareOutputContext validates config and prepares component info.
func prepareOutputContext(cmd *cobra.Command, args []string) (*schema.ConfigAndStacksInfo, *schema.AtmosConfiguration, error) {
	if err := internal.ValidateAtmosConfig(); err != nil {
		return nil, nil, err
	}
	separatedArgs := compat.GetSeparated()
	argsWithSubCommand := append([]string{"output"}, args...)
	info, err := e.ProcessCommandLineArgs(cfg.TerraformComponentType, terraformCmd, argsWithSubCommand, separatedArgs)
	if err != nil {
		return nil, nil, err
	}
	if err := resolveAndPromptForArgs(&info, cmd); err != nil {
		return nil, nil, err
	}
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
		ComponentFromArg:        info.ComponentFromArg,
		Stack:                   info.Stack,
	}
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, nil, errUtils.Build(errUtils.ErrInitializeCLIConfig).WithCause(err).Err()
	}
	return &info, &atmosConfig, nil
}

// executeOutputWithFormat retrieves and formats terraform outputs.
func executeOutputWithFormat(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, format string) error {
	v := viper.GetViper()
	skipInit := v.GetBool("skip-init")
	outputFile := v.GetString("output-file")
	uppercase := v.GetBool("uppercase")
	flatten := v.GetBool("flatten")

	outputs, err := tfoutput.GetComponentOutputs(atmosConfig, info.ComponentFromArg, info.Stack, skipInit)
	if err != nil {
		return errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithCause(err).
			WithExplanationf("Failed to get terraform outputs for component %q in stack %q.", info.ComponentFromArg, info.Stack).
			Err()
	}

	// Build format options.
	opts := tfoutput.FormatOptions{
		Uppercase: uppercase,
		Flatten:   flatten,
	}

	// Check if a specific output name was requested (in AdditionalArgsAndFlags).
	outputName := extractOutputName(info.AdditionalArgsAndFlags)
	var formatted string
	if outputName != "" {
		formatted, err = formatSingleOutput(outputs, outputName, format, opts)
	} else {
		formatted, err = tfoutput.FormatOutputsWithOptions(outputs, tfoutput.Format(format), opts)
	}
	if err != nil {
		return err
	}

	if outputFile != "" {
		return tfoutput.WriteToFile(outputFile, formatted)
	}
	return data.Write(formatted)
}

// extractOutputName extracts the output name from additional args.
// Output name is a positional arg that doesn't start with "-".
func extractOutputName(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

// formatSingleOutput formats a single output value.
func formatSingleOutput(outputs map[string]any, outputName, format string, opts tfoutput.FormatOptions) (string, error) {
	value, exists := outputs[outputName]
	if !exists {
		return "", errUtils.Build(errUtils.ErrTerraformOutputFailed).
			WithExplanationf("Output %q not found.", outputName).
			WithHint("Use 'atmos terraform output <component> -s <stack>' without an output name to see all available outputs.").
			Err()
	}
	return tfoutput.FormatSingleValueWithOptions(outputName, value, tfoutput.Format(format), opts)
}

func init() {
	outputParser = flags.NewStandardParser(
		flags.WithStringFlag("format", "f", "", "Output format: json, yaml, hcl, env, dotenv, bash, csv, tsv"),
		flags.WithStringFlag("output-file", "o", "", "Write output to file instead of stdout"),
		flags.WithBoolFlag("uppercase", "u", false, "Convert keys to uppercase (useful for env vars)"),
		flags.WithBoolFlag("flatten", "", false, "Flatten nested maps into key_subkey format"),
		flags.WithEnvVars("format", "ATMOS_TERRAFORM_OUTPUT_FORMAT"),
		flags.WithEnvVars("output-file", "ATMOS_TERRAFORM_OUTPUT_FILE"),
		flags.WithEnvVars("uppercase", "ATMOS_TERRAFORM_OUTPUT_UPPERCASE"),
		flags.WithEnvVars("flatten", "ATMOS_TERRAFORM_OUTPUT_FLATTEN"),
	)
	outputParser.RegisterFlags(outputCmd)
	if err := outputParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	if err := outputCmd.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return tfoutput.SupportedFormats, cobra.ShellCompDirectiveNoFileComp
	}); err != nil {
		_ = err
	}
	RegisterTerraformCompletions(outputCmd)
	internal.RegisterCommandCompatFlags("terraform", "output", OutputCompatFlags())
	terraformCmd.AddCommand(outputCmd)
}
