package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// describeStacksCmd describes configuration for stacks and components in the stacks
var describeStacksCmd = &cobra.Command{
	Use:                "stacks",
	Short:              "Display configuration for Atmos stacks and their components",
	Long:               "This command shows the configuration details for Atmos stacks and the components within those stacks.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Check Atmos configuration
		checkAtmosConfig()
		info, err := exec.ProcessCommandLineArgs("", cmd, args, nil)
		printErrorAndExit(err)
		atmosConfig, err := cfg.InitCliConfig(info, true)
		printErrorAndExit(err)
		err = exec.ValidateStacks(atmosConfig)
		printErrorAndExit(err)
		describe := &exec.DescribeStacksArgs{}
		err = setCliArgsForDescribeStackCli(cmd.Flags(), describe)
		printErrorAndExit(err)
		err = exec.NewDescribeStacksExec().Execute(atmosConfig, describe)
		printErrorAndExit(err)
	},
}

func printErrorAndExit(err error) {
	if err != nil {
		u.PrintErrorMarkdownAndExit("", err, "")
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
			panic(fmt.Sprintf("unsupported type %T for flag %s", v, k))
		}
		checkFlagError(err)
	}
	format := describe.Format
	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'yaml' (default) and 'json'", format)
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
	describeStacksCmd.PersistentFlags().String("components", "", "Filter by specific `atmos` components")

	describeStacksCmd.PersistentFlags().String("component-types", "", "Filter by specific component types. Supported component types: terraform, helmfile")

	describeStacksCmd.PersistentFlags().String("sections", "", "Output only the specified component sections. Available component sections: `backend`, `backend_type`, `deps`, `env`, `inheritance`, `metadata`, `remote_state_backend`, `remote_state_backend_type`, `settings`, `vars`")

	describeStacksCmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing the command")

	describeStacksCmd.PersistentFlags().Bool("include-empty-stacks", false, "Include stacks with no components in the output")

	describeStacksCmd.PersistentFlags().StringSlice("skip", nil, "Skip executing a YAML function in the Atmos stack manifests when executing the command")

	describeCmd.AddCommand(describeStacksCmd)
}
