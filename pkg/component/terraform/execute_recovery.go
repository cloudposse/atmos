package terraform

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/component/execution"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// recoveryReporter prints what error handling decided for one component.
type recoveryReporter struct {
	info *schema.ConfigAndStacksInfo
}

func (r *recoveryReporter) label() string {
	return fmt.Sprintf("`%s` in `%s`", r.info.ComponentFromArg, r.info.Stack)
}

// Retrying implements execution.Reporter.
func (r *recoveryReporter) Retrying(policy string, attempt, maxAttempts int, sleep time.Duration) {
	defer perf.Track(nil, "terraform.recoveryReporter.Retrying")()

	ui.Warningf("Retryable error matched policy `%s` for %s (attempt %d of %d). Retrying in %s.", policy, r.label(), attempt, maxAttempts, sleep)
}

// Ignored implements execution.Reporter.
func (r *recoveryReporter) Ignored(policy, message string) {
	defer perf.Track(nil, "terraform.recoveryReporter.Ignored")()

	text := fmt.Sprintf("Ignoring an error in %s because it matches policy `%s`", r.label(), policy)
	if message != "" {
		text += ": " + message
	}
	ui.Warning(text)
}

// SignalsWritten implements execution.Reporter.
func (r *recoveryReporter) SignalsWritten(path string) {
	defer perf.Track(nil, "terraform.recoveryReporter.SignalsWritten")()

	ui.Warningf("Wrote error signals to `%s`.", path)
}

// failureText collects the standard error of the steps, hooks and command, that failed during one
// attempt. Error handling matches its policies against this text only.
type failureText struct {
	mu   sync.Mutex
	text strings.Builder
}

// add records the output of a failed step. A nil receiver records nothing.
func (f *failureText) add(text string) {
	if f == nil || strings.TrimSpace(text) == "" {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.text.WriteString(text)
	f.text.WriteString("\n")
}

// recorded returns the collected output.
func (f *failureText) recorded() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.text.String()
}

// recoverPhase runs an attempt under the component's errors policy. Error text for matching is
// the standard error of the steps that failed, as it is for Terragrunt, so output on stdout, or a
// warning from a step that succeeded, never triggers a policy. The attempts also report whether the
// command itself finished, so an ignored failure of a later hook leaves its result in place.
func (e *Executor) recoverPhase(info *schema.ConfigAndStacksInfo, phase *executionPhase, policy *schema.ComponentErrors, opts []component.ExecutionOption, attempts *phaseAttempts) error {
	defer perf.Track(nil, "terraform.Executor.recoverPhase")()

	var changes error
	recoverer := execution.NewRecoverer(
		policy,
		execution.WithReporter(&recoveryReporter{info: info}),
		execution.WithSignalsFile(filepath.Join(phase.componentPath, execution.SignalsFileName)),
	)
	ignored, err := recoverer.Run(component.ContextFromOptions(opts...), func(context.Context) (string, error) {
		var failed failureText
		var runErr error
		changes, runErr = attempts.run(opts, &failed)
		return failed.recorded(), runErr
	})
	switch {
	case err != nil:
		return err
	case ignored && !phase.main:
		// A preparatory step such as the implicit init failed with an ignorable error. Terragrunt ends
		// the whole unit run there, so the command that follows must not run against a missing init.
		return errUtils.ErrComponentRunIgnored
	case ignored && attempts.commandCompleted():
		// A hook failed after the command finished. The command's result stands, including a plan
		// with changes, so the plan is still recorded.
		return changes
	case ignored:
		markMainCommandIgnored(component.ContextFromOptions(opts...))
		return nil
	}
	return changes
}

// withStderrCapture adds a standard error capture beside any capture the caller already has.
func withStderrCapture(opts []component.ExecutionOption, buffer *bytes.Buffer) []component.ExecutionOption {
	var probe component.ExecutionOptions
	for _, opt := range opts {
		opt(&probe)
	}
	var writer io.Writer = &syncWriter{w: buffer}
	if probe.StderrCapture != nil {
		writer = io.MultiWriter(probe.StderrCapture, writer)
	}
	return append(slices.Clone(opts), component.WithStderrCapture(writer))
}

// commandStderrKey carries the standard error capture of the running attempt, which error policies match.
type commandStderrKey struct{}

// resetCommandStderr discards what the running attempt captured. The smart-init retry runs inside the
// same attempt, so without it the first diagnostic, which the init already handled, would be matched
// against the policies together with the retry's own output. What was printed stays printed.
func resetCommandStderr(ctx context.Context) {
	if captured, ok := ctx.Value(commandStderrKey{}).(*bytes.Buffer); ok {
		captured.Reset()
	}
}

// phaseAttempts runs one phase's hooks and command, once per recovery attempt.
//
// The retry scope is the work that did not complete. A command that finished, such as a saved-plan
// apply that changed state, is never run again because a later hook failed: the retry runs only
// the after hooks that did not finish, and the before hooks that preceded it are not repeated.
type phaseAttempts struct {
	config  *schema.ComponentExecution
	info    *schema.ConfigAndStacksInfo
	phase   *executionPhase
	args    []string
	env     []string
	runner  *hookRunner
	capture *phaseCapture
	action  phaseAction
	// completed reports that the command finished in an earlier attempt, and changes is the exit
	// status 2 result it reported, if any.
	completed bool
	changes   error
	// started reports that the command ran in an earlier attempt, so its recorded output is stale.
	started bool
	// afterDone holds the after hooks that succeeded once the command had completed. A retry does not
	// run them again, so a notification is never published twice.
	afterDone map[*schema.ComponentHook]bool
}

// commandCompleted reports that the command finished in this or an earlier attempt.
func (a *phaseAttempts) commandCompleted() bool { return a.completed }

// run makes one attempt. With -detailed-exitcode, exit status 2 reports a plan with changes: hooks
// must not see that as a failure, but the status still has to reach the caller once the hooks are done.
func (a *phaseAttempts) run(opts []component.ExecutionOption, failed *failureText) (changesPresent, runErr error) {
	config := a.config
	if a.completed {
		stripped := *config
		stripped.BeforeHooks = nil
		config = &stripped
	}
	var commandStderr bytes.Buffer
	if failed != nil {
		opts = withStderrCapture(opts, &commandStderr)
		opts = append(opts, component.WithProcessContext(context.WithValue(component.ContextFromOptions(opts...), commandStderrKey{}, &commandStderr)))
	}
	a.runner.failed = failed
	runner := &resumingRunner{inner: a.runner, done: a.afterDone}
	runErr = execution.Run(component.ContextFromOptions(opts...), config, a.phase.command, runner, func() (execution.Output, error) {
		return a.runCommand(opts, failed, &commandStderr)
	})
	// After hooks that ran beside a failed command belong to that attempt: the next one runs them again.
	if a.completed && len(runner.succeeded) > 0 {
		if a.afterDone == nil {
			a.afterDone = map[*schema.ComponentHook]bool{}
		}
		maps.Copy(a.afterDone, runner.succeeded)
	}
	return a.changes, runErr
}

// runCommand runs the command unless an earlier attempt completed it.
func (a *phaseAttempts) runCommand(opts []component.ExecutionOption, failed *failureText, commandStderr *bytes.Buffer) (execution.Output, error) {
	if a.completed {
		return nil, nil
	}
	if a.started {
		// Only a command that runs again replaces the output the metadata parser reads.
		resetExecMetadataBufs(component.MetadataOutputCaptureFromOptions(opts...))
	}
	a.started = true
	output, actionErr := runPhaseAction(a.info, a.args, opts, a.capture, a.action)
	switch {
	case isDetailedExitCodeChanges(a.phase.command, a.args, a.env, actionErr, opts...):
		// Mark the completed plan here, so the recovery path reports it the same way.
		a.changes = &successfulPlanChanges{Err: actionErr}
		a.completed = true
		return output, nil
	case actionErr == nil:
		a.completed = true
	default:
		failed.add(commandStderr.String())
	}
	return output, actionErr
}

// resumingRunner skips the after hooks that already succeeded and records those that succeed now.
type resumingRunner struct {
	inner     execution.Runner
	done      map[*schema.ComponentHook]bool
	succeeded map[*schema.ComponentHook]bool
}

// RunHook implements execution.Runner.
func (r *resumingRunner) RunHook(ctx context.Context, kind execution.HookKind, hook *schema.ComponentHook) (execution.Output, error) {
	defer perf.Track(nil, "terraform.resumingRunner.RunHook")()

	if kind != execution.KindAfter {
		return r.inner.RunHook(ctx, kind, hook)
	}
	if r.done[hook] {
		return nil, nil
	}
	output, err := r.inner.RunHook(ctx, kind, hook)
	if err == nil {
		if r.succeeded == nil {
			r.succeeded = map[*schema.ComponentHook]bool{}
		}
		r.succeeded[hook] = true
	}
	return output, err
}

// withoutIgnoredPolicyFailure drops the saved-plan guard's failure once the errors policy has ignored
// the main command. An ignored plan wrote no artifact, so the guard fails reading it, and that must
// not undo the ignore decision.
func withoutIgnoredPolicyFailure(err error, ignored bool) error {
	if ignored && errors.Is(err, errUtils.ErrPolicyPlan) {
		return nil
	}
	return err
}

// warnOverlappingRetry tells the user that two retry mechanisms apply to the same component.
// `settings.retry` wraps each Terraform subprocess and runs first; an errors block retries what is
// left after it is exhausted, so one failure can be attempted more often than either alone.
func warnOverlappingRetry(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) {
	if prepared == nil || prepared.Execution == nil || prepared.Execution.Errors == nil || len(prepared.Execution.Errors.Retry) == 0 {
		return
	}
	if info.ComponentRetrySection == nil || len(info.ComponentRetrySection.Conditions) == 0 {
		return
	}
	ui.Warningf("Component `%s` in `%s` has both `settings.retry` and a Terragrunt `errors.retry` policy. `settings.retry` retries each Terraform command first; the `errors.retry` policy then retries what is left, so a failure can be attempted more often than either setting alone allows.", info.ComponentFromArg, info.Stack)
}

// excludedFromOperation reports whether the provider says this operation must not run the unit.
func excludedFromOperation(info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) bool {
	if prepared == nil || prepared.Exclusion == nil {
		return false
	}
	command := info.SubCommand
	if command == subcommandDeploy {
		command = subcommandApply
	}
	return slices.Contains(prepared.Exclusion.Actions, execution.CommandName(command, info.AdditionalArgsAndFlags))
}

// skipExcluded announces why a unit does not run.
func skipExcluded(info *schema.ConfigAndStacksInfo) {
	ui.Infof("Skipping `%s` for `%s` in `%s`: its Terragrunt `exclude` block sets `no_run`.", info.SubCommand, info.ComponentFromArg, info.Stack)
}

// endIgnoredRun ends the unit's run successfully after an ignore policy matched a failure that left it
// without a resolved source. It announces the decision as an ignored source fetch does.
func endIgnoredRun(info *schema.ConfigAndStacksInfo, ignored *schema.ComponentIgnoredRun) error {
	(&recoveryReporter{info: info}).Ignored(ignored.Policy, ignored.Message)
	return errUtils.ErrComponentRunIgnored
}

// runReadConfigHooks runs the unit's read-config after hooks once its configuration is read. They
// run on every command that prepares the unit, in the unit's configuration directory, before any
// source is fetched. Hooks written for `read-config` and `terragrunt-read-config` both run. They come
// from the execution the invocation runs, so a saved-plan apply runs the apply-time metadata. A
// module source the provider deferred until these hooks had run is resolved when they are done.
func (e *Executor) runReadConfigHooks(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	defer perf.Track(atmosConfig, "terraform.Executor.runReadConfigHooks")()

	if prepared == nil || info.DryRun {
		return nil
	}
	var hooks []schema.ComponentHook
	if info.ComponentExecution != nil {
		hooks = execution.HooksFor(info.ComponentExecution.AfterHooks, execution.CommandReadConfig, execution.CommandReadConfigAlias)
	}
	if len(hooks) > 0 {
		stage := lifecycleStage{command: execution.CommandReadConfig, config: &schema.ComponentExecution{AfterHooks: hooks}}
		if err := e.runLifecycle(ctx, atmosConfig, info, stage, func(context.Context) error { return nil }); err != nil {
			return err
		}
	}
	return resolveDeferredSource(ctx, atmosConfig, info, prepared)
}

// resolveDeferredSource completes a preparation whose module source the provider left as written
// until the unit's read-config hooks had run, because such a hook can create the credentials that
// resolving the source needs. The provider prepares the unit once more and resolves the source with
// the exclusion guard and the errors policies of a single preparation. Only the source, and whether
// the run still goes ahead, are taken from that second preparation: the unit's configuration was read
// once, before the hooks, as Terragrunt reads it. A dry run and a saved-plan apply never get here
// with a deferred source: a dry run prepares nothing, and a saved plan holds the source its plan resolved.
func resolveDeferredSource(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	if !prepared.SourceDeferred {
		return nil
	}
	runtime, ok := atmosConfig.ComponentConfigurations.(schema.ComponentConfigurationDeferredSource)
	if !ok {
		return fmt.Errorf("%w: the configuration provider of component %q in stack %q deferred its module source but cannot resolve it", errUtils.ErrPluginUnsupportedExtension, info.ComponentFromArg, info.Stack)
	}
	resolved, err := runtime.ResolveDeferredSource(ctx, info)
	if err != nil {
		return err
	}
	if resolved == nil || resolved.SourceDeferred {
		return fmt.Errorf("%w: the module source of component %q in stack %q is still unresolved after its read-config hooks ran", errUtils.ErrPluginProtocol, info.ComponentFromArg, info.Stack)
	}
	prepared.Exclusion, prepared.IgnoredRun = resolved.Exclusion, resolved.IgnoredRun
	if err := endUnrunnableSource(atmosConfig, info, prepared); err != nil {
		return err
	}
	prepared.Source, prepared.SourceAuthorization, prepared.SourceDeferred = resolved.Source, resolved.SourceAuthorization, false
	applyPreparedSource(info, prepared)
	return nil
}

// endUnrunnableSource ends the run of a unit whose deferred source was not resolved: the operation is
// excluded, or an ignore policy matched the failure. It reports both as a single preparation does.
func endUnrunnableSource(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	if excludedFromOperation(info, prepared) {
		skipExcluded(info)
		return errUtils.ErrComponentExcluded
	}
	if prepared.IgnoredRun == nil {
		return nil
	}
	if err := refreshIgnoredSignals(atmosConfig, info, prepared); err != nil {
		return err
	}
	return endIgnoredRun(info, prepared.IgnoredRun)
}

// initFromModuleWrapper runs the unit's init-from-module hooks around a source fetch. It returns
// nil when no hook lists the stage, so units without such hooks pay nothing.
func (e *Executor) initFromModuleWrapper(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) source.ProvisionWrapper {
	config := info.ComponentExecution
	recovered := config != nil && config.Errors != nil
	if !recovered && !hasLifecycleHooks(config, execution.CommandInitFromModule) {
		return nil
	}
	return func(ctx context.Context, target *source.Target, provision func(context.Context) error) error {
		stage := lifecycleStage{
			command: execution.CommandInitFromModule, config: config, directory: target.Dir,
			moduleDir: func() string { return fetchedModuleDir(info, target) },
		}
		fetch := provision
		if recovered {
			fetch = recoveringFetch(info, target, config.Errors, provision)
		}
		return e.runLifecycle(ctx, atmosConfig, info, stage, fetch)
	}
}

// recoveringFetch applies the errors policy to the source fetch, inside the init-from-module
// hooks, as Terragrunt does for its download. An ignored failure ends the unit's run, because
// there is no source to run the command against.
func recoveringFetch(info *schema.ConfigAndStacksInfo, target *source.Target, policy *schema.ComponentErrors, provision func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		// An explicit source subdirectory exists only once the fetch has provisioned the source, so the
		// directory that receives ignore signals is resolved when they are written.
		recoverer := execution.NewRecoverer(policy, execution.WithReporter(&recoveryReporter{info: info}),
			execution.WithSignalsPath(func() string { return fetchedSignalsFile(info, target) }))
		ignored, err := recoverer.Run(ctx, func(ctx context.Context) (string, error) {
			if err := provision(ctx); err != nil {
				return err.Error(), err
			}
			return "", nil
		})
		if err != nil {
			return err
		}
		if ignored {
			return errUtils.ErrComponentRunIgnored
		}
		return nil
	}
}

// fetchedModuleDir returns the module directory of a fetched source, or "" when that directory does
// not exist, which is the case for an ignored fetch that provisioned nothing. An explicit source
// subdirectory exists only once the fetch has provisioned the source.
func fetchedModuleDir(info *schema.ConfigAndStacksInfo, target *source.Target) string {
	if !target.IsWorkdir {
		return target.Dir
	}
	moduleDir, present, err := component.ResolvePendingSubpath(info, target.Dir)
	if err != nil || !present {
		return ""
	}
	return moduleDir
}

// fetchedSignalsFile returns the signals file in the module directory of a fetched source, or "" when
// that directory does not exist.
func fetchedSignalsFile(info *schema.ConfigAndStacksInfo, target *source.Target) string {
	moduleDir := fetchedModuleDir(info, target)
	if moduleDir == "" {
		return ""
	}
	return filepath.Join(moduleDir, execution.SignalsFileName)
}

// hasLifecycleHooks reports whether any before, after, or error hook lists the command.
func hasLifecycleHooks(config *schema.ComponentExecution, command string) bool {
	if config == nil {
		return false
	}
	return execution.ListsCommand(config.BeforeHooks, command) || execution.ListsCommand(config.AfterHooks, command) || execution.ListsCommand(config.ErrorHooks, command)
}

// lifecycleStage is a provider stage and the hooks that wrap it.
type lifecycleStage struct {
	command   string
	config    *schema.ComponentExecution
	directory string
	// moduleDir, when set, reports the directory of the module the stage produced. Hooks that run once
	// the stage has run resolve relative directories there, because it may not exist before then.
	moduleDir func() string
}

// withInitFromModule makes source provisioning run the init-from-module hooks, when the unit has any.
func (e *Executor) withInitFromModule(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) context.Context {
	if wrapper := e.initFromModuleWrapper(atmosConfig, info); wrapper != nil {
		return source.WithProvisionWrapper(ctx, wrapper)
	}
	return ctx
}

// runLifecycle runs action wrapped by the hooks of one lifecycle stage.
func (e *Executor) runLifecycle(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, stage lifecycleStage, action func(context.Context) error) error {
	phase := &executionPhase{command: stage.command, componentPath: stage.directory}
	hookOpts := hookOptions([]component.ExecutionOption{component.WithProcessContext(ctx), component.WithEnvironment(info.SanitizedEnv)})
	// Error hooks match on the output of the hooks that ran before them, so their output is recorded for as
	// long as the lifecycle runs, including the matching.
	var capture *phaseCapture
	if stage.config != nil && len(stage.config.ErrorHooks) > 0 {
		capture = &phaseCapture{}
		defer capture.cleanup()
	}
	runner := &hookRunner{executor: e, atmosConfig: atmosConfig, info: info, phase: phase, opts: hookOpts, capture: capture}
	return execution.Run(ctx, stage.config, stage.command, runner, func() (execution.Output, error) {
		err := action(ctx)
		if stage.moduleDir != nil {
			if dir := stage.moduleDir(); dir != "" {
				phase.componentPath = dir
			}
		}
		if err == nil {
			return nil, nil
		}
		return execution.TextOutput(strings.TrimSpace(err.Error())), err
	})
}
