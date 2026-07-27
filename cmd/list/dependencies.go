package list

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/list/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
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
	// Tags filters top-level entries by metadata.tags (any-match).
	Tags []string
	// LabelsRaw is the raw --labels flag value (comma-separated key=value or
	// key:value pairs, all-match), parsed in the RunE closure so an invalid
	// value errors early.
	LabelsRaw string
	// Labels holds the parsed form of LabelsRaw; populated from LabelsRaw in
	// the RunE closure after parseDependenciesOptions returns.
	Labels map[string]string
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

		labels, err := tags.ParseLabelsFlag(opts.LabelsRaw)
		if err != nil {
			return err
		}
		opts.Labels = labels

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
		AuthDisabled:     identityName == cfg.IdentityFlagDisabledValue,
		Tags:             tags.ParseTagsFlag(v.GetString("tags")),
		LabelsRaw:        v.GetString("labels"),
	}
}

func init() {
	dependenciesParser = NewListParser(
		WithDependenciesFormatFlag,
		WithDirectionFlag,
		WithStackFlag,
		WithTagsFlag,
		WithLabelsFlag,
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

	graph, describeCtx, err := buildDependencyGraphForCommand(cmd, args, opts)
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
		Tags:      opts.Tags,
		Labels:    opts.Labels,
		LeftDelim: describeCtx.leftDelim(),
	})
	if err != nil {
		return err
	}

	return data.Writeln(output)
}

// dependenciesDescribeContext bundles the config/auth state shared by every
// describe-stacks call this command makes, so ExecuteDescribeStacksWithAuthDisabled
// can be invoked more than once (lightweight structural pass, then an optional
// closure-scoped resolved pass) without redoing config/auth setup each time.
type dependenciesDescribeContext struct {
	atmosConfig  schema.AtmosConfiguration
	authManager  auth.AuthManager
	authDisabled bool
	skip         []string
}

// leftDelim returns the configured left template delimiter ("{{" unless
// 'templates.settings.delimiters' overrides it), used to detect unresolved
// selector templates in lightweight (unevaluated) graphs.
func (c *dependenciesDescribeContext) leftDelim() string {
	left, _ := tags.TemplateDelims(c.atmosConfig.Templates.Settings.Delimiters)
	return left
}

// newDependenciesDescribeContext initializes config and auth once for the command.
func newDependenciesDescribeContext(cmd *cobra.Command, args []string, opts *DependenciesOptions) (*dependenciesDescribeContext, error) {
	configAndStacksInfo, err := e.ProcessCommandLineArgs("list", cmd, args, nil)
	if err != nil {
		return nil, err
	}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	authManager, err := createAuthManagerForList(cmd, &atmosConfig, opts.ProcessTemplates, opts.ProcessFunctions)
	if err != nil {
		return nil, err
	}

	return &dependenciesDescribeContext{
		atmosConfig:  atmosConfig,
		authManager:  authManager,
		authDisabled: opts.AuthDisabled || authManager == nil,
		skip:         skipCredentialBackedYAMLFunctionsForInventory(opts.Skip, authManager),
	}, nil
}

// describeStacks runs one describe-stacks pass filtered to filterByStack (empty
// means every stack), with the given template/YAML-function evaluation enabled.
func (c *dependenciesDescribeContext) describeStacks(filterByStack string, processTemplates, processFunctions bool) (map[string]any, error) {
	stacksMap, err := e.ExecuteDescribeStacksWithAuthDisabled(
		&c.atmosConfig,
		filterByStack,
		nil,
		[]string{cfg.TerraformComponentType},
		nil,
		false, // ignoreMissingFiles
		processTemplates,
		processFunctions,
		false, // includeEmptyStacks
		c.skip,
		c.authManager,
		c.authDisabled,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}
	return stacksMap, nil
}

// buildDependencyGraphForCommand builds the dependency graph for `list
// dependencies`. When --stack/--component bounds the request, evaluation
// (templates/YAML functions/auth) is scoped to only the stacks reachable from
// that root's dependency closure — an unrelated stack's backend is never
// touched, fixing the cross-account failure this command used to hit
// unconditionally. Without a bounding filter there is no closure to scope to
// (the graph genuinely spans the whole repo), so behavior matches the
// historical single full-repo describe call exactly. See
// docs/fixes/2026-07-25-scope-before-evaluate-labels-tags-list-dependencies.md.
func buildDependencyGraphForCommand(cmd *cobra.Command, args []string, opts *DependenciesOptions) (*dependency.Graph, *dependenciesDescribeContext, error) {
	describeCtx, err := newDependenciesDescribeContext(cmd, args, opts)
	if err != nil {
		return nil, nil, err
	}

	bounded := opts.Stack != "" || opts.Component != "" || len(opts.Tags) > 0 || len(opts.Labels) > 0
	needsEvaluation := opts.ProcessTemplates || opts.ProcessFunctions
	eager := e.GetEagerEvaluationSetting(&describeCtx.atmosConfig)
	if !bounded || !needsEvaluation || eager {
		stacksMap, err := describeCtx.describeStacks("", opts.ProcessTemplates, opts.ProcessFunctions)
		if err != nil {
			return nil, nil, err
		}
		graph, err := dependencies.BuildGraph(stacksMap)
		return graph, describeCtx, err
	}

	graph, err := buildScopedDependencyGraph(describeCtx, opts)
	return graph, describeCtx, err
}

// buildScopedDependencyGraph delegates to the shared three-phase scoped
// evaluation in pkg/list/dependencies (dependencies.ResolveScopedClosure):
// lightweight structural pass → reachable closure → resolved passes limited
// to only the stacks the closure touches, re-converging until stable. See
// that function's doc comment for the full behavior and known limitation
// (and describe.settings.eager_evaluation as the fallback).
func buildScopedDependencyGraph(describeCtx *dependenciesDescribeContext, opts *DependenciesOptions) (*dependency.Graph, error) {
	var components []string
	if opts.Component != "" {
		components = []string{opts.Component}
	}
	result, err := dependencies.ResolveScopedClosure(describeCtx.describeStacks, &dependencies.ScopeRequest{
		Components:       components,
		Stack:            opts.Stack,
		Tags:             opts.Tags,
		Labels:           opts.Labels,
		Direction:        dependencies.Direction(opts.Direction),
		ProcessTemplates: opts.ProcessTemplates,
		ProcessFunctions: opts.ProcessFunctions,
		LeftDelim:        describeCtx.leftDelim(),
	})
	if err != nil {
		return nil, err
	}
	return result.Closure, nil
}
