package terraform

import (
	bytes "bytes"
	errors "errors"
	fmt "fmt"
	io "io"
	os "os"
	osexec "os/exec"
	slices "slices"
	time "time"

	errUtils "github.com/cloudposse/atmos/errors"
	component "github.com/cloudposse/atmos/pkg/component"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	metricsprocess "github.com/cloudposse/atmos/pkg/metrics/process"
	perf "github.com/cloudposse/atmos/pkg/perf"
	pro "github.com/cloudposse/atmos/pkg/pro"
	proexec "github.com/cloudposse/atmos/pkg/proexec"
	retry "github.com/cloudposse/atmos/pkg/retry"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

// checkTTYRequirement returns an error when `terraform apply` is invoked without
// -auto-approve in a non-interactive environment (stdin is nil).
func checkTTYRequirement(info *schema.ConfigAndStacksInfo) error {
	if os.Stdin != nil {
		return nil
	}
	if info.SubCommand == subcommandApply && !slices.Contains(info.AdditionalArgsAndFlags, autoApproveFlag) {
		return fmt.Errorf(
			"%w: 'terraform apply' requires a user interaction, but no TTY is attached. "+
				"Use 'terraform apply -auto-approve' or 'terraform deploy' instead",
			errUtils.ErrNoTty,
		)
	}
	return nil
}

// addRegionEnvVarForImport appends AWS_REGION to the component env list when the
// subcommand is `import` and the component has a `region` variable configured.
func addRegionEnvVarForImport(info *schema.ConfigAndStacksInfo) {
	if info.SubCommand != "import" {
		return
	}
	if region, ok := info.ComponentVarsSection["region"].(string); ok {
		info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("AWS_REGION=%s", region))
	}
}

// resolveExitCode extracts the integer exit code from an error returned by
// ExecuteShellCommand.  Returns 0 when err is nil, 1 for generic (non-typed) errors.
func resolveExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ec errUtils.ExitCodeError
	if errors.As(err, &ec) {
		return ec.Code
	}
	var osErr *osexec.ExitError
	if errors.As(err, &osErr) {
		return osErr.ExitCode()
	}
	return 1
}

// executeMainTerraformCommand runs the final terraform sub-command.
// It handles exit-code extraction, plan-status upload (for --upload-status), and
// appropriate error propagation.  A no-op when info.SubCommand is "workspace" with
// no sub-subcommand (workspace listing was already handled by runWorkspaceSetup).
func (e *Executor) executeMainTerraformCommand( //nolint:revive // argument-limit: opts variadic is not a true argument.
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	allArgsAndFlags []string,
	componentPath string,
	uploadStatusFlag bool,
	opts ...component.ExecutionOption,
) error {
	// Bare `workspace` (no sub-subcommand) was fully handled by runWorkspaceSetup.
	if info.SubCommand == subcommandWorkspace && info.SubCommand2 == "" {
		return nil
	}

	err := e.runMainTerraformCommand(atmosConfig, info, allArgsAndFlags, componentPath, opts...)
	return e.finishMainTerraformCommand(atmosConfig, info, err, uploadStatusFlag)
}

func (e *Executor) runMainTerraformCommand(
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, allArgsAndFlags []string,
	componentPath string, opts ...component.ExecutionOption,
) error {
	// An explicit `atmos terraform init` about to run below (see the matching recordAutoInit
	// call after ExecuteShellCommandWithRetry) must invalidate any existing marker first: see
	// executeTerraformInitCommand's matching call for why a failed init must not leave a
	// stale-but-still-matching marker behind for a later, unrelated invocation to trust.
	executionInfo, prepErr := prepareExplicitProviderInit(atmosConfig, info, componentPath)
	if prepErr != nil {
		return prepErr
	}

	// capturedMetrics receives this invocation's subprocess-tree resource
	// usage (terraform/tofu plus its own children, e.g. provider plugins) via
	// WithMetricsCallback, for combining with atmos's own self-usage below.
	var capturedMetrics *metricsprocess.ProcessMetrics
	shellStartedAt := time.Now()
	allOpts := make([]component.ExecutionOption, 0, len(opts)+1)
	allOpts = append(allOpts, opts...)
	allOpts = append(allOpts, component.WithMetricsCallback(func(m *metricsprocess.ProcessMetrics) {
		capturedMetrics = m
	}))

	err := e.runMainShellCommand(atmosConfig, executionInfo, allArgsAndFlags, componentPath, allOpts)

	// An explicit `atmos terraform init` reaches this main-command path rather
	// than executeTerraformInitCommand. Keep its lifecycle equivalent to an
	// implicit init so post-init provisioners can complete and persist provider
	// locks for workdir and vendored components, and so the smart-init marker
	// gets recorded (recordAutoInit) exactly as it would for an implicit init.
	if err == nil && info.SubCommand == subcommandInit && !info.DryRun {
		e.dispatchAfterInitFn(atmosConfig, info, componentPath, opts...)
		recordAutoInit(newAutoInitInputs(atmosConfig, info, componentPath, ConstructTerraformComponentVarfileName(info)), allArgsAndFlags)
	}
	exitCode := resolveExitCode(err)

	// FR-006e: record the terraform/tofu subprocess's own exit code before any
	// CI-mode remapping or local neutralization below, so TerraformExecData.exit_code
	// (via captureExecMetadataSync) reports the real subprocess outcome even when
	// Atmos's own returned status is remapped/neutralized further down.
	info.ExecMetadataRawExitCode = exitCode
	recordMainTerraformMetrics(atmosConfig, info, capturedMetrics, shellStartedAt)

	return err
}

func recordMainTerraformMetrics(
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo,
	capturedMetrics *metricsprocess.ProcessMetrics, shellStartedAt time.Time,
) {
	// Combine this invocation's subprocess-tree usage (terraform/tofu plus its
	// own children) with atmos's own self-usage so both the exec-metadata
	// upload and the local display reflect the whole command, not just one
	// side of it. Local display is scoped to the sync-upload allowlist
	// (terraform plan/apply/deploy) so it stays in lockstep with what Atmos
	// Pro actually receives — commands with no subprocess (e.g. `describe
	// affected`) keep reporting only atmos's own self-usage, unchanged.
	if capturedMetrics != nil {
		combined := metricsprocess.Combine(metricsprocess.SelfUsageSoFar(), *capturedMetrics)
		combined.WallTime = time.Since(shellStartedAt)
		info.ExecMetadataRawMetrics = &combined

		if proexec.IsSyncCommand("atmos terraform " + info.SubCommand) {
			// Identify the component/stack in the label itself — in a multi-component
			// run (e.g. --all/--affected), this line prints once per component, so a
			// bare "Completed" would be ambiguous once scrolled away from its own
			// plan/apply output. Plain "component (stack)" prose, not CLI flag syntax
			// (e.g. not "-s stack") — this is a status line, not a command to copy/paste.
			// Backticks render as inline code in the toast (ui.Info renders markdown;
			// see pkg/ui/formatter.go's toastMarkdown), matching this repo's convention
			// for identifiers in status messages (e.g. terraform_generate_varfile.go's
			// "Generated varfile `%s`").
			label := fmt.Sprintf("Completed `%s` (`%s`)", info.ComponentFromArg, info.Stack)
			metricsprocess.DisplaySummary(label, combined, atmosConfig)
		}
	}
}

func (e *Executor) finishMainTerraformCommand(
	atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, err error, uploadStatusFlag bool,
) error {
	exitCode := resolveExitCode(err)
	// Upload status only when explicitly requested via --upload-status flag.
	// Upload failures are logged but never cause the terraform command to fail —
	// the exit code should reflect the plan/apply result, not telemetry.
	if uploadStatusFlag && e.services.ShouldUploadStatus(info) {
		if uploadErr := e.uploadCommandStatus(atmosConfig, info, exitCode); uploadErr != nil {
			log.Warn("Failed to upload command status to Atmos Pro. The terraform command result is unaffected.", "error", uploadErr)
		}
	}

	// Apply CI exit code mapping: remap terraform exit codes for CI runners.
	// This is independent of upload — it only affects what the caller sees.
	if mappedCode := e.services.MapCIExitCode(atmosConfig, exitCode); mappedCode == 0 {
		return nil
	}

	// FR-006e/Decision 36: -detailed-exitcode was added to this invocation solely
	// because exec-metadata capture required it (info.ExecMetadataDetailedExitCodeAdded),
	// not because the user explicitly passed --upload-status. The global CI-mode remap
	// above didn't neutralize this exit code (atmosConfig.CI.Enabled is false, or its
	// mapping doesn't cover it), so without this local, call-site-scoped neutralization,
	// Atmos's own process exit code for `plan` would silently change from 0 to 2 in CI
	// environments that satisfy FR-001 but haven't separately set ci.enabled. This does
	// NOT touch atmosConfig.CI.Enabled or mapCIExitCode's own gate — those also govern
	// annotations/SARIF/summaries/hooks CI-mode, well outside this fix's scope.
	if info.ExecMetadataDetailedExitCodeAdded && exitCode == detailedExitCodeChangesDetected {
		return nil
	}

	return err
}

// uploadCommandStatus uploads the command status to Atmos Pro.
func (e *Executor) uploadCommandStatus(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	exitCode int,
) error {
	client, cerr := pro.NewAtmosProAPIClientFromEnv(atmosConfig)
	if cerr != nil {
		return cerr
	}
	gitRepo := &git.DefaultGitRepo{}
	return e.services.UploadStatus(info, exitCode, client, gitRepo)
}

// cleanupTerraformFiles removes ephemeral plan and varfiles that Atmos generates.
// Failures are logged at Trace level and not propagated, since cleanup errors should
// not mask the result of the main command.
func cleanupTerraformFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.DryRun {
		return
	}
	finishConfigurationPlan(atmosConfig, info)
	if info.SubCommand != "plan" && info.SubCommand != "show" && info.PlanFile == "" && !ownsConfigurationPlan(info) {
		planFilePath := ConstructTerraformComponentPlanfilePath(atmosConfig, info)
		removeTerraformFile(planFilePath, "Failed to remove plan file during cleanup")
	}

	if info.SubCommand == subcommandApply {
		varFilePath := ConstructTerraformComponentVarfilePath(atmosConfig, info)
		removeTerraformFile(varFilePath, "Failed to remove var file during cleanup")
	}

	if info.SubCommand == "test" {
		testVarFilePath := ConstructTerraformComponentTestVarfilePath(atmosConfig, info)
		removeTerraformFile(testVarFilePath, "Failed to remove Terraform test var file during cleanup")
	}
}

func removeTerraformFile(path, message string) {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Trace(message, "error", err, "file", path)
	}
}

// invokeShellCommandFunc is the signature used by ExecuteShellCommandWithRetry to delegate
// the actual subprocess invocation back to its caller.  The caller provides a closure that
// already binds the command, args, dir, env, etc., and only needs the variadic options
// (used by the retry wrapper to inject stdout/stderr capture writers).
type invokeShellCommandFunc func(opts ...component.ExecutionOption) error

// resetExecMetadataBufs resets the exec-metadata stdout/stderr tee buffers (if
// present) between retry attempts, so the exec-metadata parser only ever sees
// the latest attempt's output rather than every attempt's output concatenated
// together.
func resetExecMetadataBufs(stdoutW, stderrW io.Writer) {
	if resettable, ok := stdoutW.(interface{ Reset() }); ok {
		resettable.Reset()
	}
	if resettable, ok := stderrW.(interface{ Reset() }); ok {
		resettable.Reset()
	}
}

// RetryExecParams bundles the per-invocation shell command inputs shared by the
// helmfile/packer retry-wrapping helpers (executeHelmfileCommandWithRetry,
// executePackerCommandWithRetry), keeping their parameter lists within the argument-limit.
type RetryExecParams struct {
	AllArgsAndFlags []string
	ComponentPath   string
	EnvVars         []string
}

// ExecuteShellCommandWithRetry runs invoke exactly once when info.ComponentRetrySection is
// nil or has no Conditions configured (zero behavioural change for non-retry components).
// It is component-type agnostic: terraform, helmfile, packer, and ansible all shell out
// through ExecuteShellCommand and share this same wrapper.
//
// When retry is configured, stdout and stderr are tee'd into a buffer and the buffer is
// matched against the compiled retry conditions after each attempt.  Errors whose captured
// output matches at least one condition trigger another attempt with the configured backoff;
// other errors fail fast.  This is intentionally pattern-driven so that real failures (e.g.,
// `terraform plan` exit-code-2) are never retried unless the user opts in by listing a
// matching condition.
//
// If baseOpts already requests its own stdout/stderr capture (e.g. helmfile capturing output
// for a NodeHooks.After callback), that writer keeps receiving the full output — the retry
// buffer is teed in alongside it via io.MultiWriter, not substituted for it.
//
// `phase` is included in retry log lines so users can see which subprocess invocation is
// being retried (e.g. "init", "workspace-select", "apply", "sync", "build").
func ExecuteShellCommandWithRetry(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	phase string,
	invoke invokeShellCommandFunc,
	baseOpts ...component.ExecutionOption,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteShellCommandWithRetry")()

	cfg := info.ComponentRetrySection
	if cfg == nil || len(cfg.Conditions) == 0 {
		return invoke(baseOpts...)
	}

	patterns, err := retry.CompileConditions(cfg.Conditions)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidConfig, err)
	}

	var buf bytes.Buffer
	stdoutWriter, stderrWriter := composeRetryCaptureWriters(baseOpts, &buf)
	captureOpts := append([]component.ExecutionOption{}, baseOpts...)
	captureOpts = append(captureOpts, component.WithStdoutCapture(stdoutWriter), component.WithStderrCapture(stderrWriter))

	execMetadataStdout, execMetadataStderr := component.MetadataOutputCaptureFromOptions(baseOpts...)

	attempt := 0
	return retry.WithPredicate(
		component.ContextFromOptions(baseOpts...),
		cfg,
		func() error {
			attempt++
			buf.Reset()
			resetExecMetadataBufs(execMetadataStdout, execMetadataStderr)
			if attempt > 1 {
				log.Warn(
					"Retrying subprocess after recoverable error",
					"phase", phase,
					logFieldComponent, info.ComponentFromArg,
					"stack", info.StackFromArg,
					"attempt", attempt,
				)
			}
			return invoke(captureOpts...)
		},
		func(invokeErr error) bool {
			if invokeErr == nil {
				return false
			}
			return retry.MatchesAny(patterns, buf.String())
		},
	)
}

// composeRetryCaptureWriters returns the stdout/stderr writers ExecuteShellCommandWithRetry
// should inject: buf alone when baseOpts requests no capture of its own, or an
// io.MultiWriter teeing into both buf and the caller's writer when it does — so a caller's
// own capture (e.g. helmfile's NodeHooks.After output) keeps receiving output instead of
// being silently replaced by the retry buffer.
//
// Buf is wrapped in a syncWriter (see terraform_streaming_ui.go) because the returned
// stdout/stderr writers can end up being invoked concurrently: when the caller streams through
// the TUI (executeStreamingOrShell -> pkg/terraform/ui's newInitCommand), stdout and stderr are
// set to two distinct io.Writer values even though both ultimately tee into this same buf, and
// os/exec.Cmd copies each stream in its own goroutine whenever Stdout and Stderr aren't the
// exact same Writer value — so without synchronization, concurrent Write calls into buf would
// race (bytes.Buffer is not safe for concurrent use).
func composeRetryCaptureWriters(baseOpts []component.ExecutionOption, buf *bytes.Buffer) (stdout, stderr io.Writer) {
	var probe component.ExecutionOptions
	for _, opt := range baseOpts {
		opt(&probe)
	}

	safeBuf := &syncWriter{w: buf}

	stdout = safeBuf
	if probe.StdoutCapture != nil {
		stdout = io.MultiWriter(probe.StdoutCapture, safeBuf)
	}
	stderr = safeBuf
	if probe.StderrCapture != nil {
		stderr = io.MultiWriter(probe.StderrCapture, safeBuf)
	}
	return stdout, stderr
}
