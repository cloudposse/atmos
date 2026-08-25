package ui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Terraform subcommand constants.
const (
	subCommandApply   = "apply"
	subCommandPlan    = "plan"
	subCommandDestroy = "destroy"
	subCommandInit    = "init"
)

// Flag constants.
const (
	flagAutoApprove     = "-auto-approve"
	flagJSON            = "-json"
	flagCompactWarnings = "-compact-warnings"
)

// cancelledExitCode is returned when the user explicitly cancels a streaming run (Ctrl-C/q),
// matching the conventional shell exit code for SIGINT (128 + 2).
const cancelledExitCode = 130

// Format string constants.
const (
	fmtNewlineStr = "\n%s"
	fmtDuration   = " (%.1fs)"
	fmtWrapErr    = "%w: %w"
)

// ExecuteOptions configures streaming execution.
type ExecuteOptions struct {
	Command    string   // terraform or opentofu binary.
	Args       []string // Command arguments (plan, apply, etc.).
	WorkingDir string   // Component directory.
	Env        []string // Environment variables.
	Component  string   // Component name for display.
	Stack      string   // Stack name for display.
	SubCommand string   // "plan", "apply", "init", "refresh", "workspace".
	Workspace  string   // Workspace name (for workspace select/new).
	DryRun     bool     // If true, don't execute.
	// RenderConfig controls dependency-tree rendering (compact spacing, attribute bar,
	// max lines). Nil uses RenderTreeWithConfig's built-in defaults.
	RenderConfig *RenderConfig
}

// checkStreamingUIPreconditions returns an error if the streaming UI cannot run in the
// current environment (no TTY, or running in CI).
func checkStreamingUIPreconditions() error {
	if !term.IsTTYSupportForStdout() {
		return errUtils.ErrStreamingNotSupported
	}
	if telemetry.IsCI() {
		return errUtils.ErrStreamingNotSupported
	}
	return nil
}

// checkConfirmationPreconditions returns an error if a confirmation prompt (ConfirmApply/
// ConfirmDestroy) will not be possible - i.e. stdin isn't a TTY. Callers that will need
// confirmation should check this BEFORE running the (potentially expensive, real-work) plan
// phase, so a non-interactive stdin fails fast instead of running the plan twice: once via
// streaming (which then fails at the confirmation step) and once again via the plain fallback
// path triggered by that failure.
func checkConfirmationPreconditions() error {
	if !term.IsTTYSupportForStdin() {
		return errUtils.ErrStreamingNotSupported
	}
	return nil
}

// extractExitCode extracts the process exit code from a command's wait error.
// Returns (exitCode, nil) if err is nil or an *exec.ExitError; otherwise returns
// (0, err) unchanged so the caller can propagate the original error.
func extractExitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, err
	}
	return exitErr.ExitCode(), nil
}

// execCommandContext creates the exec.Cmd used to run terraform subprocesses. It is a
// package-level var (mirroring exec.CommandContext's signature) so tests can substitute a
// fake subprocess - e.g. the test binary itself, per this package's self-re-exec pattern in
// TestKillIfCancelled_KillsRealProcess - instead of spawning a real terraform binary.
var execCommandContext = exec.CommandContext

// runTeaProgram runs the given bubbletea program to completion. It is a package-level var so
// tests can inject a fake that returns immediately instead of driving a real terminal.
var runTeaProgram = func(p *tea.Program) (tea.Model, error) {
	return p.Run()
}

// cancellableModel is implemented by TUI models that can report whether the user explicitly
// cancelled the run (Ctrl-C/q) rather than the underlying terraform command completing on its
// own. Bubbletea returns models by value, so this must be satisfied by the value type.
type cancellableModel interface {
	Cancelled() bool
}

// runTUIProgram runs a bubbletea program to completion, killing the given command's process
// if the TUI itself fails to run, or if the user explicitly cancelled (Ctrl-C/q) - otherwise
// the command would keep running invisibly after control returns to the caller. The second
// return value reports whether the run was cancelled.
func runTUIProgram(model tea.Model, cmd *exec.Cmd) (tea.Model, bool, error) {
	p := tea.NewProgram(
		model,
		tea.WithOutput(iolib.UI),
		tea.WithoutSignalHandler(),
	)

	finalModel, err := runTeaProgram(p)
	if err != nil {
		// Kill the process if TUI failed.
		_ = cmd.Process.Kill()
		return nil, false, fmt.Errorf(fmtWrapErr, errUtils.ErrTUIRun, err)
	}

	cancelled := modelWasCancelled(finalModel)
	killIfCancelled(cancelled, cmd)

	return finalModel, cancelled, nil
}

// modelWasCancelled reports whether the given final TUI model indicates the user explicitly
// cancelled the run, defaulting to false for models that don't implement cancellableModel.
func modelWasCancelled(finalModel tea.Model) bool {
	cm, ok := finalModel.(cancellableModel)
	return ok && cm.Cancelled()
}

// killIfCancelled kills cmd's process when cancelled is true, so a subprocess left running
// after the user quits the TUI doesn't keep running invisibly in the background.
func killIfCancelled(cancelled bool, cmd *exec.Cmd) {
	if cancelled {
		_ = cmd.Process.Kill()
	}
}

// streamStderrToLog copies lines from r to the Atmos debug logger.
func streamStderrToLog(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Debug(scanner.Text())
	}
}

// newStreamingCommand creates and starts a terraform command with separate stdout/stderr
// pipes: stdout is returned for JSON message streaming, stderr is streamed to the logger.
func newStreamingCommand(ctx context.Context, opts *ExecuteOptions, args []string) (*exec.Cmd, io.ReadCloser, error) {
	cmd := execCommandContext(ctx, opts.Command, args...)
	cmd.Dir = opts.WorkingDir
	cmd.Env = opts.Env
	cmd.Stdin = os.Stdin // Allow interactive terraform commands.

	// Get stdout pipe for streaming.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf(fmtWrapErr, errUtils.ErrStdoutPipe, err)
	}

	// Capture stderr for non-JSON diagnostics (backend errors, plugin failures, etc.).
	// Terraform outputs human-readable warnings to stderr even with -json flag,
	// and some critical errors (Go runtime panics, plugin crashes) only appear on stderr.
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, fmt.Errorf(fmtWrapErr, errUtils.ErrStderrPipe, err)
	}

	// Stream stderr to logger in background.
	go streamStderrToLog(stderrPipe)

	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf(fmtWrapErr, errUtils.ErrCommandStart, err)
	}

	return cmd, stdout, nil
}

// Execute runs a terraform command with streaming UI output.
// Returns an error with exit code preserved via errUtils.ExitCodeError.
func Execute(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.Execute")()

	if err := checkStreamingUIPreconditions(); err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}

	args := buildArgsWithJSON(opts.Args)
	cmd, stdout, err := newStreamingCommand(ctx, opts, args)
	if err != nil {
		return err
	}

	// Create and run TUI model.
	model := NewModel(opts.Component, opts.Stack, opts.SubCommand, stdout)
	finalModel, cancelled, err := runTUIProgram(model, cmd)
	if err != nil {
		return err
	}

	// Wait for command to finish (reaps the process; if cancelled, it was just killed above).
	waitErr := cmd.Wait()
	if cancelled {
		return errUtils.ExitCodeError{Code: cancelledExitCode}
	}

	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		return err
	}

	// Check if model has an error.
	m, ok := finalModel.(Model)
	if !ok {
		return fmt.Errorf("%w: expected Model, got %T", errUtils.ErrUnexpectedModelType, finalModel)
	}

	return finalizeExecuteResult(opts, &m, exitCode)
}

// finalizeExecuteResult logs diagnostics, resolves the final exit code (preferring the
// model's own captured exit code, if any), and displays apply outputs on success.
func finalizeExecuteResult(opts *ExecuteOptions, m *Model, exitCode int) error {
	// Log diagnostics after TUI completes (warnings appear after completion message).
	m.LogDiagnostics()

	if m.GetError() != nil {
		return m.GetError()
	}

	// If model captured an exit code, use it.
	if m.GetExitCode() != 0 {
		exitCode = m.GetExitCode()
	}

	// Return exit code error if non-zero.
	if exitCode != 0 {
		return errUtils.ExitCodeError{Code: exitCode}
	}

	// Display outputs after successful apply.
	if opts.SubCommand == subCommandApply {
		displayOutputs(m.GetTracker())
	}

	return nil
}

// showPlanTree parses the given planfile and renders the dependency tree with a badge
// summary. Silently does nothing if the planfile can't be parsed.
func showPlanTree(ctx context.Context, opts *ExecuteOptions, planFile string) {
	tree, treeErr := BuildDependencyTree(ctx, &TreeBuildOptions{
		PlanfilePath:  planFile,
		TerraformPath: opts.Command,
		WorkingDir:    opts.WorkingDir,
		Stack:         opts.Stack,
		Component:     opts.Component,
	})
	if treeErr != nil {
		return
	}

	add, change, remove := tree.GetChangeSummary()
	// Only render tree if there are changes; otherwise just show badge.
	if add > 0 || change > 0 || remove > 0 {
		ui.Writef(fmtNewlineStr, tree.RenderTreeWithConfig(opts.RenderConfig))
	}
	ui.Write(RenderChangeSummaryBadges(add, change, remove))
}

// executePlanWithUserFile runs plan using the caller-provided -out planfile, then
// displays the dependency tree parsed from it.
func executePlanWithUserFile(ctx context.Context, opts *ExecuteOptions, planFile string) error {
	if err := Execute(ctx, opts); err != nil {
		return err
	}
	showPlanTree(ctx, opts, planFile)
	return nil
}

// executePlanWithTempFile generates a temporary planfile, runs plan into it, displays the
// dependency tree, then cleans up the temp file.
func executePlanWithTempFile(ctx context.Context, opts *ExecuteOptions) error {
	planFile := filepath.Join(opts.WorkingDir, fmt.Sprintf(".atmos-plan-%d.tfplan", time.Now().UnixNano()))
	defer os.Remove(planFile)

	// Add -out flag to args (copy slice to avoid modifying original).
	planOpts := *opts
	planOpts.Args = append(append([]string(nil), opts.Args...), "-out="+planFile)

	if err := Execute(ctx, &planOpts); err != nil {
		return err
	}

	showPlanTree(ctx, opts, planFile)
	return nil
}

// ExecutePlan runs terraform plan with streaming UI and displays the dependency tree.
// It generates a temp planfile to parse for the tree, then cleans it up.
func ExecutePlan(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecutePlan")()

	// Check if user already specified -out flag - if so, use their planfile.
	if userPlanFile := extractOutFlag(opts.Args); userPlanFile != "" {
		return executePlanWithUserFile(ctx, opts, userPlanFile)
	}

	return executePlanWithTempFile(ctx, opts)
}

// initCommandResult holds the running init/workspace command along with the merged
// output reader and a channel that closes once the command has finished.
type initCommandResult struct {
	cmd    *exec.Cmd
	reader io.Reader
	done   <-chan struct{}
	err    *error
}

// newInitCommand starts a terraform init/workspace command, merging stdout and stderr
// into a single pipe so all output is captured by the TUI. The command is waited on in
// a background goroutine so the TUI can stream output concurrently.
func newInitCommand(ctx context.Context, opts *ExecuteOptions) (*initCommandResult, error) {
	cmd := execCommandContext(ctx, opts.Command, opts.Args...)
	cmd.Dir = opts.WorkingDir
	cmd.Env = opts.Env
	cmd.Stdin = os.Stdin // Allow interactive terraform commands.

	// Use a pipe to merge stdout and stderr into a single stream.
	// This ensures all terraform output is captured by the TUI.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf(fmtWrapErr, errUtils.ErrCommandStart, err)
	}

	// Capture exit code from the goroutine.
	var cmdErr error
	cmdDone := make(chan struct{})
	go func() {
		cmdErr = cmd.Wait()
		pw.Close()
		close(cmdDone)
	}()

	return &initCommandResult{cmd: cmd, reader: pr, done: cmdDone, err: &cmdErr}, nil
}

// ExecuteInit runs terraform init with a spinner TUI that captures output.
// The output is shown in a viewport that clears when init completes.
func ExecuteInit(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteInit")()

	if err := checkStreamingUIPreconditions(); err != nil {
		return err
	}
	if opts.DryRun {
		return nil
	}

	res, err := newInitCommand(ctx, opts)
	if err != nil {
		return err
	}

	// Create and run TUI model.
	model := NewInitModel(opts.Component, opts.Stack, opts.SubCommand, res.reader, WithWorkspace(opts.Workspace))
	finalModel, cancelled, err := runTUIProgram(model, res.cmd)
	if err != nil {
		return err
	}

	// Wait for command to finish (should already be done since pipe closed; if cancelled, the
	// process was just killed above, which unblocks the background cmd.Wait() goroutine).
	<-res.done
	if cancelled {
		return errUtils.ExitCodeError{Code: cancelledExitCode}
	}

	return finalizeExecuteInitResult(finalModel, *res.err)
}

// finalizeExecuteInitResult resolves the final exit code (preferring the model's own captured
// exit code, if any) and returns an error for any non-zero result.
func finalizeExecuteInitResult(finalModel tea.Model, waitErr error) error {
	exitCode, err := extractExitCode(waitErr)
	if err != nil {
		return err
	}

	m, ok := finalModel.(InitModel)
	if !ok {
		return fmt.Errorf("%w: expected InitModel, got %T", errUtils.ErrUnexpectedModelType, finalModel)
	}
	if m.GetError() != nil {
		return m.GetError()
	}

	// If model captured an exit code, use it.
	if m.GetExitCode() != 0 {
		exitCode = m.GetExitCode()
	}

	// Return exit code error if non-zero.
	if exitCode != 0 {
		return errUtils.ExitCodeError{Code: exitCode}
	}

	return nil
}

// isStreamingUIRequested determines whether the user has opted in to the streaming UI
// via the --ui flag or atmos config, honoring an explicit --ui=false override.
func isStreamingUIRequested(uiFlagSet, uiFlag, configEnabled bool) bool {
	if uiFlagSet && !uiFlag {
		return false
	}
	return (uiFlagSet && uiFlag) || configEnabled
}

// supportsStreamingUISubCommand returns true for subcommands with streaming UI support.
// Refresh is excluded since it doesn't have good -json streaming support.
func supportsStreamingUISubCommand(subCommand string) bool {
	switch subCommand {
	case subCommandPlan, subCommandApply, subCommandInit, subCommandDestroy:
		return true
	default:
		return false
	}
}

// WouldAttemptStreamingUI reports whether the streaming UI would actually be launched for
// this invocation, independent of which specific subcommand/phase is running: the user (or
// atmos config) opted in, and the environment can support it (a real TTY, not CI). Callers
// that need to reject a combination before dispatch (e.g. --ui with concurrent multi-component
// execution, which would race multiple full-screen TUI sessions for the same terminal) use
// this instead of ShouldUseStreamingUI, since the exact phase/gate isn't known yet at that point.
func WouldAttemptStreamingUI(uiFlagSet, uiFlag, configEnabled bool) bool {
	defer perf.Track(nil, "terraform.ui.WouldAttemptStreamingUI")()

	if !isStreamingUIRequested(uiFlagSet, uiFlag, configEnabled) {
		return false
	}

	// Auto-disable in CI environments or when stdout is not a TTY (piped output).
	return !telemetry.IsCI() && term.IsTTYSupportForStdout()
}

// ShouldUseStreamingUI determines if streaming UI should be used.
// This checks the flag, config, TTY availability, and CI environment.
func ShouldUseStreamingUI(uiFlagSet, uiFlag, configEnabled bool, subCommand string) bool {
	defer perf.Track(nil, "terraform.ui.ShouldUseStreamingUI")()

	return WouldAttemptStreamingUI(uiFlagSet, uiFlag, configEnabled) && supportsStreamingUISubCommand(subCommand)
}

// UIRequestedButUnsupported reports whether the user explicitly opted in to the streaming UI
// (via --ui or atmos config) for a subcommand that doesn't support it (e.g. refresh), as
// opposed to streaming simply being unrequested, disabled by CI, or unavailable due to no TTY.
// Callers use this to warn the user instead of silently falling back to the plain execution
// path with no explanation.
func UIRequestedButUnsupported(uiFlagSet, uiFlag, configEnabled bool, subCommand string) bool {
	defer perf.Track(nil, "terraform.ui.UIRequestedButUnsupported")()

	return isStreamingUIRequested(uiFlagSet, uiFlag, configEnabled) && !supportsStreamingUISubCommand(subCommand)
}

// ExecuteApply runs terraform apply with optional confirmation.
// If -auto-approve is not present and not using --from-plan, it runs:
// (1) terraform plan -json -out=<temp> (with TUI)
// (2) Display dependency tree
// (3) Confirmation prompt
// (4) terraform apply -json <temp> (with TUI).
func ExecuteApply(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteApply")()

	// Check if -auto-approve is in args - skip confirmation.
	if hasFlag(opts.Args, flagAutoApprove) {
		return Execute(ctx, opts)
	}

	// Every remaining path below requires confirmation; fail fast if it won't be possible.
	if err := checkConfirmationPreconditions(); err != nil {
		return err
	}

	// Check if applying from existing plan - show tree and confirm.
	if planFile := extractPlanFile(opts.Args); planFile != "" {
		return executeWithPlanFile(ctx, opts, planFile)
	}

	// Two-phase: Plan → Tree → Confirm → Apply.
	return executeTwoPhaseApply(ctx, opts)
}

// generateTwoPhasePlanFile builds a unique temp planfile path for the plan phase of a
// two-phase apply/destroy operation.
func generateTwoPhasePlanFile(workingDir string, isDestroy bool) string {
	pattern := ".atmos-plan-%d.tfplan"
	if isDestroy {
		pattern = ".atmos-destroy-%d.tfplan"
	}
	return filepath.Join(workingDir, fmt.Sprintf(pattern, time.Now().UnixNano()))
}

// runTwoPhasePlan runs the plan phase (with -destroy when applicable) into planFile.
func runTwoPhasePlan(ctx context.Context, opts *ExecuteOptions, planFile string, isDestroy bool) error {
	var planArgs []string
	if isDestroy {
		planArgs = buildDestroyPlanArgs(opts.Args, planFile)
	} else {
		planArgs = buildPlanArgs(opts.Args, planFile)
	}

	planOpts := *opts
	planOpts.Args = planArgs
	planOpts.SubCommand = subCommandPlan

	return Execute(ctx, &planOpts)
}

// showTwoPhasePlanTree parses planFile and renders the dependency tree with badges.
// Returns true if the plan has no changes. If the planfile can't be parsed, it returns
// false so the caller proceeds to the confirmation phase (matching prior behavior).
func showTwoPhasePlanTree(ctx context.Context, opts *ExecuteOptions, planFile string) bool {
	tree, treeErr := BuildDependencyTree(ctx, &TreeBuildOptions{
		PlanfilePath:  planFile,
		TerraformPath: opts.Command,
		WorkingDir:    opts.WorkingDir,
		Stack:         opts.Stack,
		Component:     opts.Component,
	})
	if treeErr != nil {
		return false
	}

	add, change, remove := tree.GetChangeSummary()
	if add == 0 && change == 0 && remove == 0 {
		// No changes to apply - show badge only.
		ui.Write(RenderChangeSummaryBadges(add, change, remove))
		return true
	}

	// Display the dependency tree and badge summary.
	ui.Writef(fmtNewlineStr, tree.RenderTreeWithConfig(opts.RenderConfig))
	ui.Write(RenderChangeSummaryBadges(add, change, remove))
	return false
}

// confirmTwoPhaseOperation prompts the user to confirm the apply/destroy, printing a
// cancellation message if declined.
func confirmTwoPhaseOperation(isDestroy bool) (bool, error) {
	var confirmed bool
	var err error
	if isDestroy {
		confirmed, err = ConfirmDestroy()
	} else {
		confirmed, err = ConfirmApply()
	}
	if err != nil {
		return false, err
	}

	if !confirmed {
		cancelMsg := "Apply cancelled"
		if isDestroy {
			cancelMsg = "Destroy cancelled"
		}
		ui.Warning(cancelMsg)
	}

	return confirmed, nil
}

// applyTwoPhasePlan runs the apply phase from the already-confirmed planFile.
func applyTwoPhasePlan(ctx context.Context, opts *ExecuteOptions, planFile string) error {
	applyOpts := *opts
	applyOpts.Args = buildApplyArgs(planFile)
	applyOpts.SubCommand = subCommandApply

	return Execute(ctx, &applyOpts)
}

// executeTwoPhaseOperation executes a plan → confirm → apply workflow.
// The isDestroy parameter determines whether this is a destroy operation (affects planfile name and confirmation).
func executeTwoPhaseOperation(ctx context.Context, opts *ExecuteOptions, isDestroy bool) error {
	planFile := generateTwoPhasePlanFile(opts.WorkingDir, isDestroy)
	defer os.Remove(planFile)

	if err := runTwoPhasePlan(ctx, opts, planFile, isDestroy); err != nil {
		return err
	}

	if showTwoPhasePlanTree(ctx, opts, planFile) {
		// No changes to apply - show current outputs and exit.
		fetchAndDisplayOutputs(opts.Command, opts.WorkingDir, opts.Env)
		return nil
	}

	confirmed, err := confirmTwoPhaseOperation(isDestroy)
	if err != nil {
		return err
	}
	if !confirmed {
		return nil
	}

	return applyTwoPhasePlan(ctx, opts, planFile)
}

func executeTwoPhaseApply(ctx context.Context, opts *ExecuteOptions) error {
	return executeTwoPhaseOperation(ctx, opts, false)
}

// ExecuteDestroy runs terraform destroy with confirmation.
// It runs:
// (1) terraform plan -destroy -json -out=<temp> (with TUI)
// (2) Display dependency tree
// (3) Confirmation prompt
// (4) terraform apply -json <temp> (with TUI).
func ExecuteDestroy(ctx context.Context, opts *ExecuteOptions) error {
	defer perf.Track(nil, "terraform.ui.ExecuteDestroy")()

	// Check if -auto-approve is in args - skip confirmation.
	if hasFlag(opts.Args, flagAutoApprove) {
		return Execute(ctx, opts)
	}

	// The two-phase flow below requires confirmation; fail fast if it won't be possible.
	if err := checkConfirmationPreconditions(); err != nil {
		return err
	}

	// Two-phase: Plan → Tree → Confirm → Apply.
	return executeTwoPhaseDestroy(ctx, opts)
}

func executeTwoPhaseDestroy(ctx context.Context, opts *ExecuteOptions) error {
	return executeTwoPhaseOperation(ctx, opts, true)
}

func executeWithPlanFile(ctx context.Context, opts *ExecuteOptions, planFile string) error {
	// Parse planfile and check for changes.
	tree, err := BuildDependencyTree(ctx, &TreeBuildOptions{
		PlanfilePath:  planFile,
		TerraformPath: opts.Command,
		WorkingDir:    opts.WorkingDir,
		Stack:         opts.Stack,
		Component:     opts.Component,
	})
	if err == nil {
		// Check if there are any changes.
		add, change, remove := tree.GetChangeSummary()
		if add == 0 && change == 0 && remove == 0 {
			// No changes to apply - show badge, outputs, and exit.
			ui.Write(RenderChangeSummaryBadges(add, change, remove))
			fetchAndDisplayOutputs(opts.Command, opts.WorkingDir, opts.Env)
			return nil
		}
		// Display the dependency tree and badge summary.
		ui.Writef(fmtNewlineStr, tree.RenderTreeWithConfig(opts.RenderConfig))
		ui.Write(RenderChangeSummaryBadges(add, change, remove))
	}

	// Confirm.
	confirmed, err := ConfirmApply()
	if err != nil {
		return err
	}
	if !confirmed {
		ui.Warning("Apply cancelled")
		return nil
	}

	// Execute apply.
	return Execute(ctx, opts)
}
