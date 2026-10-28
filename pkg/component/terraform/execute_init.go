package terraform

import (
	bytes "bytes"
	context "context"
	errors "errors"
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	slices "slices"
	strings "strings"
	"sync"
	time "time"

	errUtils "github.com/cloudposse/atmos/errors"
	component "github.com/cloudposse/atmos/pkg/component"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/process"
	provisioner "github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	schema "github.com/cloudposse/atmos/pkg/schema"
	autoinit "github.com/cloudposse/atmos/pkg/terraform/autoinit"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// shouldRunTerraformInit returns true when a `terraform init` should be executed as a
// pre-step before the main command.  Init is skipped when: the subcommand is init
// itself (init runs as the main command), deploy with DeployRunInit=false is configured,
// the caller passed the --skip-init flag, or components.terraform.init.mode is "never".
// Note this only gates whether the smart-init decision point (executeTerraformInitCommand)
// runs at all -- the decision of whether that pre-step actually shells out to `terraform
// init`, and with which flags, is made by autoinit.Decide once it does run.
//
// The init.mode: never check below excludes the workspace subcommand: autoinit.RequestFromInfo
// forces Decide's decision (Force: true) whenever info.SubCommand == "workspace", independent of
// init.mode, because workspace select/new needs a freshly reconfigured backend regardless of
// whether the ordinary pre-command init would otherwise run -- this predates init.mode entirely
// (see buildInitArgs's history). Gating that decision point out here for mode: never would
// silently drop that forced reconfigure and break workspace select/new for anyone who set
// init.mode: never intending only to suppress the ordinary unconditional pre-command init.
func shouldRunTerraformInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) bool {
	if info.SubCommand == subcommandInit {
		return false
	}
	if info.SubCommand == subcommandDeploy && !atmosConfig.Components.Terraform.DeployRunInit {
		return false
	}
	if info.SkipInit {
		log.Debug("Skipping over 'terraform init' due to '--skip-init' flag being passed")
		return false
	}
	if info.SubCommand != subcommandWorkspace && atmosConfig.Components.Terraform.EffectiveInitMode() == schema.TerraformInitModeNever {
		log.Debug("Skipping over 'terraform init' due to components.terraform.init.mode: never")
		return false
	}
	return true
}

// buildInitArgs constructs the argument list for `terraform init` from an already-computed
// autoinit.Decision: `init`, then `-reconfigure` when decision.Reconfigure, then `-upgrade`
// when decision.Upgrade, then `-var-file <varFile>` when Init.PassVars is enabled.
//
// This replaces a former workdir-specific carve-out that ignored the legacy
// InitRunReconfigure setting unless the workdir was actually re-provisioned this invocation
// (WorkdirReprovisionedKey) -- adding -reconfigure unconditionally used to make OpenTofu treat
// init as a fresh backend initialization and prompt "Do you want to migrate all workspaces?"
// whenever it saw existing workspace state directories (terraform.tfstate.d/) but no evidence
// the backend itself had moved; decision.Reconfigure now encodes exactly that rule for every
// component (not just workdir ones): auto reconfigure compares the current and last-recorded
// backend fingerprints (see autoinit.Decide), so -reconfigure is added only when the backend
// configuration actually changed, or a caller explicitly forced it (workspace subcommand, a
// re-provisioned workdir, or `init.reconfigure: always`).
//
//nolint:gocritic // autoinit.Decision (88 bytes) is passed by value throughout this call chain and its tests; not worth a pointer for a non-hot-path helper.
func buildInitArgs(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, varFile string, decision autoinit.Decision) []string {
	args := []string{subcommandInit}
	if decision.Reconfigure {
		args = append(args, "-reconfigure")
	}
	if decision.Upgrade {
		args = append(args, "-upgrade")
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		args = append(args, varFileFlag, varFile)
	}
	log.Debug("autoinit: init args", logFieldComponent, info.ComponentFromArg, "args", args)
	return args
}

// newAutoInitInputs builds this invocation's fingerprint inputs -- the thin internal/exec-local
// wrapper around autoinit.InputsFromInfo shared by buildInitSubcommandArgs, recordAutoInit, and
// executeTerraformInitForced.
func newAutoInitInputs(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string) *autoinit.Inputs {
	return autoinit.InputsFromInfo(atmosConfig, info, componentPath, varFile)
}

// decideAutoInit resolves atmos.yaml's init policy plus force into an autoinit.Request and
// returns autoinit.Decide's outcome -- the thin internal/exec-local wrapper around
// autoinit.RequestFromInfo + autoinit.Decide used by buildInitSubcommandArgs for an explicit
// `atmos terraform init`.
func decideAutoInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, in *autoinit.Inputs, force bool) autoinit.Decision {
	return autoinit.Decide(autoinit.RequestFromInfo(atmosConfig, info, in, force))
}

// recordAutoInit best-effort records the init marker after a successful init -- the thin
// internal/exec-local wrapper around autoinit.RecordFromInfo.
func recordAutoInit(in *autoinit.Inputs, initArgs []string) {
	autoinit.RecordFromInfo(in, initArgs)
}

// classifyInitRecovery applies Atmos's init-recovery policy to a failed main command's captured
// output: classify the diagnostic (autoinit.Classify), then decide whether/how to recover
// (autoinit.ShouldRecover) given the component's resolved init policy and opt-out signals. Used
// by recoverFromInitRequired, smart init's plan/apply-time safety net.
func classifyInitRecovery(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, output string) (autoinit.Recovery, error) {
	tf := &atmosConfig.Components.Terraform
	return autoinit.ShouldRecover(
		autoinit.Classify(output),
		tf.EffectiveInitMode(),
		tf.EffectiveInitReconfigure(),
		tf.EffectiveInitUpgrade(),
		autoinit.OptedOut(atmosConfig, info),
	)
}

// executeTerraformInitForced runs `terraform init` unconditionally with exactly the
// -reconfigure/-upgrade flags smart-init's plan/apply-time recovery (recoverFromInitRequired)
// determined were needed, dispatches the after.terraform.init provisioners, and records the
// resulting marker -- exactly as an implicit init would.
//
//nolint:revive // argument-limit: every parameter is load-bearing context from recoverFromInitRequired's forced init/retry.
func (e *Executor) executeTerraformInitForced(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, reconfigure, upgrade bool, opts ...component.ExecutionOption) error {
	defer perf.Track(atmosConfig, "exec.executeTerraformInitForced")()

	args := []string{subcommandInit}
	if reconfigure {
		args = append(args, "-reconfigure")
	}
	if upgrade {
		args = append(args, "-upgrade")
	}
	if atmosConfig.Components.Terraform.Init.PassVars {
		args = append(args, varFileFlag, varFile)
	}

	in := newAutoInitInputs(atmosConfig, info, componentPath, varFile)
	// Invalidate before attempting: see executeTerraformInitCommand's matching call for why a
	// forced init that fails must not leave a stale-but-still-matching marker behind.
	if !info.DryRun {
		autoinit.InvalidateFromInfo(in)
	}
	if _, err := e.runTerraformInitSubprocess(atmosConfig, info, componentPath, args, opts...); err != nil {
		return err
	}

	e.dispatchAfterInit(atmosConfig, info, componentPath, opts...)
	if !info.DryRun {
		recordAutoInit(in, args)
	}
	return nil
}

// prepareInitExecution performs the pre-init housekeeping:
//  1. Deletes the .terraform/environment file so Terraform doesn't prompt for workspace selection
//     (skipped for workdir-enabled components — see note below).
//  2. Executes all provisioners registered for the before.terraform.init hook event.
//  3. Returns the effective component path (which may be overridden by a workdir provisioner).
//
// NOTE on cleanTerraformWorkspace and workdir components:
// cleanTerraformWorkspace was designed to prevent workspace-selection prompts when different
// backends are used for the same component across runs.  For workdir-enabled components the
// backend configuration is always consistent (generated fresh from the same stack config),
// so deleting .terraform/environment is not only unnecessary — it is actively harmful:
// when -reconfigure or init_run_reconfigure is also used, OpenTofu sees workspace state
// directories (terraform.tfstate.d/) but no .terraform/environment file and interprets the
// situation as a backend migration, producing the "Do you want to migrate all workspaces?"
// prompt on every apply.  Skipping the cleanup for workdir components avoids this.
func prepareInitExecution(ctx context.Context, writers provisioner.OutputWriters, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) (string, error) {
	_, isWorkdir := info.ComponentSection[provWorkdir.WorkdirPathKey].(string)
	if !isWorkdir && !info.DryRun {
		cleanTerraformWorkspace(*atmosConfig, componentPath)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	provisionCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	if err := executeBeforeInitProvisioners(
		provisionCtx,
		provisioner.HookEvent(beforeTerraformInitEvent),
		atmosConfig,
		info.ComponentSection,
		info.AuthContext,
		writers,
		&provisioner.TerraformExecContext{DryRun: info.DryRun},
	); err != nil {
		return componentPath, fmt.Errorf("provisioner execution failed: %w", err)
	}

	if workdirPath, ok := info.ComponentSection[provWorkdir.WorkdirPathKey].(string); ok && workdirPath != "" {
		log.Debug("Using workdir path for terraform command", "workdirPath", workdirPath)
		return workdirPath, nil
	}

	return componentPath, nil
}

// executeTerraformInitPhase runs `terraform init` as a pre-step before the main command.
// It prepares the init execution environment, builds the init args, and delegates to
// ExecuteShellCommand.  Returns the (possibly updated) component path.
//
// MUTUAL EXCLUSION CONTRACT: this function is called ONLY when shouldRunTerraformInit()
// returns true (i.e. SubCommand ≠ "init").  For the "init" subcommand itself,
// buildInitSubcommandArgs in terraform_execute_helpers_args.go handles the provisioner
// invocation via prepareInitExecution.  These two code paths must never both execute
// in the same command invocation or provisioners will run twice.
func (e *Executor) executeTerraformInitPhase(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, opts ...component.ExecutionOption) (string, error) {
	newPath, err := prepareInitExecution(component.ContextFromOptions(opts...), executionOutputWriters(opts...), atmosConfig, info, componentPath)
	if err != nil {
		return componentPath, err
	}

	if err = e.executeTerraformInitCommand(atmosConfig, info, newPath, varFile, opts...); err != nil {
		return newPath, err
	}

	return newPath, nil
}

// runTerraformInitSubprocess runs `terraform init` (or a forced re-init retry with recovery
// flags) with args, teeing its stdout+stderr into a buffer so callers can classify the output
// for smart-init recovery diagnostics. Shared by executeTerraformInitCommand's own init run and
// executeCommandPipeline's plan/apply-time forced re-init retry -- the one low-level piece both
// need that pkg/terraform/autoinit itself cannot own, since it talks to ExecuteShellCommandWithRetry
// and executeStreamingOrShell, both internal/exec concerns.
func (e *Executor) runTerraformInitSubprocess(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, args []string, opts ...component.ExecutionOption) (string, error) {
	preparedInfo, prepErr := providerRecoveryInfo(atmosConfig, info, componentPath)
	if prepErr != nil {
		return "", prepErr
	}
	info = preparedInfo
	opts = prerequisiteOutputOptions(info, opts)
	var buf bytes.Buffer
	stdoutW, stderrW := composeRetryCaptureWriters(opts, &buf)
	captureOpts := append(slices.Clone(opts), component.WithStdoutCapture(stdoutW), component.WithStderrCapture(stderrW))

	err := ExecuteShellCommandWithRetry(
		atmosConfig,
		info,
		"init",
		func(o ...component.ExecutionOption) error {
			return e.executeStreamingOrShell(atmosConfig, info, &streamingExecRequest{
				componentPath:  componentPath,
				args:           args,
				gatePhase:      subcommandInit,
				subCommand:     subcommandInit,
				redirectStdErr: info.RedirectStdErr,
				shellOpts:      o,
			})
		},
		captureOpts...,
	)
	return buf.String(), err
}

// prerequisiteOutputOptions keeps initialization diagnostics out of a data command's stdout.
// Explicit init and ordinary plan/apply retain their existing terminal and capture behavior.
func prerequisiteOutputOptions(info *schema.ConfigAndStacksInfo, opts []component.ExecutionOption) []component.ExecutionOption {
	if info.SubCommand != "output" && info.SubCommand != "show" && (info.SubCommand != "state" || info.SubCommand2 != "pull") {
		return opts
	}
	var options component.ExecutionOptions
	for _, opt := range opts {
		opt(&options)
	}
	streams := process.OSStreams()
	if options.Streams != nil {
		streams = *options.Streams
	}
	if streams.Stderr == nil {
		streams.Stderr = process.OSStreams().Stderr
	}
	// Both subprocess pumps now share stderr. Serialize their display and capture
	// writes together, including callers that use one buffer for both destinations.
	writeMu := &sync.Mutex{}
	streams.Stderr = &syncWriter{w: streams.Stderr, sharedMu: writeMu}
	capture := options.StderrCapture
	if capture != nil {
		capture = &syncWriter{w: capture, sharedMu: writeMu}
	}
	return append(slices.Clone(opts), component.WithProcessStreams(streams),
		component.WithStdoutOverride(streams.Stderr), component.WithStdoutCapture(capture), component.WithStderrCapture(capture))
}

// executeTerraformInitCommand is the "smart init" decision point: it fingerprints this
// invocation's init inputs, compares them against the marker recorded by the last successful
// init (autoinit.Decide), and either skips the subprocess entirely or runs it with exactly the
// flags the decision calls for -- recovering once, via autoinit.RecoverInit, if terraform/tofu's
// own output says init needs -upgrade or -reconfigure after all. Unlike executeTerraformInitPhase
// it does NOT run prepareInitExecution — it skips cleanTerraformWorkspace, the
// before.terraform.init provisioners, and workdir-path resolution. Callers that have already
// performed that preparation (e.g. ExecuteTerraformShell, which fires the before.terraform.init
// provisioners itself) use this directly to avoid running the provisioners twice.
func (e *Executor) executeTerraformInitCommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, opts ...component.ExecutionOption) error {
	defer perf.Track(atmosConfig, "exec.executeTerraformInitCommand")()

	in := autoinit.InputsFromInfo(atmosConfig, info, componentPath, varFile)
	decision := autoinit.Decide(autoinit.RequestFromInfo(atmosConfig, info, in, false))

	if !decision.RunInit {
		info.InitSkipped = true
		autoinit.AnnounceSkipped(info, decision.Reason)
		return nil
	}

	tf := &atmosConfig.Components.Terraform
	args := autoinit.InitArgs(decision, tf.Init.PassVars, varFile)
	// Invalidate before attempting: if this init fails, a stale-but-still-matching marker must
	// not survive it -- see autoinit.InvalidateFromInfo's doc comment. RecordFromInfo below
	// re-records a fresh marker once (and only once) this attempt or its recovery retry succeeds.
	if !info.DryRun {
		autoinit.InvalidateFromInfo(in)
	}
	output, err := e.runTerraformInitSubprocess(atmosConfig, info, componentPath, args, opts...)
	usedArgs := args

	if err != nil {
		var rec autoinit.Recovery
		rec, err = autoinit.RecoverInit(autoinit.RecoverInitParams{
			Output:      output + "\n" + err.Error(),
			Err:         err,
			Mode:        tf.EffectiveInitMode(),
			Reconfigure: tf.EffectiveInitReconfigure(),
			Upgrade:     tf.EffectiveInitUpgrade(),
			Rerun: func(r autoinit.Recovery) error {
				_, rerunErr := e.runTerraformInitSubprocess(atmosConfig, info, componentPath, autoinit.ApplyRecovery(args, r), opts...)
				return rerunErr
			},
			Warn: func(msg string, kv ...any) { log.Warn(msg, kv...) },
		})
		if err != nil {
			return err
		}
		usedArgs = autoinit.ApplyRecovery(args, rec)
	}

	e.dispatchAfterInit(atmosConfig, info, componentPath, opts...)
	if !info.DryRun {
		autoinit.RecordFromInfo(in, usedArgs)
	}

	return nil
}

// dispatchAfterInit fires the after.terraform.init provisioners (e.g. the multi-platform
// providers-lock hook) once init has succeeded. It hands them a TerraformExecContext whose
// runner reuses the same binary, env (incl. TF_CLI_CONFIG_FILE pointing at the live proxy),
// and working directory as init, so a `providers lock` runs against the already-warm cache.
// The runner goes through executeStreamingOrShell (not a raw ExecuteShellCommand) so that,
// under --ui, `providers lock`'s own provider-fetch/checksum output (e.g. when a registry
// cache/mirror forces a cross-platform lock) is captured by the same init spinner instead of
// leaking raw terraform/opentofu output between the "Init … completed" and
// "Selected … workspace" lines. Lock completion is best-effort: a failure is logged, not
// propagated, so it never fails the user's plan/apply.
func (e *Executor) dispatchAfterInit(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption) {
	opts = prerequisiteOutputOptions(info, opts)
	execCtx := &provisioner.TerraformExecContext{
		DryRun:     info.DryRun,
		WorkingDir: componentPath,
		Run: func(args []string) error {
			return e.executeStreamingOrShell(atmosConfig, info, &streamingExecRequest{
				componentPath:  componentPath,
				args:           args,
				gatePhase:      subcommandInit,
				subCommand:     subcommandProvidersLock,
				redirectStdErr: info.RedirectStdErr,
				shellOpts:      opts,
			})
		},
	}

	ctx, cancel := context.WithTimeout(component.ContextFromOptions(opts...), 5*time.Minute)
	defer cancel()

	if err := provisioner.ExecuteProvisioners(
		ctx,
		provisioner.HookEvent(afterTerraformInitEvent),
		atmosConfig,
		info.ComponentSection,
		info.AuthContext,
		executionOutputWriters(opts...),
		execCtx,
	); err != nil {
		log.Warn("Failed to complete multi-platform provider lock", "error", err)
	}
}

// handleDeploySubcommand converts `deploy` into `apply` and ensures -auto-approve is
// added when appropriate.  When ApplyAutoApprove is set in atmos.yaml, it is also
// applied to plain `apply` subcommands.
func handleDeploySubcommand(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if info.SubCommand == subcommandDeploy {
		info.SubCommand = subcommandApply
		if !info.UseTerraformPlan && !slices.Contains(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}

	if info.SubCommand == subcommandApply && atmosConfig.Components.Terraform.ApplyAutoApprove && !info.UseTerraformPlan {
		if !slices.Contains(info.AdditionalArgsAndFlags, autoApproveFlag) {
			info.AdditionalArgsAndFlags = append(info.AdditionalArgsAndFlags, autoApproveFlag)
		}
	}
}

// logTerraformContext emits a debug log line with the full execution context
// (executable, command, component, stack, flags, working directory, inheritance chain).
func logTerraformContext(info *schema.ConfigAndStacksInfo, workingDir string) {
	command := info.SubCommand
	if info.SubCommand2 != "" {
		command = fmt.Sprintf("%s %s", info.SubCommand, info.SubCommand2)
	}

	var inheritance string
	if len(info.ComponentInheritanceChain) > 0 {
		inheritance = info.ComponentFromArg + " -> " + strings.Join(info.ComponentInheritanceChain, " -> ")
	}

	log.Debug(
		"Terraform context",
		"executable", info.Command,
		"command", command,
		logFieldComponent, info.ComponentFromArg,
		"stack", info.StackFromArg,
		"arguments and flags", info.AdditionalArgsAndFlags,
		"terraform component", info.BaseComponentPath,
		"inheritance", inheritance,
		"working directory", workingDir,
	)
}

// shouldSkipWorkspaceSetup returns true when workspace setup should be skipped.
func shouldSkipWorkspaceSetup(info *schema.ConfigAndStacksInfo) bool {
	if info.SubCommand == subcommandInit || (info.SubCommand == subcommandWorkspace && info.SubCommand2 != "") {
		return true
	}
	// `providers` subcommands (mirror, lock, schema) operate on configuration, not
	// state, so they need no workspace — and selecting one fails on never-applied
	// components.
	if strings.HasPrefix(info.SubCommand, "providers") {
		return true
	}
	if info.ComponentBackendType == "http" {
		return true
	}
	//nolint:forbidigo // TF_WORKSPACE is a Terraform convention, not an Atmos config var.
	return os.Getenv("TF_WORKSPACE") != ""
}

// runWorkspaceSetup selects (or creates) the Terraform workspace before the main command
// runs.  It is a no-op when shouldSkipWorkspaceSetup returns true.
func (e *Executor) runWorkspaceSetup(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption) error {
	if shouldSkipWorkspaceSetup(info) {
		return nil
	}

	// Default: merge workspace-select stderr into stdout and capture both streams.
	// Workspace setup is an Atmos pre-command implementation detail; Terraform may
	// print "Workspace doesn't exist" or "You're now on a new, empty workspace"
	// during normal first-run setup, which is noisy in user-facing command output.
	redirectStdErr := "/dev/stdout"
	if info.RedirectStdErr != "" {
		redirectStdErr = info.RedirectStdErr
	}

	// Merge caller-provided opts (e.g., WithEnvironment) with workspace-specific opts.
	var workspaceOutput bytes.Buffer
	wsOpts := append([]component.ExecutionOption{}, opts...)
	wsOpts = append(wsOpts, component.WithStdoutOverride(&workspaceOutput))

	err := ExecuteShellCommandWithRetry(
		atmosConfig,
		info,
		"workspace-select",
		func(o ...component.ExecutionOption) error {
			return e.executeStreamingOrShell(atmosConfig, info, &streamingExecRequest{
				componentPath:  componentPath,
				args:           []string{"workspace", "select", "-or-create", info.TerraformWorkspace},
				gatePhase:      subcommandInit,
				subCommand:     subcommandWorkspace,
				workspace:      info.TerraformWorkspace,
				redirectStdErr: redirectStdErr,
				shellOpts:      o,
			})
		},
		wsOpts...,
	)
	if err == nil {
		return nil
	}

	// Exit code 1 means the workspace doesn't exist yet; create it.
	var exitCodeErr errUtils.ExitCodeError
	if !errors.As(err, &exitCodeErr) || exitCodeErr.Code != 1 {
		return err
	}

	return e.createWorkspaceFallback(atmosConfig, info, componentPath, opts...)
}

// createWorkspaceFallback attempts to create a new workspace when select fails.
// If workspace creation also fails with exit code 1 and the workspace is already
// active (stale .terraform/environment file), it proceeds with a warning.
func (e *Executor) createWorkspaceFallback(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption) error {
	redirectStdErr := info.RedirectStdErr
	if redirectStdErr == "" {
		redirectStdErr = "/dev/stdout"
	}

	var workspaceOutput bytes.Buffer
	wsOpts := append([]component.ExecutionOption{}, opts...)
	wsOpts = append(wsOpts, component.WithStdoutOverride(&workspaceOutput))

	newErr := ExecuteShellCommandWithRetry(
		atmosConfig,
		info,
		"workspace-new",
		func(o ...component.ExecutionOption) error {
			return e.executeStreamingOrShell(atmosConfig, info, &streamingExecRequest{
				componentPath:  componentPath,
				args:           []string{"workspace", "new", info.TerraformWorkspace},
				gatePhase:      subcommandInit,
				subCommand:     subcommandWorkspace,
				workspace:      info.TerraformWorkspace,
				redirectStdErr: redirectStdErr,
				shellOpts:      o,
			})
		},
		wsOpts...,
	)
	if newErr == nil {
		return nil
	}

	// If `workspace new` also fails with exit code 1, the workspace may already be the
	// active workspace (the .terraform/environment file names it) but its state directory
	// was deleted.  In that case we are already in the correct workspace and can proceed.
	var newExitCodeErr errUtils.ExitCodeError
	if errors.As(newErr, &newExitCodeErr) && newExitCodeErr.Code == 1 &&
		e.isTerraformCurrentWorkspace(componentPath, info.TerraformWorkspace, info.ComponentEnvList) {
		log.Warn("Workspace is already active but its state directory is missing; proceeding — subsequent terraform commands may report missing state",
			"workspace", info.TerraformWorkspace)
		return nil
	}

	return newErr
}

// CleanTerraformWorkspace deletes the `.terraform/environment` file from the component directory.
// The `.terraform/environment` file contains the name of the currently selected workspace,
// helping Terraform identify the active workspace context for managing your infrastructure.
// We delete the file to prevent the Terraform prompt asking to select the default or the
// previously used workspace. This happens when different backends are used for the same component.
//
//nolint:nestif // Preserve workspace cleanup and Windows retry behavior during extraction.
func CleanTerraformWorkspace(componentPath string) {
	defer perf.Track(nil, "terraform.CleanTerraformWorkspace")()

	// Get `TF_DATA_DIR` ENV variable, default to `.terraform` if not set
	//nolint:forbidigo // TF_DATA_DIR is a Terraform convention, not an Atmos configuration variable.
	tfDataDir := os.Getenv("TF_DATA_DIR")
	if tfDataDir == "" {
		tfDataDir = ".terraform"
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(tfDataDir) {
		tfDataDir = filepath.Join(componentPath, tfDataDir)
	}

	// Ensure the path is cleaned properly
	tfDataDir = filepath.Clean(tfDataDir)

	// Construct the full file path
	filePath := filepath.Join(tfDataDir, "environment")

	// Check if the file exists before attempting deletion
	if _, err := os.Stat(filePath); err == nil {
		log.Debug("Terraform environment file found. Proceeding with deletion.", "file", filePath)

		// Use retry logic on Windows to handle file locking
		deleteErr := tfoutput.RetryOnWindows(func() error {
			return os.Remove(filePath)
		})

		if deleteErr != nil {
			log.Debug("Failed to delete Terraform environment file.", "file", filePath, "error", deleteErr)
		} else {
			log.Debug("Successfully deleted Terraform environment file.", "file", filePath)
		}
	} else if os.IsNotExist(err) {
		log.Debug("Terraform environment file not found. No action needed.", "file", filePath)
	} else {
		log.Debug("Error checking Terraform environment file.", "file", filePath, "error", err)
	}
}
