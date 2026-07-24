package terraform

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/shared"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	h "github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
)

// errWrapFormat is the format string for wrapping errors with a cause.
const errWrapFormat = "%w: %w"

// ciHookFailedMsg is the log message emitted when a CI hook fails to execute.
const ciHookFailedMsg = "CI hook execution failed"

// logKeyComponent is the structured-log key for a component name.
const logKeyComponent = "component"

// verifyPlanFlagName is the tri-state planfile-verify flag (--verify-plan,
// --verify-plan=false).
const verifyPlanFlagName = "verify-plan"

// multiComponentPlaceholder satisfies the legacy compound-subcommand parser while
// fleet options are applied immediately afterward. It never reaches Terraform.
const multiComponentPlaceholder = "__atmos_multi_component__"

// ciHookConfigInitFailedMsg is the log message emitted when CI-hook config init fails.
const ciHookConfigInitFailedMsg = "CI hook config init failed"

// wasMultiComponentExecution records whether the most recent terraformRunWithOptions call
// was routed to ExecuteTerraformQuery. Read in plan.go and deploy.go PostRunE to suppress
// the global CI hook call when per-component hooks already fired inside the component walker.
var wasMultiComponentExecution bool

// preResolvedComponent carries the interactively-selected component from PreRunE
// (runBeforeHooks) into the before-hooks and RunE. The selected stack is persisted
// to the --stack flag by PromptForStack, but the component is a positional arg with
// no flag to write back, so it is threaded through this package var instead. Set by
// preResolveInteractiveSelection; consumed by applyPreResolvedComponent.
var preResolvedComponent string

// multiComponentFlagNames are the flags that put terraform into multi-component
// mode, where interactive single-component/stack selection does not apply.
var multiComponentFlagNames = []string{"all", "affected", "components", "query", "tags", "labels"}

// runBeforeHooks resolves interactive component/stack selection BEFORE firing the
// before-hooks, so lifecycle hooks (e.g. a `kind: step` emulator hook on
// before.terraform.test) operate on the chosen target instead of an empty one. With
// explicit args or in non-interactive contexts it is a no-op beyond the normal hook run.
func runBeforeHooks(event h.HookEvent, cmd_ *cobra.Command, args []string) error {
	if err := validateTerraformMockFlags(cmd_); err != nil {
		return err
	}
	if err := preResolveInteractiveSelection(cmd_, args); err != nil {
		return err
	}
	return runHooks(event, cmd_, args)
}

// validateTerraformMockFlags rejects an invalid mock invocation before hook or
// stack resolution. RunE repeats this validation for commands without hooks and
// for values supplied through environment variables.
func validateTerraformMockFlags(cmd_ *cobra.Command) error {
	if cmd_ == nil || cmd_.Flags().Lookup("use-mocks") == nil {
		return nil
	}

	useMocks, err := cmd_.Flags().GetBool("use-mocks")
	if err != nil || !useMocks {
		return err
	}
	processFunctions, err := cmd_.Flags().GetBool("process-functions")
	if err != nil {
		return err
	}
	return validateTerraformMockOptions(cmd_.Name(), useMocks, processFunctions)
}

// preResolveInteractiveSelection prompts for a missing component/stack up front (when
// interactive and single-component), persisting the stack to the --stack flag and the
// component to preResolvedComponent so both the before-hooks and RunE observe the
// selection. It resets preResolvedComponent on every call.
func preResolveInteractiveSelection(cmd_ *cobra.Command, args []string) error {
	preResolvedComponent = ""

	// Multi-component invocations (--all/--affected/--components/--query) have no single
	// component/stack to select; leave them to the normal flow.
	if isMultiComponentInvocation(cmd_) {
		return nil
	}

	finalArgs := append([]string{cmd_.Name()}, args...)
	info, err := e.ProcessCommandLineArgs(cfg.TerraformComponentType, cmd_, finalArgs, compat.GetSeparated())
	if err != nil {
		return err
	}

	// resolveAndPromptForArgs is a no-op when not interactive or when both values are
	// already provided; otherwise it shows the component/stack pickers.
	if err := resolveAndPromptForArgs(&info, cmd_); err != nil {
		return err
	}

	preResolvedComponent = info.ComponentFromArg
	return nil
}

// isMultiComponentInvocation reports whether any multi-component flag is set.
// It checks explicit Cobra flags first, then Viper for env/config-driven values,
// because this runs in PreRunE before applyOptionsToInfo has populated info.
func isMultiComponentInvocation(cmd_ *cobra.Command) bool {
	for _, name := range multiComponentFlagNames {
		if f := cmd_.Flags().Lookup(name); f != nil && f.Changed {
			return true
		}
	}
	v := viper.GetViper()
	return v.GetBool("all") ||
		v.GetBool("affected") ||
		len(v.GetStringSlice("components")) > 0 ||
		v.GetString("query") != "" ||
		len(v.GetStringSlice("tags")) > 0 ||
		v.GetString("labels") != ""
}

// applyPreResolvedComponent injects the interactively-selected component into info
// when info has none (the positional arg is empty after a re-parse). No-op otherwise.
func applyPreResolvedComponent(info *schema.ConfigAndStacksInfo) {
	if preResolvedComponent != "" && info.ComponentFromArg == "" {
		info.ComponentFromArg = preResolvedComponent
	}
}

func runHooks(event h.HookEvent, cmd_ *cobra.Command, args []string) error {
	return runHooksWithOutput(event, cmd_, args, "")
}

// runHooksOnError runs CI hooks with command error context.
// Used to update check runs to failure status when RunE fails
// (Cobra skips PostRunE on error, so this must be called explicitly).
func runHooksOnError(event h.HookEvent, cmd_ *cobra.Command, args []string, cmdErr error) {
	runHooksOnErrorWithOutput(event, cmd_, args, cmdErr, "")
}

// runHooksOnErrorWithOutput runs user hooks (with failure context) and CI hooks
// after a failed command. Declared as a package-level var so tests can stub it
// to verify the RunE defer-guard in deploy.go suppresses the global error hook
// in multi-component mode.
var runHooksOnErrorWithOutput = func(event h.HookEvent, cmd_ *cobra.Command, args []string, cmdErr error, output string) {
	hctx, err := prepareHookContext(cmd_, args)
	if err != nil {
		return
	}

	// Fire user-defined hooks with failure context (e.g. a `kind: step` hook on
	// `when: failure` / `always` that announces "the <component> in <stack>
	// failed"). Cobra skips PostRunE on error, so the success-path runHooks
	// never runs — this is the only place user hooks see a failed operation.
	// Errors here are advisory: never mask the original command error.
	outcome := h.Outcome{Status: h.RunFailure, Err: cmdErr, ExitCode: errUtils.GetExitCode(cmdErr)}
	if err := runUserHooks(&hctx, event, cmd_, args, outcome); err != nil {
		log.Warn("hook failed on error path", "error", err)
	}

	forceCIMode, _ := cmd_.Flags().GetBool("ci")
	if !forceCIMode {
		forceCIMode = viper.GetBool("ci")
	}

	// Extract the exit code from the command error. errUtils.GetExitCode unwraps
	// the error chain (exec.ExitError, ExecError, exitCoder, etc.) and returns 1
	// by default for non-nil errors with no attached code (e.g., auth failures).
	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:        event,
		AtmosConfig:  &hctx.atmosConfig,
		Info:         &hctx.info,
		Output:       output,
		ForceCIMode:  forceCIMode,
		CommandError: cmdErr,
		ExitCode:     errUtils.GetExitCode(cmdErr),
	}); err != nil {
		log.Warn(ciHookFailedMsg, "error", err)
	}
}

// hookContext bundles the fully-resolved component info and Atmos config shared
// by user hooks and CI hooks, so helpers can pass it as one argument.
type hookContext struct {
	info        schema.ConfigAndStacksInfo
	atmosConfig schema.AtmosConfiguration
}

// prepareHookContext builds the hook context: command-line parsing, auth-context
// injection (so store hooks can read terraform outputs from backends requiring
// role assumption), config validation/init, the store auth resolver, and
// path resolution.
func prepareHookContext(cmd_ *cobra.Command, args []string) (hookContext, error) {
	// Note: Double-dash processing is handled by AtmosFlagParser in terraformRun
	// (RunE); hooks run afterward and only need component/stack info.
	finalArgs := append([]string{cmd_.Name()}, args...)

	info, err := e.ProcessCommandLineArgs("terraform", cmd_, finalArgs, nil)
	if err != nil {
		return hookContext{info: info}, err
	}

	// Honor a component chosen interactively in PreRunE so before-hooks resolve
	// against the selected component (the stack comes from the persisted --stack flag).
	applyPreResolvedComponent(&info)

	if authCtx, authMgr := e.GetLastAuthContext(); authCtx != nil {
		info.AuthContext = authCtx
		info.AuthManager = authMgr
	}

	// Validate Atmos config first to provide specific error messages
	// (e.g., stacks directory does not exist) before full initialization.
	if err := internal.ValidateAtmosConfig(); err != nil {
		return hookContext{info: info}, err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return hookContext{info: info, atmosConfig: atmosConfig}, errors.Join(errUtils.ErrInitializeCLIConfig, err)
	}
	injectHookStoreAuthResolver(&atmosConfig, &info)

	// Resolve path-based component arguments before getting hooks. GetHooks calls
	// ExecuteDescribeComponent which needs a valid component name, not a raw path.
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := resolveComponentPath(&info, cfg.TerraformComponentType); err != nil {
			return hookContext{info: info, atmosConfig: atmosConfig}, err
		}
	}

	return hookContext{info: info, atmosConfig: atmosConfig}, nil
}

// runUserHooks runs user-defined hooks from stack configuration for the given
// event, attaching the operation outcome (success/failure) so hooks can filter
// on `when` and report what happened.
func runUserHooks(hctx *hookContext, event h.HookEvent, cmd_ *cobra.Command, args []string, outcome h.Outcome) error {
	hooks, err := h.GetHooks(&hctx.atmosConfig, &hctx.info)
	if err != nil {
		return err
	}
	if hooks == nil || !hooks.HasHooks() {
		return nil
	}
	hooks.SetOutcome(outcome)
	log.Info("Running hooks", "event", event, "status", outcome.Status)
	return hooks.RunAll(event, &hctx.atmosConfig, &hctx.info, cmd_, args)
}

func runHooksWithOutput(event h.HookEvent, cmd_ *cobra.Command, args []string, output string) error {
	hctx, err := prepareHookContext(cmd_, args)
	if err != nil {
		return err
	}

	// Success path: user hooks see a successful outcome (when: success / always).
	if err := runUserHooks(&hctx, event, cmd_, args, h.Outcome{Status: h.RunSuccess}); err != nil {
		return err
	}

	// Check for --ci flag or CI environment variable.
	// Read directly from Cobra flag (not Viper) because pflags are only bound
	// to Viper in RunE via BindFlagsToViper. During PreRunE, Viper doesn't
	// yet see the Cobra flag value — only env vars and defaults.
	forceCIMode, _ := cmd_.Flags().GetBool("ci")
	if !forceCIMode {
		// Fall back to Viper for env var support (ATMOS_CI, CI).
		forceCIMode = viper.GetBool("ci")
	}

	// Read the verify-plan flag early (same pattern as --ci above). PreRunE runs
	// before RunE, so info is not yet populated by applyOptionsToInfo(). The
	// before.terraform.deploy hook reads the resulting CLI override to decide
	// whether to download the stored planfile (skipped when verification is off).
	hctx.info.VerifyPlanMode = resolveVerifyPlanMode(cmd_)

	// Run CI hooks based on component provider bindings.
	// This is separate from user-defined hooks and runs automatically when CI is enabled.
	// Success path: cmdErr is nil and exit code is 0.
	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:       event,
		AtmosConfig: &hctx.atmosConfig,
		Info:        &hctx.info,
		Output:      output,
		ForceCIMode: forceCIMode,
	}); err != nil {
		log.Warn(ciHookFailedMsg, "error", err)
		// Don't fail the command on CI hook errors.
	}

	return nil
}

// resolveVerifyPlanMode resolves the explicit planfile-verify override from the
// tri-state --verify-plan flag: fail for --verify-plan(=true), off for
// --verify-plan=false, empty when the flag was not set (defer to config / the CI
// default).
//
// It delegates to deployParser.IsBoolFlagExplicitlySet which uses
// cmd.Flags().Changed for CLI detection and os.LookupEnv over the flag's
// registered env vars (from the flags registry) for environment detection.
// We deliberately avoid viper.IsSet here: SetDefault registers a default that
// makes IsSet return true even when neither the CLI flag nor the env var was
// provided — collapsing the unset case to off and silently disabling
// verification (a missing stored plan would no longer block the deploy).
func resolveVerifyPlanMode(cmd *cobra.Command) schema.PlanfileVerifyMode {
	set, verify := deployParser.IsBoolFlagExplicitlySet(cmd, verifyPlanFlagName)
	if !set {
		return ""
	}
	return verifyPlanModeFromBool(verify)
}

// verifyPlanModeFromBool maps the resolved --verify-plan boolean to its mode:
// true forces fail, false forces off.
func verifyPlanModeFromBool(verify bool) schema.PlanfileVerifyMode {
	if verify {
		return schema.PlanfileVerifyFail
	}
	return schema.PlanfileVerifyOff
}

// injectHookStoreAuthResolver wires the resolved auth manager from info into
// atmosConfig as the store auth-context resolver so stores invoked by hooks can
// resolve credentials lazily. It is a no-op when the inputs are nil or info holds
// no usable AuthManager.
//
// Stores that omit `identity` inherit the run's auto-detected identity (the same one the apply ran
// as), matching the main terraform path. Without this, the after-apply `store-outputs` hook would
// fall back to the default AWS SDK credential chain — which is empty under Atmos auth (credentials
// live in the keyring, not the environment) — and fail with "no EC2 IMDS role found". Inheritance is
// guarded by HookStoreDefaultIdentity (returns "" when no identity is resolved), so runs without
// Atmos auth keep their prior ambient/default-credential behavior.
func injectHookStoreAuthResolver(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	if atmosConfig == nil || info == nil || info.AuthManager == nil {
		return
	}

	authManager, ok := info.AuthManager.(authtypes.AuthManager)
	if !ok {
		return
	}

	resolver := authbridge.NewResolver(authManager, info)
	atmosConfig.Stores.SetAuthContextResolverWithDefaultIdentity(resolver, e.HookStoreDefaultIdentity(authManager, info))
}

// runCIHooksForDeploy fires CI hooks using already-resolved info.
// Unlike runHooksWithOutput, this avoids a second ProcessCommandLineArgs call
// which would eagerly resolve !store YAML functions and fail if referenced
// components haven't been deployed yet.
func runCIHooksForDeploy(event h.HookEvent, cmd_ *cobra.Command, _ []string, info *schema.ConfigAndStacksInfo, output string) {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Warn(ciHookConfigInitFailedMsg, "error", err)
		return
	}

	forceCIMode, _ := cmd_.Flags().GetBool("ci")
	if !forceCIMode {
		forceCIMode = viper.GetBool("ci")
	}

	// Before-event hook (e.g., before.terraform.deploy): no command has run yet,
	// so there is no exit code or error to report.
	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:       event,
		AtmosConfig: &atmosConfig,
		Info:        info,
		Output:      output,
		ForceCIMode: forceCIMode,
	}); err != nil {
		log.Warn(ciHookFailedMsg, "error", err)
	}
}

// terraformNodeHooks implements schema.ComponentNodeHooks for one
// multi-component Terraform run. It fires user-defined hooks.RunAll (the
// actual bug fix — user hooks previously had no wiring at all in bulk
// dispatch) and, unless an aggregate TerraformPlanCIResultHandler already
// owns CI output for this run, also fires CI hooks.RunCIHooks per node —
// superseding the former runCIHooksForPlanComponent/DeployComponent/
// ApplyComponent wrappers.
type terraformNodeHooks struct {
	cmd           *cobra.Command
	args          []string
	beforeEvent   h.HookEvent
	afterEvent    h.HookEvent
	skipPerNodeCI bool
}

// Before implements schema.ComponentNodeHooks.
func (n *terraformNodeHooks) Before(_ context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "terraform.terraformNodeHooks.Before")()

	injectLastAuthContext(info)
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Warn(ciHookConfigInitFailedMsg, logKeyComponent, info.Component, "error", err)
		return nil // Config errors surface on the real execution path, not here.
	}
	// Each graph node initializes its own configuration. Reinstall the store auth
	// resolver from the node's authenticated context before running hooks so
	// identity-aware store hooks (for example, after-apply output publishing)
	// do not fall back to ambient credentials.
	injectHookStoreAuthResolver(&atmosConfig, info)
	return n.runUserHooksForNode(&atmosConfig, info, n.beforeEvent, h.Outcome{Status: h.RunSuccess})
}

// After implements schema.ComponentNodeHooks.
func (n *terraformNodeHooks) After(_ context.Context, info *schema.ConfigAndStacksInfo, output string, execErr error) error {
	defer perf.Track(nil, "terraform.terraformNodeHooks.After")()

	injectLastAuthContext(info)
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Warn(ciHookConfigInitFailedMsg, logKeyComponent, info.Component, "error", err)
		return nil
	}
	// See Before: cfg.InitCliConfig creates a fresh store registry for every
	// scheduler node, so its resolver must be restored before after-hooks use it.
	injectHookStoreAuthResolver(&atmosConfig, info)

	outcome := h.Outcome{Status: h.RunSuccess}
	if execErr != nil {
		outcome = h.Outcome{Status: h.RunFailure, Err: execErr, ExitCode: errUtils.GetExitCode(execErr)}
	}
	hookErr := n.runUserHooksForNode(&atmosConfig, info, n.afterEvent, outcome)

	if !n.skipPerNodeCI {
		n.runCIHooksForNode(&atmosConfig, info, output, execErr)
	}
	return hookErr
}

// injectLastAuthContext makes the credentials and endpoint selected for the
// aggregate command available to its per-component hooks. The executor has
// already authenticated before it creates the node info; without this bridge,
// after hooks that read Terraform state start a fresh unauthenticated output
// process (notably losing an emulator's S3 endpoint).
func injectLastAuthContext(info *schema.ConfigAndStacksInfo) {
	if info == nil || info.AuthContext != nil {
		return
	}
	if authCtx, authMgr := e.GetLastAuthContext(); authCtx != nil {
		info.AuthContext = authCtx
		info.AuthManager = authMgr
	}
}

// runUserHooksForNode resolves and runs this node's user-defined hooks for a
// single lifecycle event, propagating hooks.RunPerComponentHooks' error
// verbatim: RunAll already resolves each hook's on_failure mode internally
// (applyOnFailure) — a non-nil return specifically means on_failure: fail.
func (n *terraformNodeHooks) runUserHooksForNode(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, event h.HookEvent, outcome h.Outcome) error {
	if event == "" {
		return nil
	}
	return h.RunPerComponentHooks(&h.RunPerComponentHooksOptions{
		Event:       event,
		AtmosConfig: atmosConfig,
		Info:        info,
		Cmd:         n.cmd,
		Args:        n.args,
		Outcome:     outcome,
	})
}

// runCIHooksForNode fires the CI hook for this node's after-event. CI-hook
// errors are advisory only (unrelated to a hook's on_failure setting) and are
// only logged, matching existing CI-hook behavior.
func (n *terraformNodeHooks) runCIHooksForNode(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, rawOutput string, execErr error) {
	forceCIMode, _ := n.cmd.Flags().GetBool("ci")
	if !forceCIMode {
		forceCIMode = viper.GetBool("ci")
	}

	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:        n.afterEvent,
		AtmosConfig:  atmosConfig,
		Info:         info,
		Output:       ansi.Strip(rawOutput),
		ForceCIMode:  forceCIMode,
		CommandError: execErr,
		ExitCode:     errUtils.GetExitCode(execErr),
	}); err != nil {
		log.Warn(ciHookFailedMsg, logKeyComponent, info.Component, "error", err)
	}
}

// terraformHookEvents returns the user-hook before/after event pair for a
// Terraform subcommand, and whether user hooks are supported for it. Destroy
// is intentionally excluded — no before/after per-component user-hook events
// exist for it yet (a separate, pre-existing gap: single-component destroy
// doesn't wire user hooks either).
func terraformHookEvents(subCommand string) (before, after h.HookEvent, ok bool) {
	switch subCommand {
	case "plan":
		return h.BeforeTerraformPlan, h.AfterTerraformPlan, true
	case "apply":
		return h.BeforeTerraformApply, h.AfterTerraformApply, true
	case "deploy":
		return h.BeforeTerraformDeploy, h.AfterTerraformDeploy, true
	default:
		return "", "", false
	}
}

// terraformPlanCIResultHandler forwards scheduler results into the aggregate CI hook.
type terraformPlanCIResultHandler struct {
	cmd     *cobra.Command
	info    *schema.ConfigAndStacksInfo
	command string
}

// HandleTerraformPlanCIResults initializes config and runs the aggregate CI hook.
func (handler *terraformPlanCIResultHandler) HandleTerraformPlanCIResults(resultSet schema.TerraformPlanCIResultSet) error {
	if handler == nil || handler.cmd == nil || handler.info == nil {
		return nil
	}

	command := handler.command
	if command == "" {
		command = resultSet.Command
	}
	if command == "" {
		command = handler.info.SubCommand
	}
	resultSet.Command = command

	atmosConfig, err := cfg.InitCliConfig(*handler.info, true)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrInitializeCLIConfig, err)
	}

	if err := h.RunCIHooks(&h.RunCIHooksOptions{
		Event:       terraformAggregateEvent(command),
		AtmosConfig: &atmosConfig,
		Info:        handler.info,
		ForceCIMode: terraformCIModeEnabled(handler.cmd),
		Aggregate:   resultSet,
	}); err != nil {
		return err
	}
	return nil
}

// terraformAggregateEvent returns the aggregate CI hook event for a Terraform command.
func terraformAggregateEvent(command string) h.HookEvent {
	switch command {
	case "apply":
		return h.AfterTerraformApplyAggregate
	case "destroy":
		return h.AfterTerraformDestroyAggregate
	default:
		return h.AfterTerraformPlanAggregate
	}
}

// terraformCIModeEnabled returns true when CLI, config, or native provider detection enables CI mode.
func terraformCIModeEnabled(cmd *cobra.Command) bool {
	forceCIMode := false
	if cmd != nil {
		forceCIMode, _ = cmd.Flags().GetBool("ci")
	}
	if forceCIMode {
		return true
	}
	if viper.GetBool("ci") {
		return true
	}
	return ci.IsCI()
}

// wirePerComponentHook installs per-component lifecycle hooks on info for a
// multi-component Terraform run (`--all`, `--affected`, `--components`,
// `--query`): user-defined hooks.RunAll (the actual bug fix — this previously
// had no wiring at all in bulk dispatch) and, unless an aggregate
// TerraformPlanCIResultHandler already owns CI output for this run, CI hooks
// too. The wiring is identical for --affected/--all/--query dispatch paths;
// keep it in one place so a new subcommand only needs to be added once.
func wirePerComponentHook(info *schema.ConfigAndStacksInfo, subCommand string, actualCmd *cobra.Command, args []string) {
	ciAggregate := false
	if terraformCIModeEnabled(actualCmd) {
		switch subCommand {
		case "plan", "apply", "destroy":
			info.TerraformPlanCIResultHandler = &terraformPlanCIResultHandler{
				cmd:     actualCmd,
				info:    info,
				command: subCommand,
			}
			ciAggregate = true
		}
	}

	before, after, ok := terraformHookEvents(subCommand)
	if !ok {
		return
	}

	info.NodeHooks = &terraformNodeHooks{
		cmd:         actualCmd,
		args:        args,
		beforeEvent: before,
		afterEvent:  after,
		// deploy never uses the aggregate CI handler above (it isn't in that
		// switch), so it always gets a per-node CI hook regardless of CI mode —
		// matching the CI-hook behavior this wiring replaces.
		skipPerNodeCI: ciAggregate && subCommand != "deploy",
	}
}

// resolveComponentPath resolves a path-based component argument to a component name.
// It validates the component exists in the specified stack and handles ambiguous paths.
func resolveComponentPath(info *schema.ConfigAndStacksInfo, commandName string) error {
	// Initialize config with processStacks=true to enable stack-based validation.
	// This is needed to detect ambiguous paths (multiple components referencing the same folder).
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf(errWrapFormat, errUtils.ErrPathResolutionFailed, err)
	}

	// Resolve component from path WITH stack validation.
	// This will:
	// 1. Extract the component name from the path (e.g., "vpc" from "components/terraform/vpc").
	// 2. Look up which Atmos components reference this terraform folder in the stack.
	// 3. If multiple components reference the same folder, return an ambiguous path error.
	resolvedComponent, err := e.ResolveComponentFromPath(
		&atmosConfig,
		info.ComponentFromArg,
		info.Stack,
		commandName,
	)
	if err != nil {
		return handlePathResolutionError(err)
	}

	log.Debug(
		"Resolved component from path",
		"original_path", info.ComponentFromArg,
		"resolved_component", resolvedComponent,
		"stack", info.Stack,
	)

	info.ComponentFromArg = resolvedComponent
	info.NeedsPathResolution = false // Mark as resolved.
	return nil
}

// handlePathResolutionError wraps path resolution errors with appropriate hints.
func handlePathResolutionError(err error) error {
	// These errors already have detailed hints from the resolver, return directly.
	// Using fmt.Errorf to wrap would lose the cockroachdb/errors hints.
	if errors.Is(err, errUtils.ErrAmbiguousComponentPath) ||
		errors.Is(err, errUtils.ErrComponentNotInStack) ||
		errors.Is(err, errUtils.ErrStackNotFound) ||
		errors.Is(err, errUtils.ErrUserAborted) {
		return err
	}
	// Generic path resolution error - add hint.
	// Use WithCause to preserve the underlying error for errors.Is introspection.
	return errUtils.Build(errUtils.ErrPathResolutionFailed).
		WithCause(err).
		WithHint("Make sure the path is within your component directories").
		Err()
}

// executeAffectedCommand handles the --affected flag execution flow.
func executeAffectedCommand(ctx context.Context, parentCmd *cobra.Command, args []string, info *schema.ConfigAndStacksInfo) error {
	// Add these flags because `atmos describe affected` needs them, but `atmos terraform --affected` does not define them.
	parentCmd.PersistentFlags().String("file", "", "")
	parentCmd.PersistentFlags().String("format", "yaml", "")
	parentCmd.PersistentFlags().Bool("verbose", false, "")
	parentCmd.PersistentFlags().Bool("include-spacelift-admin-stacks", false, "")
	parentCmd.PersistentFlags().Bool("include-settings", false, "")
	parentCmd.PersistentFlags().Bool("upload", false, "")

	a, err := e.ParseDescribeAffectedCliArgs(parentCmd, args)
	if err != nil {
		return err
	}

	a.IncludeSpaceliftAdminStacks = false
	a.IncludeSettings = false
	a.Upload = false
	a.OutputFile = ""

	return e.ExecuteTerraformAffectedWithContext(ctx, &a, info)
}

// isMultiComponentExecution checks if the command should be routed to multi-component execution.
// isMultiComponentExecution reports whether the parsed command targets more than one component.
func isMultiComponentExecution(info *schema.ConfigAndStacksInfo) bool {
	return shared.IsMultiComponentExecution(info)
}

// executeSingleComponent executes terraform for a single component.
func executeSingleComponent(info *schema.ConfigAndStacksInfo, shellOpts ...e.ShellCommandOption) error {
	log.Debug("Routing to ExecuteTerraform (single-component)")
	err := e.ExecuteTerraform(*info, shellOpts...)
	if err != nil {
		if errors.Is(err, errUtils.ErrPlanHasDiff) {
			errUtils.CheckErrorAndPrint(err, "", "")
		}
		return err
	}
	return nil
}

// newTerraformPassthroughSubcommand creates a Cobra subcommand that delegates to the parent
// terraform subcommand's execution flow. This enables proper Cobra command tree routing for
// compound terraform subcommands like "state list", "providers lock", etc.
//
// When invoked, the sub-subcommand prepends its name to the argument list and delegates
// to terraformRun with the parent command, which then follows the standard terraform
// execution pipeline (ProcessCommandLineArgs → ExecuteTerraform).
func newTerraformPassthroughSubcommand(parent *cobra.Command, name, short string) *cobra.Command {
	return newTerraformPassthroughSubcommandWithParsers(parent, name, short)
}

// newTerraformPassthroughSubcommandWithParsers creates a passthrough command
// with any parent-specific Atmos flag parsers required by the leaf command.
func newTerraformPassthroughSubcommandWithParsers(parent *cobra.Command, name, short string, parsers ...*flags.StandardParser) *cobra.Command {
	cmd := &cobra.Command{
		Use:                name + " [component] -s [stack]",
		Short:              short,
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
		RunE: func(leaf *cobra.Command, args []string) error {
			argsForParent := append([]string{name}, args...)

			// Cobra consumes inherited Atmos flags (such as --all and --affected)
			// on this leaf command. Bind those flags from the leaf before delegating
			// to the parent command's Terraform execution path, otherwise the
			// multi-component options are silently lost.
			opts, err := parseTerraformRunOptions(leaf, parsers...)
			if err != nil {
				return err
			}
			return terraformRunWithOptions(terraformCmd, parent, argsForParent, opts)
		},
	}
	RegisterTerraformCompletions(cmd)
	return cmd
}

// terraformRun is for simple subcommands without their own parsers.
// It binds terraformParser and delegates to terraformRunWithOptions.
func terraformRun(parentCmd, actualCmd *cobra.Command, args []string) error {
	opts, err := parseTerraformRunOptions(actualCmd)
	if err != nil {
		return err
	}
	return terraformRunWithOptions(parentCmd, actualCmd, args, opts)
}

// parseTerraformRunOptions binds the flags from the command Cobra executed and
// returns the shared Terraform run options. Compound Terraform commands must
// use their leaf command here because Cobra parses inherited flags on that leaf.
func parseTerraformRunOptions(cmd *cobra.Command, parsers ...*flags.StandardParser) (*TerraformRunOptions, error) {
	v := viper.GetViper()
	if err := terraformParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}
	for _, parser := range parsers {
		if parser == nil {
			continue
		}
		if err := parser.BindFlagsToViper(cmd, v); err != nil {
			return nil, err
		}
	}

	opts, err := ParseTerraformRunOptions(v)
	if err != nil {
		return nil, err
	}
	return opts, nil
}

// applyOptionsToInfo transfers parsed options to the info struct.
func applyOptionsToInfo(info *schema.ConfigAndStacksInfo, opts *TerraformRunOptions) {
	shared.ApplyRunOptions(info, opts)
}

// terraformRunWithOptions is the shared execution logic for terraform subcommands.
// Commands with their own parsers (plan, apply, deploy) bind their parsers in RunE.
// Optional ShellCommandOption values are forwarded to ExecuteTerraform for stdout capture, etc.
func terraformRunWithOptions(parentCmd, actualCmd *cobra.Command, args []string, opts *TerraformRunOptions, shellOpts ...e.ShellCommandOption) error {
	subCommand := actualCmd.Name()
	log.Debug("terraformRunWithOptions entry", "subCommand", subCommand, "args", args)

	if err := validateTerraformMockOptions(subCommand, opts.UseMocks, opts.ProcessFunctions); err != nil {
		return err
	}

	// Validate Atmos config first to provide specific error messages.
	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}

	// Build info from args. SeparatedArgs are terraform pass-through flags.
	separatedArgs := compat.GetSeparated()
	argsWithSubCommand := append([]string{subCommand}, args...)
	argsForProcessing := argsWithSubCommand
	insertedMultiComponentPlaceholder := false
	if (opts.All || opts.Affected || opts.Query != "" || len(opts.Components) > 0 || len(opts.Tags) > 0 || len(opts.Labels) > 0) &&
		isCompoundTerraformCommandWithoutComponent(argsWithSubCommand) {
		// ProcessCommandLineArgs predates fleet execution and requires a component
		// for compound commands (for example, `providers lock`). Supply a private
		// placeholder only for parsing, then clear it before validation and routing.
		argsForProcessing = append(append([]string{}, argsWithSubCommand...), multiComponentPlaceholder)
		insertedMultiComponentPlaceholder = true
	}
	info, err := e.ProcessCommandLineArgs(cfg.TerraformComponentType, parentCmd, argsForProcessing, separatedArgs)
	if err != nil {
		return err
	}
	if insertedMultiComponentPlaceholder {
		info.ComponentFromArg = ""
	}

	// Apply parsed options to info BEFORE prompting, so hasMultiComponentFlags() works correctly.
	// This fixes issue #1945: --all flag must be set before resolveAndPromptForArgs checks it.
	applyOptionsToInfo(&info, opts)

	// Honor a component already chosen interactively in PreRunE (runBeforeHooks) so the
	// prompt below sees both component and stack and does not ask again.
	applyPreResolvedComponent(&info)

	// Resolve the tri-state --verify-plan override from the command (reliable
	// Changed/env detection) rather than from opts, which cannot tell an unset
	// flag from --verify-plan=false through Viper. Drives the RunE verify gate.
	info.VerifyPlanMode = resolveVerifyPlanMode(actualCmd)

	// Resolve paths and prompt for missing component/stack interactively.
	if err := resolveAndPromptForArgs(&info, actualCmd); err != nil {
		return err
	}

	if info.NeedHelp {
		return actualCmd.Usage()
	}

	// Handle --identity flag for interactive selection when used without a value.
	if info.Identity == cfg.IdentityFlagSelectValue {
		if err := handleInteractiveIdentitySelection(&info); err != nil {
			return err
		}
	}

	// Check Terraform Single-Component and Multi-Component flags.
	if err = checkTerraformFlags(&info); err != nil {
		return err
	}

	// Fire before.terraform.deploy CI hook after stack processing is complete.
	// This runs inside RunE (not PreRunE) because ProcessCommandLineArgs eagerly
	// resolves !store YAML functions for all stacks, which would fail if referenced
	// components haven't been deployed yet. By running here, the hook has access
	// to the resolved info without a second ProcessCommandLineArgs call.
	if subCommand == "deploy" {
		runCIHooksForDeploy(h.BeforeTerraformDeploy, actualCmd, args, &info, "")
	}

	// Route to appropriate execution path.
	if info.Affected {
		wasMultiComponentExecution = true
		wirePerComponentHook(&info, subCommand, actualCmd, args)
		ctx, stop := terraformSignalContext(actualCmd)
		defer stop()
		return executeAffectedCommand(ctx, parentCmd, args, &info)
	}
	// --all routes to ExecuteTerraformAll for dependency-ordered execution.
	// --components / --query / bare `-s stack` continue to route to ExecuteTerraformQuery.
	if info.All {
		wasMultiComponentExecution = true
		log.Debug("Routing to ExecuteTerraformAll (dependency-ordered)")
		wirePerComponentHook(&info, subCommand, actualCmd, args)
		ctx, stop := terraformSignalContext(actualCmd)
		defer stop()
		return e.ExecuteTerraformAllWithContext(ctx, &info)
	}
	if isMultiComponentExecution(&info) {
		wasMultiComponentExecution = true
		log.Debug("Routing to ExecuteTerraformQuery (multi-component)")
		wirePerComponentHook(&info, subCommand, actualCmd, args)
		ctx, stop := terraformSignalContext(actualCmd)
		defer stop()
		return e.ExecuteTerraformQueryWithContext(ctx, &info)
	}
	wasMultiComponentExecution = false

	// Verify the stored planfile matches current state before deploying.
	if verifyErr := verifyStoredPlanForDeploy(subCommand, &info); verifyErr != nil {
		return verifyErr
	}

	return executeSingleComponent(&info, shellOpts...)
}

func isCompoundTerraformCommandWithoutComponent(args []string) bool {
	if len(args) != 2 {
		return false
	}
	switch args[0] {
	case "providers", "state", "workspace":
		return true
	default:
		return false
	}
}

func validateTerraformMockOptions(subCommand string, useMocks, processFunctions bool) error {
	if !useMocks {
		return nil
	}
	if !processFunctions {
		return fmt.Errorf("%w: --use-mocks requires --process-functions=true", errUtils.ErrInvalidFlagValue)
	}
	if subCommand != "plan" {
		return fmt.Errorf("%w: --use-mocks is supported only by `atmos terraform plan`", errUtils.ErrInvalidFlagValue)
	}
	return nil
}

// verifyStoredPlanForDeploy runs planfile drift verification before a deploy
// apply. It is a no-op for non-deploy commands, when planfile storage is not
// configured (unless --verify-plan explicitly requested verification, which then
// errors), when planfile verification is off, or when no stored planfile was
// downloaded (the stored planfile only exists when the before.terraform.deploy
// hook fetched it under CI). On a match, or under warn, it points info at the
// freshly generated plan for apply.
func verifyStoredPlanForDeploy(subCommand string, info *schema.ConfigAndStacksInfo) error {
	if subCommand != "deploy" {
		return nil
	}

	verifyAtmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		// Config errors surface on the normal execution path; nothing to verify here.
		return nil //nolint:nilerr // intentionally deferring config errors to the main path.
	}

	if v := verifyAtmosConfig.Components.Terraform.Planfiles.Verify; !v.IsValid() {
		return fmt.Errorf("%w: components.terraform.planfiles.verify %q is invalid (want fail|warn|off)", errUtils.ErrInvalidConfig, v)
	}

	// Planfile verification is opt-in via planfile storage. Without it there is no
	// stored plan to download, verify, or require (mirrors the
	// before.terraform.deploy download hook's storage gate): an explicit
	// --verify-plan request fails loudly instead of silently no-op'ing, while plain
	// `deploy` (no planfile config) proceeds untouched and free of verification
	// warnings.
	if !planfile.StorageConfigured(&verifyAtmosConfig.Components.Terraform.Planfiles) {
		return handleUnconfiguredPlanfileStorage(&verifyAtmosConfig, info)
	}

	canonicalPlanPath := e.ConstructTerraformComponentPlanfilePath(&verifyAtmosConfig, info)
	storedPlanPath := filepath.Join(filepath.Dir(canonicalPlanPath), planfile.StoredPlanPrefix+planfile.PlanFilename)
	if _, statErr := os.Stat(storedPlanPath); statErr != nil {
		// No stored planfile was downloaded. Whether that blocks the deploy is
		// configurable via components.terraform.planfiles.required (default:
		// tracks the verify mode, so a fail-by-default CI deploy fails loudly
		// instead of silently applying an unverified fresh plan).
		return handleMissingStoredPlan(&verifyAtmosConfig, info)
	}

	// A stored planfile implies the CI download hook ran, so resolve with ciEnabled=true.
	mode := planfile.ResolveVerifyMode(&verifyAtmosConfig, true, info.VerifyPlanMode)
	if mode == schema.PlanfileVerifyOff {
		return nil
	}

	return e.VerifyPlanfile(info, storedPlanPath, mode)
}

// handleUnconfiguredPlanfileStorage resolves a verification request that cannot be
// honored because no planfile storage is configured. An explicit --verify-plan /
// ATMOS_TERRAFORM_VERIFY_PLAN=true errors: verification depends on storage settings
// (stores, default, priority) the flag alone cannot stand in for. A config-set
// verify mode only warns (pre-existing configs may carry it without storage), and
// the default (nothing requested) stays silent so plain deploys are unaffected.
func handleUnconfiguredPlanfileStorage(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if info.VerifyPlanMode == schema.PlanfileVerifyFail {
		return errUtils.Build(errUtils.ErrPlanfileStorageNotConfigured).
			WithExplanationf("`--verify-plan` needs a stored planfile to verify component %q in stack %q against, but no planfile storage is configured", info.ComponentFromArg, info.Stack).
			WithHint("Configure planfile storage in `atmos.yaml` under `components.terraform.planfiles` (named `stores` plus a `default` store or a `priority` list); see https://atmos.tools/ci/planfile-storage").
			Err()
	}

	if v := atmosConfig.Components.Terraform.Planfiles.Verify; v == schema.PlanfileVerifyFail || v == schema.PlanfileVerifyWarn {
		log.Warn("components.terraform.planfiles.verify is set but planfile storage is not configured; skipping planfile verification",
			logKeyComponent, info.ComponentFromArg, "stack", info.Stack)
	}
	return nil
}

// handleMissingStoredPlan applies the configured behavior when a deploy found no
// stored planfile to verify against. When a stored plan is required it errors (a
// reviewed plan was expected); otherwise it logs and proceeds with a fresh apply.
// Whether a plan is required is resolved with real CI detection because, unlike
// the drift path, the absence of a stored plan does not imply the download hook ran.
func handleMissingStoredPlan(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	if planfile.IsPlanRequired(atmosConfig, ci.IsCI(), info.VerifyPlanMode) {
		return fmt.Errorf("%w: expected a stored planfile for component %q in stack %q but none was found",
			errUtils.ErrStoredPlanfileMissing, info.ComponentFromArg, info.Stack)
	}

	log.Warn("No stored planfile found to verify; applying a fresh plan without verification",
		logKeyComponent, info.ComponentFromArg, "stack", info.Stack)
	return nil
}

func terraformSignalContext(actualCmd *cobra.Command) (context.Context, context.CancelFunc) {
	ctx := actualCmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
}

// hasMultiComponentFlags checks if any multi-component flags are set.
func hasMultiComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return shared.HasMultiComponentFlags(info)
}

// hasNonAffectedMultiFlags checks if multi-component flags (excluding --affected) are set.
// --tags/--labels are deliberately excluded here: they compose with --affected to further
// narrow the affected set (`--affected --tags production`), rather than being an alternative
// selection mechanism that conflicts with it.
func hasNonAffectedMultiFlags(info *schema.ConfigAndStacksInfo) bool {
	return shared.HasNonAffectedMultiFlags(info)
}

// hasSingleComponentFlags checks if single-component flags are set.
func hasSingleComponentFlags(info *schema.ConfigAndStacksInfo) bool {
	return shared.HasSingleComponentFlags(info)
}

// checkTerraformFlags checks the usage of the Single-Component and Multi-Component flags.
func checkTerraformFlags(info *schema.ConfigAndStacksInfo) error {
	return shared.CheckTerraformFlags(info)
}

// handleInteractiveIdentitySelection handles the case where --identity was used without a value.
func handleInteractiveIdentitySelection(info *schema.ConfigAndStacksInfo) error {
	return shared.HandleInteractiveIdentitySelection(info)
}

// resolveAndPromptForArgs handles path resolution and interactive prompts for component/stack.
func resolveAndPromptForArgs(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	return shared.ResolveAndPromptForArgs(info, cmd)
}

// handleInteractiveComponentStackSelection prompts for missing component and stack
// when running in interactive mode. Skipped for multi-component operations.
func handleInteractiveComponentStackSelection(info *schema.ConfigAndStacksInfo, cmd *cobra.Command) error {
	// Skip if multi-component mode or help requested.
	if hasMultiComponentFlags(info) || info.NeedHelp {
		return nil
	}

	// Validate stack exists if provided via flag (fail fast before prompting or execution).
	if info.Stack != "" && info.ComponentFromArg == "" {
		if err := shared.ValidateStackExists(cmd, info.Stack); err != nil {
			return err
		}
	}

	// Both provided - nothing to do.
	if info.ComponentFromArg != "" && info.Stack != "" {
		return nil
	}

	// Prompt for component if missing.
	// If stack is already provided (via --stack flag), filter components to that stack.
	if info.ComponentFromArg == "" {
		component, err := promptForComponent(cmd, info.Stack)
		if err = handlePromptError(err, "component"); err != nil {
			return err
		}
		info.ComponentFromArg = component
	}

	// Prompt for stack if missing.
	if info.Stack == "" {
		stack, err := promptForStack(cmd, info.ComponentFromArg)
		if err = handlePromptError(err, "stack"); err != nil {
			return err
		}
		info.Stack = stack
	}

	return nil
}

// handlePromptError delegates to shared.HandlePromptError.
func handlePromptError(err error, name string) error {
	return shared.HandlePromptError(err, name)
}

// promptForComponent delegates to shared.PromptForComponent.
// If stack is provided, filters components to only those in that stack.
// Declared as a var so tests can stub the interactive prompt.
var promptForComponent = shared.PromptForComponent

// promptForStack delegates to shared.PromptForStack.
// Declared as a var so tests can stub the interactive prompt.
var promptForStack = shared.PromptForStack

// enableHeatmapIfRequested checks os.Args for --heatmap flag and enables performance tracking.
// This is needed because --heatmap must be detected before flag parsing occurs.
// We only enable tracking if --heatmap is present; --heatmap-mode is only relevant when --heatmap is set.
func enableHeatmapIfRequested() {
	enableHeatmapIfRequestedWithArgs(os.Args)
}

// enableHeatmapIfRequestedWithArgs checks the given args for --heatmap flag and enables performance tracking.
// This is a testable version of enableHeatmapIfRequested that accepts args as a parameter.
func enableHeatmapIfRequestedWithArgs(args []string) {
	for _, arg := range args {
		if arg == "--heatmap" {
			perf.EnableTracking(true)
			return
		}
	}
}

// identityFlagCompletion provides shell completion for identity flags by fetching
// available identities from the Atmos configuration.
func identityFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var identities []string
	if atmosConfig.Auth.Identities != nil {
		for name := range atmosConfig.Auth.Identities {
			identities = append(identities, name)
		}
	}

	sort.Strings(identities)

	return identities, cobra.ShellCompDirectiveNoFileComp
}

// addIdentityCompletion registers shell completion for the identity flag if present on the command.
// The identity flag may be defined directly on the command or inherited from a parent command.
func addIdentityCompletion(cmd *cobra.Command) {
	// Check both local flags and inherited flags.
	flag := cmd.Flag("identity")
	if flag == nil {
		flag = cmd.InheritedFlags().Lookup("identity")
	}
	if flag != nil {
		if err := cmd.RegisterFlagCompletionFunc("identity", identityFlagCompletion); err != nil {
			log.Trace("Failed to register identity flag completion", "error", err)
		}
	}
}

// componentsArgCompletion delegates to shared.ComponentsArgCompletion.
func componentsArgCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shared.ComponentsArgCompletion(cmd, args, toComplete)
}

// stackFlagCompletion delegates to shared.StackFlagCompletion.
func stackFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return shared.StackFlagCompletion(cmd, args, toComplete)
}
