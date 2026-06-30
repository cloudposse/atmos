package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

var dependenciesParser *flags.StandardParser

// DependenciesOptions contains parsed flags for the dependencies command.
type DependenciesOptions struct {
	global.Flags
	Format           string
	Direction        string
	Stack            string
	Component        string
	ProcessTemplates bool
	ProcessFunctions bool
	Skip             []string
	AuthDisabled     bool
}

// dependenciesCmd lists Atmos component dependencies as a tree.
var dependenciesCmd = &cobra.Command{
	Use:   "dependencies [component]",
	Short: "List Atmos component dependencies as a tree",
	Long: `List the dependency relationships between Atmos components across stacks.

By default the output is a tree showing both directions for every component:
what each component depends on, and what depends on it. Use --direction to show
only one side, --stack and the optional [component] argument to focus on a single
component, and --format to emit json or yaml instead of a tree.`,
	Aliases:            []string{"deps"},
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.MaximumNArgs(1),
	Example: "atmos list dependencies\n" +
		"atmos list dependencies --stack plat-ue2-dev\n" +
		"atmos list dependencies vpc --stack plat-ue2-dev\n" +
		"atmos list dependencies --direction forward\n" +
		"atmos list dependencies --format json",
	RunE: func(cmd *cobra.Command, args []string) error {
		v := viper.GetViper()

		if err := checkAtmosConfig(cmd, v); err != nil {
			return err
		}

		if err := dependenciesParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := parseDependenciesOptions(cmd, v, args)

		return executeListDependenciesCmd(cmd, args, opts)
	},
}

// parseDependenciesOptions maps viper state and positional args into a
// DependenciesOptions struct. Extracted so the mapping can be unit-tested
// without driving the whole cobra command.
func parseDependenciesOptions(cmd *cobra.Command, v *viper.Viper, args []string) *DependenciesOptions {
	identityName := getIdentityFromCommand(cmd)

	var component string
	if len(args) > 0 {
		component = args[0]
	}

	return &DependenciesOptions{
		Flags:            flags.ParseGlobalFlags(cmd, v),
		Format:           v.GetString("format"),
		Direction:        v.GetString("direction"),
		Stack:            v.GetString("stack"),
		Component:        component,
		ProcessTemplates: v.GetBool("process-templates"),
		ProcessFunctions: v.GetBool("process-functions"),
		Skip:             v.GetStringSlice("skip"),
		AuthDisabled:     identityName == "" || identityName == cfg.IdentityFlagDisabledValue,
	}
}

func init() {
	dependenciesParser = NewListParser(
		WithDependenciesFormatFlag,
		WithDirectionFlag,
		WithStackFlag,
		WithProcessTemplatesFlag,
		WithProcessFunctionsFlag,
		WithSkipFlag,
	)

	dependenciesParser.RegisterFlags(dependenciesCmd)

	if err := dependenciesParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}

func executeListDependenciesCmd(cmd *cobra.Command, args []string, opts *DependenciesOptions) error {
	defer perf.Track(nil, "list.executeListDependenciesCmd")()

	stacksMap, err := describeStacksForDependencies(cmd, args, opts)
	if err != nil {
		return err
	}

	graph, err := dependencies.BuildGraph(stacksMap)
	if err != nil {
		return err
	}

	if graph.Size() == 0 {
		ui.Info("No components found")
		return nil
	}

	output, err := dependencies.Render(graph, dependencies.Options{
		Format:    opts.Format,
		Direction: dependencies.Direction(opts.Direction),
		Component: opts.Component,
		Stack:     opts.Stack,
	})
	if err != nil {
		return err
	}

	return data.Writeln(output)
}

// describeStacksForDependencies initializes config and auth and returns the
// described terraform stacks map used to build the dependency graph.
func describeStacksForDependencies(cmd *cobra.Command, args []string, opts *DependenciesOptions) (map[string]any, error) {
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, err
	}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
	if err != nil {
		return nil, err
	}

	stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(
		&atmosConfig,
		"", // all stacks; filtering is applied during rendering so cross-stack edges resolve.
		nil,
		[]string{cfg.TerraformComponentType},
		nil,
		false, // ignoreMissingFiles
		opts.ProcessTemplates,
		opts.ProcessFunctions,
		false, // includeEmptyStacks
		skipCredentialBackedYAMLFunctionsForInventory(opts.Skip, authManager),
		authManager,
		opts.AuthDisabled || authManager == nil,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	return stacksMap, nil
}
