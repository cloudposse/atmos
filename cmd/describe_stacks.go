package cmd

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

var describeStacksParser = flags.NewDescribeStacksParser()

// describeStacksCmd describes configuration for stacks and components in the stacks.
var describeStacksCmd = &cobra.Command{
	Use:   "stacks",
	Short: "Display configuration for Atmos stacks and their components",
	Long:  "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	Args:  cobra.NoArgs,
	RunE: getRunnableDescribeStacksCmd(getRunnableDescribeStacksCmdProps{
		checkAtmosConfig,
		exec.ProcessCommandLineArgs,
		cfg.InitCliConfig, exec.ValidateStacks,
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
	initCliConfig         func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	validateStacks        func(atmosConfig *schema.AtmosConfiguration) error
	newDescribeStacksExec exec.DescribeStacksExec
}

func getRunnableDescribeStacksCmd(
	g getRunnableDescribeStacksCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
		g.checkAtmosConfig()

		// Parse flags using DescribeStacksOptions.
		opts, err := describeStacksParser.Parse(context.Background(), args)
		if err != nil {
			return err
		}

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

		// Build DescribeStacksArgs from parsed options.
		describe := &exec.DescribeStacksArgs{
			FilterByStack:        opts.Stack,
			Format:               opts.Format,
			File:                 opts.File,
			Components:           opts.Components,
			ComponentTypes:       opts.ComponentTypes,
			Sections:             opts.Sections,
			IncludeEmptyStacks:   opts.IncludeEmptyStacks,
			ProcessTemplates:     opts.ProcessTemplates,
			ProcessYamlFunctions: opts.ProcessFunctions,
			Skip:                 opts.Skip,
			Query:                opts.Query,
		}

		// Format validation is now handled by the parser at parse time.
		// Default format is set in the parser as well.

		// Global --pager flag is now handled in cfg.InitCliConfig.

		err = g.newDescribeStacksExec.Execute(&atmosConfig, describe)
		return err
	}
}

func init() {
	describeStacksCmd.DisableFlagParsing = false

	// Register DescribeStacksOptions flags.
	describeStacksParser.RegisterFlags(describeStacksCmd)
	_ = describeStacksParser.BindToViper(viper.GetViper())

	// Add stack completion.
	AddStackCompletion(describeStacksCmd)

	describeCmd.AddCommand(describeStacksCmd)
}
