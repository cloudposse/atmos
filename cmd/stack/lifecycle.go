package stack

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	authdeferred "github.com/cloudposse/atmos/pkg/auth/deferred"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/hooks"
	ioLayer "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/scheduler"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
)

//go:embed markdown/plan_usage.md
var stackPlanUsage string

//go:embed markdown/test_usage.md
var stackTestUsage string

const flagSkipHooks = "skip-hooks"

type lifecycleOptions struct {
	verb, stack, identity, failureMode, logOrder         string
	components                                           []string
	pickComponents                                       bool
	all, affected, dryRun, allowDestroy, autoApprove, ci bool
	concurrency, dependencies, dependents                int
	tags                                                 []string
	labels                                               map[string]string
}

func init() {
	stackCmd.AddCommand(newLifecycleCommand("plan"), newLifecycleCommand("test"), newLifecycleCommand("deploy"), newLifecycleCommand("destroy"))
}

// affectedOnlyFlags configure the Git comparison and are meaningless without --affected.
var affectedOnlyFlags = []string{"base", "ref", "sha", "repo-path", "ssh-key", "ssh-key-password", "clone-target-ref"}

func buildLifecycleParser(verb string) *flags.StandardParser {
	options := []flags.Option{
		flags.WithStackFlag(), flags.WithDryRunFlag(), flags.WithIdentityFlag(),
		flags.WithStringSliceFlag("component", "", nil, "Select component names (repeat or comma-separate); omit the value to choose interactively"),
		flags.WithNoOptDefVal("component", lifecycleComponentSelect),
		flags.WithBoolFlag("all", "", false, "Select all components defining this operation"),
		flags.WithBoolFlag("affected", "", false, "Select components affected by the Git comparison"),
		flags.WithStringFlag("base", "", "", "Base Git ref or SHA (auto-detected in supported CI environments)"),
		flags.WithStringFlag("ref", "", "", "Git reference to compare against"),
		flags.WithStringFlag("sha", "", "", "Git SHA to compare against"),
		flags.WithStringFlag("repo-path", "", "", "Existing repository to use as the comparison baseline"),
		flags.WithStringFlag("ssh-key", "", "", "SSH key for cloning the comparison baseline"),
		flags.WithStringFlag("ssh-key-password", "", "", "SSH key password for cloning the comparison baseline"),
		flags.WithBoolFlag("clone-target-ref", "", false, "Clone the comparison baseline"),
		flags.WithStringFlag(flags.FlagIncludeDependencies, "", "", "Include prerequisites; use --include-dependencies=N to limit depth"),
		flags.WithNoOptDefVal(flags.FlagIncludeDependencies, flags.ClosureDepthUnlimited),
		flags.WithStringFlag(flags.FlagIncludeDependents, "", "", "Include consumers; use --include-dependents=N to limit depth"),
		flags.WithNoOptDefVal(flags.FlagIncludeDependents, flags.ClosureDepthUnlimited),
		flags.WithIntFlag("max-concurrency", "", 1, "Maximum concurrent component operations (one worker permits interactive prompts)"),
		flags.WithStringFlag("failure-mode", "", "fail-fast", "Failure handling: fail-fast or keep-going"),
		flags.WithStringFlag("log-order", "", "stream", "Component output order: stream or grouped (grouped applies when --max-concurrency > 1)"),
		flags.WithStringFlag(flagSkipHooks, "", "", "Skip all hooks or a comma-separated list of hook names"),
		flags.WithNoOptDefVal(flagSkipHooks, "*"),
		flags.WithEnvVars(flagSkipHooks, "ATMOS_SKIP_HOOKS"),
		flags.WithBoolFlag("auto-approve", "", false, "Pass the registered handler's automatic approval arguments where supported"),
		flags.WithBoolFlag(flagCI, "", false, "Enable CI mode for automated pipelines (writes job summary, outputs)"),
		flags.WithEnvVars(flagCI, "ATMOS_CI", "CI"),
		flags.WithStringSliceFlag("tags", "", nil, "Filter by tags (comma-separated, matches any): --tags=production,tier-1"),
		flags.WithEnvVars("tags", "ATMOS_TAGS"),
		flags.WithViperKey("tags", lifecycleTagsKey),
		flags.WithStringArrayFlag("labels", "", nil, "Filter by labels (comma-separated key=value or key:value pairs within an occurrence, and/or repeated, matches all): --labels=cost-center=platform,compliance=sox"),
		flags.WithEnvVars("labels", "ATMOS_LABELS"),
		flags.WithViperKey("labels", lifecycleLabelsKey),
	}
	if verb == "deploy" {
		options = append(options, flags.WithBoolFlag("allow-destroy", "", false, "Request deleted-component teardown (blocked until isolated recovery is available)"))
	}
	return flags.NewStandardParser(options...)
}

func lifecycleShort(verb string) string {
	if verb == "destroy" {
		return "Destroy stack components in reverse dependency order"
	}
	return strings.ToUpper(verb[:1]) + verb[1:] + " stack components in dependency order"
}

// lifecycleArgs rejects positional components in favor of the repeatable flag.
func lifecycleArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return errUtils.Build(fmt.Errorf("%w: atmos stack %s does not accept positional arguments", errUtils.ErrInvalidPositionalArgs, cmd.Name())).
		WithHint("Use --component=NAME, repeat --component, or comma-separate names.").
		Err()
}

func newLifecycleCommand(verb string) *cobra.Command {
	parser := buildLifecycleParser(verb)
	cmd := &cobra.Command{
		Use: verb, Short: lifecycleShort(verb), Args: lifecycleArgs,
		Long: lifecycleShort(verb) + ".\n\n" +
			"Select a whole stack with --stack, or narrow it with --component=app,api.\n" +
			"Repeat --component to add names; use --component without a value for a\n" +
			"multiselect picker. Without --stack, --all, or --affected, choose a stack\n" +
			"interactively. Use --all or --affected to select across stacks.",
		// Children are top-level Atmos invocations that open their own CI log groups.
		Annotations: map[string]string{annotationSkipInvocationGroup: "true"},
	}
	switch verb {
	case "plan":
		cmd.Example = stackPlanUsage
	case "test":
		cmd.Example = stackTestUsage
	}
	parser.RegisterFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		opts, err := resolveLifecycleOptions(cmd, parser, verb)
		if err != nil {
			return err
		}
		return runLifecycle(cmd, &opts)
	}
	internal.RegisterCommandMetadata(
		cmd,
		internal.WithDocumentationURL("https://atmos.tools/cli/commands/stack/stack-"+verb),
	)
	return cmd
}

// resolveLifecycleOptions parses the command's options through a private viper
// instance. The global instance carries env bindings other commands register on
// the same keys (for example ATMOS_ALL or ATMOS_TERRAFORM_DESTROY_ALL), which
// must never change what this command selects.
func resolveLifecycleOptions(cmd *cobra.Command, parser *flags.StandardParser, verb string) (lifecycleOptions, error) {
	v := viper.New()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return lifecycleOptions{}, err
	}
	opts, err := parseLifecycleOptions(verb, v)
	if err != nil {
		return opts, err
	}
	if err := parseLifecycleComponents(cmd, v, &opts); err != nil {
		return opts, err
	}
	opts.ci = lifecycleCIMode(cmd, v)
	return opts, checkAffectedOnlyFlags(cmd, &opts)
}

// Namespaced Viper keys for the selector flags keep a job-level ATMOS_TAGS or
// ATMOS_LABELS from reaching this command through AutomaticEnv; the variables
// still apply through their explicit bindings. The namespace must not match a
// flag name: "stack" is the --stack value, so "stack.*" keys could not nest.
const (
	lifecycleTagsKey   = "lifecycle.tags"
	lifecycleLabelsKey = "lifecycle.labels"
)

// parseLifecycleSelectors reads the --tags and --labels filters with the same
// grammar bulk Terraform commands use.
func parseLifecycleSelectors(v *viper.Viper) ([]string, map[string]string, error) {
	selectedTags := tags.ParseTagsFlag(strings.Join(v.GetStringSlice(lifecycleTagsKey), ","))
	labels, err := tags.ParseLabelsFlag(tags.ReadLabelsFlagKey(v, lifecycleLabelsKey))
	return selectedTags, labels, err
}

// checkLifecycleSelection rejects incompatible bulk selections.
func checkLifecycleSelection(o *lifecycleOptions) error {
	if o.all && o.affected {
		return fmt.Errorf("%w: specify exactly one of --all or --affected", errUtils.ErrInvalidFlag)
	}
	return nil
}

// checkAffectedOnlyFlags rejects comparison flags that would be silently ignored.
func checkAffectedOnlyFlags(cmd *cobra.Command, opts *lifecycleOptions) error {
	if opts.affected {
		return nil
	}
	for _, name := range affectedOnlyFlags {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("%w: --%s requires --affected", errUtils.ErrInvalidFlag, name)
		}
	}
	return nil
}

func parseLifecycleOptions(verb string, v *viper.Viper) (lifecycleOptions, error) {
	o := lifecycleOptions{verb: verb, stack: v.GetString("stack"), identity: cfg.NormalizeIdentityValue(v.GetString("identity")), all: v.GetBool("all"), affected: v.GetBool("affected"), dryRun: v.GetBool("dry-run"), allowDestroy: verb == "deploy" && v.GetBool("allow-destroy"), autoApprove: v.GetBool("auto-approve"), concurrency: v.GetInt("max-concurrency"), failureMode: v.GetString("failure-mode"), logOrder: v.GetString("log-order")}
	var err error
	if o.tags, o.labels, err = parseLifecycleSelectors(v); err != nil {
		return o, err
	}
	if err := checkLifecycleSelection(&o); err != nil {
		return o, err
	}
	if o.allowDestroy && !o.affected {
		return o, fmt.Errorf("%w: --allow-destroy requires --affected", errUtils.ErrInvalidFlag)
	}
	if err := validateLifecycleChoices(&o); err != nil {
		return o, err
	}
	o.dependencies, err = flags.ParseClosureDepth(flags.FlagIncludeDependencies, v.GetString(flags.FlagIncludeDependencies))
	if err != nil {
		return o, err
	}
	o.dependents, err = flags.ParseClosureDepth(flags.FlagIncludeDependents, v.GetString(flags.FlagIncludeDependents))
	return o, err
}

// validateLifecycleChoices reports each invalid execution flag by name and value.
func validateLifecycleChoices(o *lifecycleOptions) error {
	if o.concurrency < 1 {
		return fmt.Errorf("%w: --max-concurrency must be at least 1, got %d", errUtils.ErrInvalidFlag, o.concurrency)
	}
	if !slices.Contains([]string{"fail-fast", "keep-going"}, o.failureMode) {
		return fmt.Errorf("%w: invalid --failure-mode %q (use fail-fast or keep-going)", errUtils.ErrInvalidFlag, o.failureMode)
	}
	if !slices.Contains([]string{"stream", "grouped"}, o.logOrder) {
		return fmt.Errorf("%w: invalid --log-order %q (use stream or grouped)", errUtils.ErrInvalidFlag, o.logOrder)
	}
	return nil
}

func runLifecycle(cmd *cobra.Command, opts *lifecycleOptions) error {
	if err := checkLifecyclePromptAvailability(opts); err != nil {
		return err
	}
	config, err := loadLifecycleConfig(cmd)
	if err != nil {
		return err
	}
	manager := authdeferred.NewManager(authdeferred.AuthOptions{Config: &config, Disabled: opts.identity == cfg.IdentityFlagDisabledValue || opts.dryRun})
	// Only graph selectors are needed now. Vars, hooks, and runtime output
	// references remain unevaluated until the selected child starts. Custom
	// component kinds are registered before stacks are described.
	stacks, commands, err := loadLifecycleInventory(cmd, &config, opts.verb, manager)
	if err != nil {
		return err
	}
	if err := resolveLifecycleTargets(cmd, opts, stacks, commands); err != nil {
		return err
	}
	selection, affected, err := resolveLifecycleSelection(cmd, &config, opts, stacks)
	if err != nil {
		return err
	}
	left, _ := tags.TemplateDelims(config.Templates.Settings.Delimiters)
	plan, err := lifecycle.BuildPlan(stacks, commands, &selection, left)
	if err != nil {
		return err
	}
	applyLifecycleApproval(plan, opts.autoApprove)
	globalArgs := lifecycleGlobalArgs(cmd, &config, opts)
	return executeLifecyclePlans(cmd, &lifecycleRun{config: &config, plan: plan, globalArgs: globalArgs, options: opts}, affected)
}

func applyLifecycleApproval(plan *lifecycle.Plan, approve bool) {
	if approve {
		for id, node := range plan.Graph.Nodes {
			plan.Commands[id] = append(plan.Commands[id], plan.Handlers[node.Type].ApprovalArgs...)
		}
	}
}

// loadLifecycleConfig loads the Atmos configuration and every stack manifest path.
func loadLifecycleConfig(cmd *cobra.Command) (schema.AtmosConfiguration, error) {
	info, err := e.ProcessCommandLineArgs("", cmd, nil, nil)
	if err != nil {
		return schema.AtmosConfiguration{}, err
	}
	config, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return config, err
	}
	// Unlike single-component commands, an empty inventory is a successful
	// selection, including when every stack was removed in the comparison.
	config.StackConfigFilesAbsolutePaths, config.StackConfigFilesRelativePaths, _, err = cfg.FindAllStackConfigsInPathsForStack(
		config, "", config.IncludeStackAbsolutePaths, config.ExcludeStackAbsolutePaths,
	)
	if err != nil && !errors.Is(err, errUtils.ErrNoStackManifestsFound) {
		return config, err
	}
	return config, nil
}

// resolveLifecycleSelection builds the selection, including the affected comparison when requested.
func resolveLifecycleSelection(cmd *cobra.Command, config *schema.AtmosConfiguration, opts *lifecycleOptions, stacks map[string]any) (lifecycle.Selection, []schema.Affected, error) {
	selection := lifecycle.Selection{Operation: lifecycle.Event(opts.verb), Stack: opts.stack, Components: opts.components, Dependencies: opts.dependencies, Dependents: opts.dependents, Reverse: opts.verb == "destroy"}
	if len(opts.tags) > 0 || len(opts.labels) > 0 {
		selection.Filter = &lifecycle.Filter{Tags: opts.tags, Labels: opts.labels}
	}
	var affected []schema.Affected
	if opts.affected {
		var err error
		affected, err = compareLifecycleAffected(cmd, config, opts)
		if err != nil {
			return selection, nil, err
		}
		selection.Affected = append([]schema.Affected{}, affected...)
	}
	if err := checkLifecycleStackExists(stacks, affected, opts.stack); err != nil {
		return selection, nil, err
	}
	if err := checkLifecycleComponentExists(stacks, affected, opts); err != nil {
		return selection, nil, err
	}
	return selection, affected, nil
}

// checkLifecycleStackExists fails when --stack names a stack that appears
// neither in the inventory nor in the affected comparison. A stack that exists
// but has nothing selected is a valid, empty selection.
func checkLifecycleStackExists(stacks map[string]any, affected []schema.Affected, stack string) error {
	if stack == "" {
		return nil
	}
	if _, ok := stacks[stack]; ok {
		return nil
	}
	for i := range affected {
		if affected[i].Stack == stack {
			return nil
		}
	}
	return errUtils.Build(fmt.Errorf("%w: %q", errUtils.ErrStackNotFound, stack)).
		WithHint("Run `atmos list stacks` to see the available stacks.").
		Err()
}

// checkLifecycleApproval fails before any child starts when concurrent workers
// cannot answer an approval prompt: stdin is detached above one worker.
func checkLifecycleApproval(plan *lifecycle.Plan, opts *lifecycleOptions) error {
	if opts.dryRun || opts.autoApprove || opts.concurrency <= 1 {
		return nil
	}
	var prompting []string
	for _, node := range plan.Graph.Nodes {
		if handler := plan.Handlers[node.Type]; handler != nil && len(handler.ApprovalArgs) > 0 {
			prompting = append(prompting, lifecycle.Label(node))
		}
	}
	if len(prompting) == 0 {
		return nil
	}
	slices.Sort(prompting)
	return errUtils.Build(fmt.Errorf("%w: --max-concurrency=%d cannot answer approval prompts for %s", errUtils.ErrInvalidFlag, opts.concurrency, strings.Join(prompting, ", "))).
		WithHint("Pass --auto-approve to skip the prompts, or use --max-concurrency=1 to approve interactively.").
		Err()
}

// lifecycleTally separates execution outcomes, exclusions, and definitions awaiting teardown.
func lifecycleTally(results []scheduler.Result, excluded, deleted int) string {
	counts := map[scheduler.Status]int{}
	for i := range results {
		counts[results[i].Status]++
	}
	summary := fmt.Sprintf("%d succeeded, %d failed, %d skipped, %d excluded", counts[scheduler.StatusSucceeded], counts[scheduler.StatusFailed], counts[scheduler.StatusSkipped], excluded)
	if deleted > 0 {
		summary += fmt.Sprintf(", %d deleted awaiting teardown", deleted)
	}
	return summary
}

// reportLifecycleResults prints one status line per component, then the tally.
func reportLifecycleResults(verb string, results []scheduler.Result, excluded, deleted int) {
	const statusFormat = "%s %s: %s"
	// Scheduler errors identify nodes by their collision-safe IDs. Render the
	// same readable component labels as the status lines instead.
	labels := make([]string, 0, len(results)*2)
	for i := range results {
		if results[i].NodeID != "" {
			labels = append(labels, results[i].NodeID, lifecycle.Label(&results[i].Node))
		}
	}
	replacer := strings.NewReplacer(labels...)
	for i := range results {
		r := &results[i]
		switch r.Status {
		case scheduler.StatusSucceeded:
			ui.Successf(statusFormat, verb, lifecycle.Label(&r.Node), r.Status)
		case scheduler.StatusFailed:
			ui.Errorf(statusFormat, verb, lifecycle.Label(&r.Node), r.Status)
		case scheduler.StatusSkipped:
			ui.Warningf(statusFormat, verb, lifecycle.Label(&r.Node), lifecycleSkippedStatus(r.Err, replacer))
		default:
			ui.Warningf(statusFormat, verb, lifecycle.Label(&r.Node), r.Status)
		}
	}
	ui.Info(lifecycleTally(results, excluded, deleted))
}

// lifecycleSkippedStatus preserves the scheduler's reason without repeating
// its skipped sentinel or exposing internal node IDs.
func lifecycleSkippedStatus(err error, labels *strings.Replacer) string {
	if err == nil {
		return string(scheduler.StatusSkipped)
	}
	reason := err.Error()
	if errors.Is(err, errUtils.ErrNodeSkipped) {
		reason = strings.TrimPrefix(reason, errUtils.ErrNodeSkipped.Error()+": ")
	}
	return string(scheduler.StatusSkipped) + ": " + labels.Replace(reason)
}

func compareLifecycleAffected(cmd *cobra.Command, config *schema.AtmosConfiguration, opts *lifecycleOptions) ([]schema.Affected, error) {
	args := &e.DescribeAffectedCmdArgs{CLIConfig: config, AuthDisabled: opts.identity == cfg.IdentityFlagDisabledValue}
	e.SetDescribeAffectedFlagValueInCliArgs(cmd.Flags(), args)
	if args.RepoPath != "" && (args.Base != "" || args.Ref != "" || args.SHA != "" || args.SSHKeyPath != "" || args.SSHKeyPassword != "" || args.CloneTargetRef) {
		return nil, fmt.Errorf("%w: --repo-path cannot be combined with ref, SHA, or clone options", errUtils.ErrInvalidFlag)
	}
	// Discovery is unfiltered so closure and surviving-consumer checks retain
	// cross-stack edges. Runtime functions are resolved by each child operation.
	args.Stack, args.IncludeDependents, args.ProcessYamlFunctions = "", false, false
	// Comparing declarations avoids invoking runtime output functions embedded
	// in Go templates. Children resolve those values at their actual start time.
	args.ProcessTemplates = false
	return e.GetAffectedComponents(args)
}

// forwardedLifecycleIdentity keeps the interactive and disabled sentinels intact and
// canonicalizes named identities before they are forwarded to children.
func forwardedLifecycleIdentity(config *schema.AuthConfig, identity string) string {
	if identity == cfg.IdentityFlagSelectValue || identity == cfg.IdentityFlagDisabledValue {
		return identity
	}
	return canonicalLifecycleIdentity(config, identity)
}

func lifecycleGlobalArgs(cmd *cobra.Command, config *schema.AtmosConfiguration, opts *lifecycleOptions) []string {
	args := []string{"--base-path=" + config.BasePathAbsolute}
	if opts.identity != "" {
		args = append(args, "--identity="+forwardedLifecycleIdentity(&config.Auth, opts.identity))
	}
	for _, name := range []string{"config", "config-path", "profile"} {
		f := cmd.Flag(name)
		if f == nil || !f.Changed {
			continue
		}
		values, _ := cmd.Flags().GetStringSlice(name)
		for _, value := range values {
			value = lifecycleConfigArgument(name, value)
			args = append(args, "--"+name+"="+value)
		}
	}
	for _, name := range []string{"edition", "settings-list-merge-strategy", "logs-level", "no-color", "mask"} {
		if f := cmd.Flag(name); f != nil && f.Changed {
			args = append(args, "--"+name+"="+f.Value.String())
		}
	}
	return args
}

func lifecycleConfigArgument(name, value string) string {
	if name == "profile" {
		return value
	}
	if absolute, err := filepath.Abs(value); err == nil {
		return absolute
	}
	return value
}

func lifecycleEnvironment(cmd *cobra.Command, ciMode bool) []string {
	env := withCIMode(os.Environ(), ciMode)
	if skip := hooks.ResolveSkipHooks(cmd); skip != "" || cmd.Flags().Changed(flagSkipHooks) {
		env = slices.DeleteFunc(env, func(entry string) bool { return strings.HasPrefix(entry, "ATMOS_SKIP_HOOKS=") })
		env = append(env, "ATMOS_SKIP_HOOKS="+skip)
	}
	return env
}

type lifecycleRun struct {
	config     *schema.AtmosConfiguration
	plan       *lifecycle.Plan
	globalArgs []string
	options    *lifecycleOptions
}

func runLifecyclePlan(cmd *cobra.Command, run *lifecycleRun) error {
	plan, globalArgs, opts := run.plan, run.globalArgs, run.options
	for _, n := range plan.Notices {
		ui.Infof("%s/%s/%s: %s", n.Kind, n.Stack, n.Component, n.Reason)
	}
	if opts.dryRun {
		return previewLifecycle(plan, opts.verb, globalArgs, cmd)
	}
	if plan.Graph.Size() == 0 {
		ui.Info("No components matched")
		if plan.Deleted > 0 {
			ui.Info(lifecycleTally(nil, len(plan.Notices)-plan.Deleted, plan.Deleted))
		}
		captured, err := newLifecycleCI(run.config, opts, plan, nil, newLifecycleReporter(run.config))
		if err != nil {
			return err
		}
		if err := captured.publish(nil, len(plan.Notices)-plan.Deleted); err != nil {
			ui.Warningf("Failed to publish the CI summary and outputs: %v", err)
		}
		return nil
	}
	if opts.concurrency > 1 && opts.identity == cfg.IdentityFlagSelectValue {
		return fmt.Errorf("%w: concurrent execution requires a non-interactive identity", errUtils.ErrInvalidConfig)
	}
	if err := checkLifecycleExecution(cmd, run); err != nil {
		return err
	}
	return executeLifecycleChildren(cmd, run)
}

// executeLifecycleChildren launches the planned children, then reports the
// per-component results and, in CI mode, the combined summary and outputs.
func executeLifecycleChildren(cmd *cobra.Command, run *lifecycleRun) error {
	config, plan, globalArgs, opts := run.config, run.plan, run.globalArgs, run.options
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	environment := lifecycleEnvironment(cmd, opts.ci)
	captured, err := newLifecycleCI(config, opts, plan, environment, newLifecycleReporter(config))
	if err != nil {
		return err
	}
	defer captured.cleanup()
	streams := ioLayer.GetContext()
	guards := make(map[string]func(*dependency.Node) func())
	for kind, handler := range plan.Handlers {
		if handler.NewResourceGuard != nil {
			guards[kind] = handler.NewResourceGuard(config)
		}
	}
	execution := &lifecycle.ExecutionOptions{
		Executable: executable, GlobalArgs: globalArgs, Environment: environment,
		Streams: process.Streams{Stdin: streams.Input(), Stdout: streams.Data(), Stderr: streams.UI()}, Runner: process.NewDefaultRunner(),
		Concurrency: opts.concurrency, KeepGoing: opts.failureMode == "keep-going", Grouped: opts.logOrder == "grouped",
		ResourceGuards: guards,
	}
	if captured != nil {
		execution.NodeEnvironment = captured.nodeEnvironment
	}
	if execution.Grouped && execution.Concurrency > 1 {
		ui.Infof("Starting stack %s: %d components, %d workers", opts.verb, plan.Graph.Size(), opts.concurrency)
		execution.OnStart = func(node *dependency.Node) {
			ui.Infof("Running %s: %s", opts.verb, lifecycle.Label(node))
		}
		execution.OnComplete = func(node *dependency.Node, err error) {
			if err != nil {
				ui.Errorf("Failed %s: %s", opts.verb, lifecycle.Label(node))
				return
			}
			ui.Successf("Completed %s: %s", opts.verb, lifecycle.Label(node))
		}
	}
	result := lifecycle.Execute(cmd.Context(), plan, execution)
	reportLifecycleResults(opts.verb, result.Results, len(plan.Notices)-plan.Deleted, plan.Deleted)
	if err := captured.publish(result.Results, len(plan.Notices)-plan.Deleted); err != nil {
		ui.Warningf("Failed to publish the CI summary and outputs: %v", err)
	}
	return result.Err
}
