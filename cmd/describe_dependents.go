package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

type describeDependentExecCreator func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:                "dependents",
	Aliases:            []string{"dependants"},
	Short:              "List Atmos components that depend on a given component",
	Long:               "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.ExactArgs(1),
	ValidArgsFunction:  ComponentsArgCompletion,
	RunE: getRunnableDescribeDependentsCmd(
		checkAtmosConfig,
		exec.ProcessCommandLineArgs,
		cfg.InitCliConfig,
		exec.NewDescribeDependentsExec),
}

func getRunnableDescribeDependentsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	processCommandLineArgs func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	newDescribeDependentsExec describeDependentExecCreator,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		info, err := processCommandLineArgs("terraform", cmd, args, nil)
		if err != nil {
			return err
		}

		atmosConfig, err := initCliConfig(info, true)
		if err != nil {
			return err
		}

		describe := &exec.DescribeDependentsExecProps{}
		err = setFlagsForDescribeDependentsCmd(cmd.Flags(), describe)
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

		describe.Component = args[0]
		err = newDescribeDependentsExec(&atmosConfig).Execute(describe)
		return err
	}
}

func setFlagsForDescribeDependentsCmd(flags *pflag.FlagSet, describe *exec.DescribeDependentsExecProps) error {
	err := setStringFlagIfChanged(flags, "format", &describe.Format)
	if err != nil {
		return err
	}

	err = setStringFlagIfChanged(flags, "file", &describe.File)
	if err != nil {
		return err
	}

	err = setStringFlagIfChanged(flags, "stack", &describe.Stack)
	if err != nil {
		return err
	}

	err = setStringFlagIfChanged(flags, "query", &describe.Query)
	if err != nil {
		return err
	}

	// `true` by default
	describe.ProcessTemplates = true
	err = setBoolFlagIfChanged(flags, "process-templates", &describe.ProcessTemplates)
	if err != nil {
		return err
	}

	// `true` by default
	describe.ProcessYamlFunctions = true
	err = setBoolFlagIfChanged(flags, "process-functions", &describe.ProcessYamlFunctions)
	if err != nil {
		return err
	}

	err = setSliceOfStringsFlagIfChanged(flags, "skip", &describe.Skip)
	if err != nil {
		return err
	}

	if describe.Format == "" {
		describe.Format = "json"
	}

	if describe.Format != "json" && describe.Format != "yaml" {
		return ErrInvalidFormat
	}

	return nil
}

func init() {
	describeDependentsCmd.DisableFlagParsing = false

	AddStackCompletion(describeDependentsCmd)
	describeDependentsCmd.PersistentFlags().StringP("format", "f", "json", "The output format (`json` is default)")
	describeDependentsCmd.PersistentFlags().String("file", "", "Write the result to the file")
	describeDependentsCmd.PersistentFlags().StringP("query", "q", "", "Filter the output using a YQ expression")
	describeDependentsCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")
	describeDependentsCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")
	describeDependentsCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function when processing Atmos stack manifests")

	err := describeDependentsCmd.MarkPersistentFlagRequired("stack")
	errUtils.CheckErrorPrintAndExit(err, "", "")

	describeCmd.AddCommand(describeDependentsCmd)
}
