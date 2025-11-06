package cmd

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/describe"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeDependentsParser = flags.NewStandardOptionsBuilder().
	WithStack(true).
	WithFormat([]string{"json", "yaml"}, "json").
	WithFile().
	WithQuery().
	WithProcessTemplates(true).
	WithProcessFunctions(true).
	WithSkip().
	WithPositionalArgs(describe.NewDependentsPositionalArgsBuilder().
		WithComponent(true). // Required component argument.
		Build()).
	Build()

type describeDependentExecCreator func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:     "dependents <component>",
	Aliases: []string{"dependants"},
	Short:   "List Atmos components that depend on a given component",
	Long:    "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	// Positional args are validated by the StandardParser using the builder pattern.
	Args:              cobra.ArbitraryArgs,
	ValidArgsFunction: ComponentsArgCompletion,
	RunE: getRunnableDescribeDependentsCmd(
		checkAtmosConfig,
		cfg.InitCliConfig,
		exec.NewDescribeDependentsExec),
}

func getRunnableDescribeDependentsCmd(
	checkAtmosConfig func(opts ...AtmosValidateOption),
	initCliConfig func(info schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error),
	newDescribeDependentsExec describeDependentExecCreator,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration
		checkAtmosConfig()

		// Parse flags and positional args using builder pattern.
		// Component is extracted by builder pattern into opts.Component field.
		opts, err := describeDependentsParser.Parse(cmd.Context(), args)
		if err != nil {
			return err
		}

		info := schema.ConfigAndStacksInfo{}
		atmosConfig, err := initCliConfig(info, false)
		if err != nil {
			return err
		}

		// Get identity from flag and create AuthManager if provided.
		identityName := GetIdentityFromFlags(cmd, os.Args)
		authManager, err := CreateAuthManagerFromIdentity(identityName, &atmosConfig.Auth)
		if err != nil {
			return err
		}

		describe := &exec.DescribeDependentsExecProps{
			Format:               opts.Format,
			File:                 opts.File,
			Stack:                opts.Stack,
			Query:                opts.Query,
			ProcessTemplates:     opts.ProcessTemplates,
			ProcessYamlFunctions: opts.ProcessYamlFunctions,
			Skip:                 opts.Skip,
			Component:            opts.Component,
			AuthManager:          authManager,
		}

		// Global --pager flag is now handled in cfg.InitCliConfig.

		err = newDescribeDependentsExec(&atmosConfig).Execute(describe)
		return err
	}
}

func init() {
	// Register StandardOptions flags (includes required stack flag via WithStack(true)).
	describeDependentsParser.RegisterFlags(describeDependentsCmd)
	_ = describeDependentsParser.BindToViper(viper.GetViper())

	// Register stack completion after flags are registered.
	_ = describeDependentsCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)

	describeCmd.AddCommand(describeDependentsCmd)
}
