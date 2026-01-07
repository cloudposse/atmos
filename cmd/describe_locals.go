package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeLocalsCmd describes locals for stacks.
var describeLocalsCmd = &cobra.Command{
	Use:   "locals",
	Short: "Display locals from Atmos stack manifests",
	Long:  "This command displays the locals defined in Atmos stack manifests.",
	Example: `  # Show all locals
  atmos describe locals

  # Show locals for a specific stack
  atmos describe locals --stack deploy/dev

  # Output as JSON
  atmos describe locals --format json

  # Query specific values
  atmos describe locals --query '.["deploy/dev"].merged.namespace'`,
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE: getRunnableDescribeLocalsCmd(getRunnableDescribeLocalsCmdProps{
		checkAtmosConfig:       checkAtmosConfig,
		processCommandLineArgs: exec.ProcessCommandLineArgs,
		initCliConfig:          cfg.InitCliConfig,
		validateStacks:         exec.ValidateStacks,
		newDescribeLocalsExec:  exec.NewDescribeLocalsExec(),
	}),
}

type getRunnableDescribeLocalsCmdProps struct {
	checkAtmosConfig       func(opts ...AtmosValidateOption)
	processCommandLineArgs func(
		componentType string,
		cmd *cobra.Command,
		args []string,
		additionalArgsAndFlags []string,
	) (schema.ConfigAndStacksInfo, error)
	initCliConfig         func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	validateStacks        func(atmosConfig *schema.AtmosConfiguration) error
	newDescribeLocalsExec exec.DescribeLocalsExec
}

func getRunnableDescribeLocalsCmd(
	g getRunnableDescribeLocalsCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		// Check Atmos configuration.
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

		describeArgs := &exec.DescribeLocalsArgs{}
		err = setCliArgsForDescribeLocalsCli(cmd.Flags(), describeArgs)
		if err != nil {
			return err
		}

		err = g.newDescribeLocalsExec.Execute(&atmosConfig, describeArgs)
		return err
	}
}

func setCliArgsForDescribeLocalsCli(flags *pflag.FlagSet, args *exec.DescribeLocalsArgs) error {
	var err error

	if flags.Changed("stack") {
		args.FilterByStack, err = flags.GetString("stack")
		if err != nil {
			return err
		}
	}

	if flags.Changed("format") {
		args.Format, err = flags.GetString("format")
		if err != nil {
			return err
		}
	}

	if flags.Changed("file") {
		args.File, err = flags.GetString("file")
		if err != nil {
			return err
		}
	}

	if flags.Changed("query") {
		args.Query, err = flags.GetString("query")
		if err != nil {
			return err
		}
	}

	// Set default format.
	if args.Format == "" {
		args.Format = "yaml"
	}

	return nil
}

func init() {
	describeLocalsCmd.DisableFlagParsing = false

	describeLocalsCmd.PersistentFlags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)",
	)
	AddStackCompletion(describeLocalsCmd)

	describeLocalsCmd.PersistentFlags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")

	describeLocalsCmd.PersistentFlags().String("file", "", "Write the result to file")

	describeLocalsCmd.PersistentFlags().StringP("query", "q", "", "Query the result using `yq` expression")

	describeCmd.AddCommand(describeLocalsCmd)
}
