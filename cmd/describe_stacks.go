package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Display configuration for Atmos stacks and their components",
	Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		checkAtmosConfig,
		exec.ProcessCommandLineArgs,
		cfg.InitCliConfig, exec.ValidateStacks,
		setCliArgsForDescribeStackCli,
		exec.NewDescribeStacksExec(),
	}),
}

type getRunnableDescribeStacksCmdProps struct {
	checkAtmosConfig       func(opts ...AtmosValidateOption)
	processCommandLineArgs func(
		componentType string,
		cmd *cobra.Command,
		args []string,
		additionalArgsAndFlags []string,
	) (schema.ConfigAndStacksInfo, error)
	initCliConfig                 func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	validateStacks                func(atmosConfig *schema.AtmosConfiguration) error
	setCliArgsForDescribeStackCli func(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error
	newDescribeStacksExec         exec.DescribeStacksExec
}

func getRunnableDescribeStacksCmd(
	g getRunnableDescribeStacksCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		g.checkAtmosConfig()

		info, err := g.processCommandLineArgs("", cmd, args, nil)
		if err != nil {
			return err
		}

		atmosConfig, err := g.initCliConfig(info, true)
		if err != nil {
			return err
		}

		err = g.validateStacks(&atmosConfig)
		if err != nil {
			return err
		}

		describe := &exec.DescribeStacksArgs{}
		err = setCliArgsForDescribeStackCli(cmd.Flags(), describe)
		if err != nil {
			return err
		}

		// Get identity from flag and create AuthManager if provided.
		// Use the WithAtmosConfig variant to enable stack-level default identity scanning.
		identityName := GetIdentityFromFlags(cmd, os.Args)
		authManager, err := CreateAuthManagerFromIdentityWithAtmosConfig(identityName, &atmosConfig.Auth, &atmosConfig)
		if err != nil {
			return err
		}
		describe.AuthManager = authManager

		// Global --pager flag is now handled in cfg.InitCliConfig

		err = g.newDescribeStacksExec.Execute(&atmosConfig, describe)
		return err
	}
}

func setCliArgsForDescribeStackCli(flags *pflag.FlagSet, describe *exec.DescribeStacksArgs) error {
	flagsKeyValue := map[string]any{
		"stack":                &describe.FilterByStack,
		"format":               &describe.Format,
		"file":                 &describe.File,
		"include-empty-stacks": &describe.IncludeEmptyStacks,
		"components":           &describe.Components,
		"component-types":      &describe.ComponentTypes,
		"sections":             &describe.Sections,
		"process-templates":    &describe.ProcessTemplates,
		"process-functions":    &describe.ProcessYamlFunctions,
		"query":                &describe.Query,
		"skip":                 &describe.Skip,
	}

	// `true` by default.
	describe.ProcessTemplates = true
	describe.ProcessYamlFunctions = true

	var err error
	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
		default:
			er := fmt.Errorf("unsupported type %T for flag %s", v, k)
			errUtils.CheckErrorPrintAndExit(er, "", "")
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	return validateFormat(describe)
}

func validateFormat(describe *exec.DescribeStacksArgs) error {
	format := describe.Format
	if format != "" && format != "yaml" && format != "json" {
		return exec.ErrInvalidFormat
	}
	if format == "" {
		format = "yaml"
	}
	describe.Format = format
	return nil
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	describeStacksCmd.PersistentFlags().String("file", "", "Write the result to file")

	describeStacksCmd.PersistentFlags().String("format", "yaml", "Specify the output format (`yaml` is default)")

	describeStacksCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)",
	)
	AddStackCompletion(describeStacksCmd)
	describeStacksCmd.PersistentFlags().StringSlice("components", nil, "Filter by specific `atmos` components")

	describeStacksCmd.PersistentFlags().StringSlice("component-types", nil, "Filter by specific component types. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().StringSlice("sections", nil, "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")

	describeStacksCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output")

	describeStacksCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

	describeCmd.AddCommand(describeStacksCmd)
}
