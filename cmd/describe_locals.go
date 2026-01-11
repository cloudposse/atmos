package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// describeLocalsCmd describes locals for stacks.
var describeLocalsCmd = &cobra.Command{
	Use:   "locals [component] -s <stack>",
	Short: "Display locals from Atmos stack manifests",
	Long: `This command displays the locals defined in Atmos stack manifests.

When called with --stack, it shows the locals defined in that stack manifest file.
When a component is also specified, it shows the merged locals that would be
available to that component (global + section-specific + component-level).`,
	Example: `  # Show locals for a specific stack
  atmos describe locals --stack deploy/dev

  # Show locals for a specific stack (using logical stack name)
  atmos describe locals -s prod-us-east-1

  # Show locals for a component in a stack
  atmos describe locals vpc -s prod

  # Output as JSON
  atmos describe locals --stack dev --format json

  # Query specific values
  atmos describe locals -s deploy/dev --query '.["deploy/dev"].locals.namespace'`,
	Args: cobra.MaximumNArgs(1),
	RunE: getRunnableDescribeLocalsCmd(getRunnableDescribeLocalsCmdProps{
		checkAtmosConfig:             checkAtmosConfig,
		processCommandLineArgs:       exec.ProcessCommandLineArgs,
		initCliConfig:                cfg.InitCliConfig,
		validateStacks:               exec.ValidateStacks,
		newDescribeLocalsExecFactory: exec.NewDescribeLocalsExec,
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
	initCliConfig                func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error)
	validateStacks               func(atmosConfig *schema.AtmosConfiguration) error
	newDescribeLocalsExecFactory func() exec.DescribeLocalsExec
}

func getRunnableDescribeLocalsCmd(
	g getRunnableDescribeLocalsCmdProps,
) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "cmd.describeLocals")()

		// Check Atmos configuration.
		g.checkAtmosConfig()

		// Pass args to ensure config-selection flags (--base-path, --config, etc.) are parsed.
		// The component positional arg is extracted separately below.
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

		// Extract component from positional argument if provided.
		if len(args) > 0 {
			describeArgs.Component = args[0]
		}

		err = setCliArgsForDescribeLocalsCli(cmd.Flags(), describeArgs)
		if err != nil {
			return err
		}

		// Fail fast if --stack is not specified (required).
		if describeArgs.FilterByStack == "" {
			if describeArgs.Component != "" {
				return errUtils.ErrStackRequiredWithComponent
			}
			return errUtils.ErrStackRequired
		}

		// Create executor lazily to avoid init-time side effects.
		executor := g.newDescribeLocalsExecFactory()
		err = executor.Execute(&atmosConfig, describeArgs)
		return err
	}
}

func setCliArgsForDescribeLocalsCli(flags *pflag.FlagSet, args *exec.DescribeLocalsArgs) error {
	var err error

	if flags.Changed("stack") {
		args.FilterByStack, err = flags.GetString("stack")
		if err != nil {
			return fmt.Errorf("%w: read --stack: %w", errUtils.ErrInvalidFlag, err)
		}
	}

	if flags.Changed("format") {
		args.Format, err = flags.GetString("format")
		if err != nil {
			return fmt.Errorf("%w: read --format: %w", errUtils.ErrInvalidFlag, err)
		}
	}

	if flags.Changed("file") {
		args.File, err = flags.GetString("file")
		if err != nil {
			return fmt.Errorf("%w: read --file: %w", errUtils.ErrInvalidFlag, err)
		}
	}

	if flags.Changed("query") {
		args.Query, err = flags.GetString("query")
		if err != nil {
			return fmt.Errorf("%w: read --query: %w", errUtils.ErrInvalidFlag, err)
		}
	}

	// Set default format.
	if args.Format == "" {
		args.Format = "yaml"
	}

	return nil
}

func init() {
	// Use Flags() instead of PersistentFlags() since this command has no subcommands.
	describeLocalsCmd.Flags().StringP("stack", "s", "",
		"Filter by a specific stack\n"+
			"The filter supports names of the top-level stack manifests (including subfolder paths), and `atmos` stack names (derived from the context vars)\n"+
			"Note: YAML parse errors are reported when filtering by stack; otherwise they are silently skipped during batch processing",
	)
	AddStackCompletion(describeLocalsCmd)

	describeLocalsCmd.Flags().StringP("format", "f", "yaml", "Specify the output format (`yaml` is default)")

	describeLocalsCmd.Flags().String("file", "", "Write the result to file")

	describeLocalsCmd.Flags().StringP("query", "q", "", "Query the result using `yq` expression")

	describeCmd.AddCommand(describeLocalsCmd)
}
