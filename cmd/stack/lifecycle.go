package stack

import (
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
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/stack/lifecycle"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
)

const flagSkipHooks = "skip-hooks"

type lifecycleOptions struct {
	verb, stack, identity, failureMode, logOrder     string
	all, affected, dryRun, allowDestroy, autoApprove bool
	concurrency, dependencies, dependents            int
}

func init() {
	stackCmd.AddCommand(newLifecycleCommand("deploy"), newLifecycleCommand("destroy"))
}

func newLifecycleCommand(verb string) *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(), flags.WithDryRunFlag(), flags.WithIdentityFlag(),
		flags.WithBoolFlag("all", "", false, "Select all components defining this operation"),
		flags.WithBoolFlag("affected", "", false, "Select components affected by the Git comparison"),
		flags.WithStringFlag("base", "", "", "Base Git ref or SHA (auto-detected in supported CI environments)"),
		flags.WithStringFlag("ref", "", "", "Git reference to compare against"),
		flags.WithStringFlag("sha", "", "", "Git SHA to compare against"),
		flags.WithStringFlag("repo-path", "", "", "Existing repository to use as the comparison baseline"),
		flags.WithStringFlag("ssh-key", "", "", "SSH key for cloning the comparison baseline"),
		flags.WithStringFlag("ssh-key-password", "", "", "SSH key password for cloning the comparison baseline"),
		flags.WithBoolFlag("clone-target-ref", "", false, "Clone the comparison baseline"),
		flags.WithStringFlag(flags.FlagIncludeDependencies, "", "", "Include prerequisites; optionally limit depth with =N"),
		flags.WithNoOptDefVal(flags.FlagIncludeDependencies, flags.ClosureDepthUnlimited),
		flags.WithStringFlag(flags.FlagIncludeDependents, "", "", "Include consumers; optionally limit depth with =N"),
		flags.WithNoOptDefVal(flags.FlagIncludeDependents, flags.ClosureDepthUnlimited),
		flags.WithIntFlag("max-concurrency", "", 1, "Maximum concurrent component operations (one worker permits interactive prompts)"),
		flags.WithStringFlag("failure-mode", "", "fail-fast", "Failure handling: fail-fast or keep-going"),
		flags.WithStringFlag("log-order", "", "stream", "Component output order: stream or grouped"),
		flags.WithStringFlag(flagSkipHooks, "", "", "Skip all hooks or a comma-separated list of hook names"),
		flags.WithNoOptDefVal(flagSkipHooks, "*"),
		flags.WithEnvVars(flagSkipHooks, "ATMOS_SKIP_HOOKS"),
		flags.WithBoolFlag("auto-approve", "", false, "Pass the registered handler's automatic approval arguments where supported"),
	)
	if verb == "deploy" {
		parser = flags.NewStandardParser(flags.WithFlagRegistry(parser.Registry()), flags.WithBoolFlag("allow-destroy", "", false, "Request deleted-component teardown (blocked until isolated recovery is available)"))
	}
	cmd := &cobra.Command{Use: verb, Short: strings.ToUpper(verb[:1]) + verb[1:] + " stack components in dependency order", Args: cobra.NoArgs}
	parser.RegisterFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		v := viper.GetViper()
		if err := parser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		opts, err := parseLifecycleOptions(verb, v)
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

func parseLifecycleOptions(verb string, v *viper.Viper) (lifecycleOptions, error) {
	o := lifecycleOptions{verb: verb, stack: v.GetString("stack"), identity: v.GetString("identity"), all: v.GetBool("all"), affected: v.GetBool("affected"), dryRun: v.GetBool("dry-run"), allowDestroy: verb == "deploy" && v.GetBool("allow-destroy"), autoApprove: v.GetBool("auto-approve"), concurrency: v.GetInt("max-concurrency"), failureMode: v.GetString("failure-mode"), logOrder: v.GetString("log-order")}
	if o.all == o.affected {
		return o, fmt.Errorf("%w: specify exactly one of --all or --affected", errUtils.ErrInvalidFlag)
	}
	if o.allowDestroy && !o.affected {
		return o, fmt.Errorf("%w: --allow-destroy requires --affected", errUtils.ErrInvalidFlag)
	}
	if o.concurrency < 1 || !slices.Contains([]string{"fail-fast", "keep-going"}, o.failureMode) || !slices.Contains([]string{"stream", "grouped"}, o.logOrder) {
		return o, fmt.Errorf("%w: invalid concurrency, failure mode, or log order", errUtils.ErrInvalidFlag)
	}
	var err error
	o.dependencies, err = flags.ParseClosureDepth(flags.FlagIncludeDependencies, v.GetString(flags.FlagIncludeDependencies))
	if err != nil {
		return o, err
	}
	o.dependents, err = flags.ParseClosureDepth(flags.FlagIncludeDependents, v.GetString(flags.FlagIncludeDependents))
	return o, err
}

func runLifecycle(cmd *cobra.Command, opts *lifecycleOptions) error {
	info, err := e.ProcessCommandLineArgs("", cmd, nil, nil)
	if err != nil {
		return err
	}
	config, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}
	// Unlike single-component commands, an empty inventory is a successful
	// selection, including when every stack was removed in the comparison.
	config.StackConfigFilesAbsolutePaths, config.StackConfigFilesRelativePaths, _, err = cfg.FindAllStackConfigsInPathsForStack(
		config, "", config.IncludeStackAbsolutePaths, config.ExcludeStackAbsolutePaths,
	)
	if err != nil && !errors.Is(err, errUtils.ErrNoStackManifestsFound) {
		return err
	}
	manager := authdeferred.NewManager(authdeferred.AuthOptions{Config: &config, Disabled: opts.identity == "false" || opts.dryRun})
	// Only graph selectors are needed now. Vars, hooks, and runtime output
	// references remain unevaluated until the selected child starts.
	stacks, err := e.ExecuteDescribeStacksWithEvalSections(&config, "", nil, nil, nil, false, true, false, true, nil, manager, nil, nil, e.DescribeStacksErrorOptions{}, []string{"dependencies", "metadata", "settings"})
	if err != nil {
		return err
	}
	selection := lifecycle.Selection{Stack: opts.stack, Dependencies: opts.dependencies, Dependents: opts.dependents, Reverse: opts.verb == "destroy"}
	var affected []schema.Affected
	if opts.affected {
		affected, err = compareLifecycleAffected(cmd, &config, opts)
		if err != nil {
			return err
		}
		selection.Affected = append([]schema.Affected{}, affected...)
	}
	commands := discoverLifecycleCommands(cmd.Root(), &config, opts.verb)
	left, _ := tags.TemplateDelims(config.Templates.Settings.Delimiters)
	plan, err := lifecycle.BuildPlan(stacks, commands, selection, left)
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

func compareLifecycleAffected(cmd *cobra.Command, config *schema.AtmosConfiguration, opts *lifecycleOptions) ([]schema.Affected, error) {
	args := &e.DescribeAffectedCmdArgs{CLIConfig: config, AuthDisabled: opts.identity == "false"}
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

func lifecycleGlobalArgs(cmd *cobra.Command, config *schema.AtmosConfiguration, opts *lifecycleOptions) []string {
	args := []string{"--base-path=" + config.BasePathAbsolute}
	if opts.identity != "" {
		args = append(args, "--identity="+opts.identity)
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

func lifecycleEnvironment(cmd *cobra.Command) []string {
	env := os.Environ()
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
	config, plan, globalArgs, opts := run.config, run.plan, run.globalArgs, run.options
	for _, n := range plan.Notices {
		ui.Infof("%s/%s/%s: %s", n.Kind, n.Stack, n.Component, n.Reason)
	}
	if opts.dryRun {
		return previewLifecycle(plan, opts.verb, globalArgs, cmd)
	}
	if plan.Graph.Size() == 0 {
		ui.Info("No components matched")
		return nil
	}
	if opts.concurrency > 1 && opts.identity == cfg.IdentityFlagSelectValue {
		return fmt.Errorf("%w: concurrent execution requires a non-interactive identity", errUtils.ErrInvalidConfig)
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	streams := ioLayer.GetContext()
	guards := make(map[string]func(*dependency.Node) func())
	for kind, handler := range plan.Handlers {
		if handler.NewResourceGuard != nil {
			guards[kind] = handler.NewResourceGuard(config)
		}
	}
	result := lifecycle.Execute(cmd.Context(), plan, &lifecycle.ExecutionOptions{
		Executable: executable, GlobalArgs: globalArgs, Environment: lifecycleEnvironment(cmd),
		Streams: process.Streams{Stdin: streams.Input(), Stdout: streams.Data(), Stderr: streams.UI()}, Runner: process.NewDefaultRunner(),
		Concurrency: opts.concurrency, KeepGoing: opts.failureMode == "keep-going", Grouped: opts.logOrder == "grouped",
		ResourceGuards: guards,
	})
	for i := range result.Results {
		r := &result.Results[i]
		ui.Infof("%s %s: %s", opts.verb, lifecycle.Label(&r.Node), r.Status)
	}
	return result.Err
}
