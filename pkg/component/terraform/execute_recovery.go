package terraform

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
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

// attemptFunc runs the hooks and the command once. It returns the exit status 2 of
// `plan -detailed-exitcode` separately, because that is a result and not a failure.
type attemptFunc func(opts []component.ExecutionOption, hookStderr io.Writer) (changes, err error)

// recoverPhase runs an attempt under the component's errors policy. Error text for matching is
// the command's standard error, as it is for Terragrunt, so output on stdout never triggers a policy.
func (e *Executor) recoverPhase(info *schema.ConfigAndStacksInfo, phase *executionPhase, policy *schema.ComponentErrors, opts []component.ExecutionOption, attempt attemptFunc) error {
	defer perf.Track(nil, "terraform.Executor.recoverPhase")()

	var changes error
	attempts := 0
	recoverer := execution.NewRecoverer(
		policy,
		execution.WithReporter(&recoveryReporter{info: info}),
		execution.WithSignalsFile(filepath.Join(phase.componentPath, execution.SignalsFileName)),
	)
	stdoutMetadata, stderrMetadata := component.MetadataOutputCaptureFromOptions(opts...)
	ignored, err := recoverer.Run(component.ContextFromOptions(opts...), func(context.Context) (string, error) {
		attempts++
		if attempts > 1 {
			resetExecMetadataBufs(stdoutMetadata, stderrMetadata)
		}
		var stderr bytes.Buffer
		var runErr error
		changes, runErr = attempt(withStderrCapture(opts, &stderr), &stderr)
		return stderr.String(), runErr
	})
	switch {
	case err != nil:
		return err
	case ignored && !phase.main:
		// A preparatory step such as the implicit init failed with an ignorable error. Terragrunt ends
		// the whole unit run there, so the command that follows must not run against a missing init.
		return errUtils.ErrComponentRunIgnored
	case ignored:
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

// runReadConfigHooks runs the unit's read-config after hooks once its configuration is read. They
// run on every command that prepares the unit, in the unit's configuration directory, before any
// source is fetched. Hooks written for `read-config` and `terragrunt-read-config` both run.
func (e *Executor) runReadConfigHooks(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, prepared *schema.PreparedComponentConfiguration) error {
	defer perf.Track(atmosConfig, "terraform.Executor.runReadConfigHooks")()

	if prepared == nil || prepared.Execution == nil || info.DryRun {
		return nil
	}
	hooks := execution.HooksFor(prepared.Execution.AfterHooks, execution.CommandReadConfig, execution.CommandReadConfigAlias)
	if len(hooks) == 0 {
		return nil
	}
	stage := lifecycleStage{command: execution.CommandReadConfig, config: &schema.ComponentExecution{AfterHooks: hooks}}
	return e.runLifecycle(ctx, atmosConfig, info, stage, func(context.Context) error { return nil })
}

// initFromModuleWrapper runs the unit's init-from-module hooks around a source fetch. It returns
// nil when no hook lists the stage, so units without such hooks pay nothing.
func (e *Executor) initFromModuleWrapper(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) source.ProvisionWrapper {
	config := info.ComponentExecution
	recovered := config != nil && config.Errors != nil
	if !recovered && !hasLifecycleHooks(config, execution.CommandInitFromModule) {
		return nil
	}
	stage := lifecycleStage{command: execution.CommandInitFromModule, config: config}
	return func(ctx context.Context, target *source.Target, provision func(context.Context) error) error {
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

// fetchedSignalsFile returns the signals file in the module directory of a fetched source, or "" when
// that directory does not exist, which is the case for an ignored fetch that provisioned nothing.
func fetchedSignalsFile(info *schema.ConfigAndStacksInfo, target *source.Target) string {
	moduleDir := target.Dir
	if target.IsWorkdir {
		var present bool
		var err error
		moduleDir, present, err = component.ResolvePendingSubpath(info, target.Dir)
		if err != nil || !present {
			return ""
		}
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
	command string
	config  *schema.ComponentExecution
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
	phase := &executionPhase{command: stage.command}
	hookOpts := hookOptions([]component.ExecutionOption{component.WithProcessContext(ctx), component.WithEnvironment(info.SanitizedEnv)})
	runner := &hookRunner{executor: e, atmosConfig: atmosConfig, info: info, phase: phase, opts: hookOpts}
	return execution.Run(ctx, stage.config, stage.command, runner, func() (execution.Output, error) {
		err := action(ctx)
		if err == nil {
			return nil, nil
		}
		return execution.TextOutput(strings.TrimSpace(err.Error())), err
	})
}
