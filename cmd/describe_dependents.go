package cmd

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeDependentsParser = flags.NewStandardOptionsBuilder().
	WithStack(true).
	WithFormat("json", "json", "yaml").
	WithFile().
	WithQuery().
	WithProcessTemplates(true).
	WithProcessFunctions(true).
	WithSkip().
	Build()

type describeDependentExecCreator func(atmosConfig *schema.AtmosConfiguration) exec.DescribeDependentsExec

// describeDependentsCmd produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component
var describeDependentsCmd = &cobra.Command{
	Use:               "dependents",
	Aliases:           []string{"dependants"},
	Short:             "List Atmos components that depend on a given component",
	Long:              "This command generates a list of Atmos components within stacks that depend on the specified Atmos component.",
	Args:              cobra.ExactArgs(1),
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

		opts, err := describeDependentsParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

		info := schema.ConfigAndStacksInfo{}
		atmosConfig, err := initCliConfig(info, true)
		if err != nil {
			return err
		}

		// Format validation is now handled by the parser at parse time.

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
			Component:            opts.GetPositionalArgs()[0],
			AuthManager:          authManager,
		}

		// Global --pager flag is now handled in cfg.InitCliConfig.

		err = newDescribeDependentsExec(&atmosConfig).Execute(describe)
		return err
	}
}

func init() {
	describeDependentsCmd.DisableFlagParsing = true // IMPORTANT: Manual parsing required for our unified parser

	// Register StandardOptions flags (includes required stack flag via WithStack(true)).
	describeDependentsParser.RegisterFlags(describeDependentsCmd)
	_ = describeDependentsParser.BindToViper(viper.GetViper())

	// Register stack completion after flags are registered.
	_ = describeDependentsCmd.RegisterFlagCompletionFunc("stack", stackFlagCompletion)

	describeCmd.AddCommand(describeDependentsCmd)
}
