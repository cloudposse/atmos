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

	graph, err := buildDependencyGraphForCommand(cmd, args, opts)
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

	authManager, err := createAuthManagerForList(cmd, &atmosConfig)
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
func buildDependencyGraphForCommand(cmd *cobra.Command, args []string, opts *DependenciesOptions) (*dependency.Graph, error) {
	describeCtx, err := newDependenciesDescribeContext(cmd, args, opts)
	if err != nil {
		return nil, err
	}

	bounded := opts.Stack != "" || opts.Component != ""
	needsEvaluation := opts.ProcessTemplates || opts.ProcessFunctions
	eager := e.GetEagerEvaluationSetting(&describeCtx.atmosConfig)
	if !bounded || !needsEvaluation || eager {
		stacksMap, err := describeCtx.describeStacks("", opts.ProcessTemplates, opts.ProcessFunctions)
		if err != nil {
			return nil, err
		}
		return dependencies.BuildGraph(stacksMap)
	}

	return buildScopedDependencyGraph(describeCtx, opts)
}

// buildScopedDependencyGraph implements the two-phase scoped evaluation: a
// lightweight structural pass across every stack (component identity plus
// dependencies.components/settings.depends_on edges, which are static in
// practice — see the fix doc's grep of examples/tests), then a closure-scoped
// resolved pass limited to only the stacks the requested root's closure
// touches. Reachability is recomputed against each resolved pass (never reused
// from the lightweight pass) via resolveClosure, so a same-stack templated
// dependency target (e.g. `stack: "{{ .vars.stage }}"` resolving to its own
// stack — a real pattern in this repo's own dependencies-components-inheritance
// fixture) still produces the correct closure.
//
// Known limitation: a dependency target `stack:` value templated to point at a
// DIFFERENT stack the lightweight pass could not discover at all converges
// once that stack is described (resolveClosure expands and redescribes until
// stable), but only within the stacks reachable that way — it cannot discover
// a target stack whose name depends on a value this pass never touches (e.g. a
// remote store lookup). Set settings.describe.settings.eager_evaluation to
// fall back to the historical full-repo-evaluation behavior if needed.
func buildScopedDependencyGraph(describeCtx *dependenciesDescribeContext, opts *DependenciesOptions) (*dependency.Graph, error) {
	lightweightStacks, err := describeCtx.describeStacks("", false, false)
	if err != nil {
		return nil, err
	}

	graph, err := dependencies.BuildGraph(lightweightStacks)
	if err != nil {
		return nil, err
	}
	if graph.Size() == 0 {
		return graph, nil
	}

	direction := dependencies.Direction(opts.Direction)
	roots := dependencies.RootNodeIDs(graph, opts.Component, opts.Stack)
	closure := dependencies.ReachableClosure(graph, roots, direction)
	if closure.Size() == 0 {
		return closure, nil
	}

	return resolveClosure(describeCtx, opts, roots, direction, dependencies.StackNames(closure))
}

// resolveClosure re-describes (templates/YAML functions/auth ON) each stack in
// stackNames, then recomputes the reachable closure against that resolved
// graph. If the resolved edges reveal additional stacks the lightweight pass
// could not see, it describes those too and recomputes again — repeating until
// the touched-stack set stabilizes. This terminates in at most
// len(repo stacks) rounds, since each round only adds genuinely new,
// structurally-discovered stacks.
func resolveClosure(
	describeCtx *dependenciesDescribeContext,
	opts *DependenciesOptions,
	roots []string,
	direction dependencies.Direction,
	stackNames []string,
) (*dependency.Graph, error) {
	resolvedStacks := make(map[string]any)
	described := make(map[string]bool)
	var resolvedGraph *dependency.Graph

	for {
		pending := pendingStacks(stackNames, described)
		if len(pending) == 0 {
			return dependencies.ReachableClosure(resolvedGraph, roots, direction), nil
		}

		for _, stackName := range pending {
			partial, err := describeCtx.describeStacks(stackName, opts.ProcessTemplates, opts.ProcessFunctions)
			if err != nil {
				return nil, err
			}
			for k, v := range partial {
				resolvedStacks[k] = v
			}
			described[stackName] = true
		}

		var err error
		resolvedGraph, err = dependencies.BuildGraph(resolvedStacks)
		if err != nil {
			return nil, err
		}
		stackNames = dependencies.StackNames(dependencies.ReachableClosure(resolvedGraph, roots, direction))
	}
}

// pendingStacks returns the stack names not yet described.
func pendingStacks(stackNames []string, described map[string]bool) []string {
	pending := make([]string, 0, len(stackNames))
	for _, stackName := range stackNames {
		if !described[stackName] {
			pending = append(pending, stackName)
		}
	}
	return pending
}
