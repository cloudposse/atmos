package terraform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/cmd/terraform/shared"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/auth"
	authtypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/ci"
	citerraform "github.com/cloudposse/atmos/pkg/ci/plugins/terraform"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	h "github.com/cloudposse/atmos/pkg/hooks"
	iolib "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/proexec"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store/authbridge"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// errWrapFormat is the format string for wrapping errors with a cause.
const errWrapFormat = "%w: %w"

// ciHookFailedMsg is the log message emitted when a CI hook fails to execute.
const ciHookFailedMsg = "CI hook execution failed"

// logKeyComponent is the structured-log key for a component name.
const logKeyComponent = "component"

// logKeyStack is the structured-log key for a stack name.
const logKeyStack = "stack"

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
	// Resolve path-based component arguments before getting hooks. GetHooks calls
	// ExecuteDescribeComponent which needs a valid component name, not a raw path.
	if info.NeedsPathResolution && info.ComponentFromArg != "" {
		if err := resolveComponentPath(&info, cfg.TerraformComponentType); err != nil {
			return hookContext{info: info, atmosConfig: atmosConfig}, err
		}
	}

	// InitCliConfig processes stack configuration on its private copy of info.
	// Hooks need that resolved information too: in particular, FinalComponent
	// and ComponentFolderPrefix must reflect metadata.component before engines
	// derive a component working directory. Keep template and YAML-function
	// processing disabled here because hook discovery runs before auth.
	//
	// Multi-component invocations (--all/--affected/--components/--query/--tags/
	// --labels) fire the global before/after hook with no single component
	// resolved yet — ProcessStacks requires ComponentFromArg and would reject
	// that with "component is required". Per-component hooks inside the
	// component walker call GetHooks/RunAll directly with an already-resolved
	// component, so they are unaffected by skipping this step here.
	//
	// Best-effort: a resolution failure here (missing/invalid stack, unknown
	// component — cases prepareHookContext never used to validate before this
	// resolution step existed) must not become the command's user-facing error.
	// GetHooks (called next, in runUserHooks) already tolerates an empty Stack
	// or an unresolved component by returning no hooks, so PreRunE proceeds
	// with the original, unresolved info and RunE's own validation — not this
	// hook-prep step — produces the correct, authoritative error and message.
	if info.ComponentFromArg != "" {
		authManager, _ := info.AuthManager.(auth.AuthManager)
		if resolved, procErr := e.ProcessStacks(&atmosConfig, info, true, false, false, nil, authManager); procErr != nil {
			log.Debug("hook context: failed to resolve component metadata; hooks will use the unresolved component/stack", "error", procErr)
		} else {
			info = resolved
		}
	}
	injectHookStoreAuthResolver(&atmosConfig, &info)

	return hookContext{info: info, atmosConfig: atmosConfig}, nil
}

// componentSourceProvisionTimeout bounds JIT source provisioning triggered from
// hook context preparation, matching ExecuteTerraform's own provisioning timeout.
const componentSourceProvisionTimeout = 5 * time.Minute

// ensureComponentSourceProvisioned JIT-provisions the component's `source:`
// (if configured) so lifecycle hooks observe the same resolved, populated
// directory Terraform itself will use. No-op for components with no `source:`
// (component.ProvisionAndResolveComponentPath short-circuits on that) and for
// components that are already provisioned and not TTL-expired. Errors are
// logged, not returned: hooks should still attempt to run (and the actual
// Terraform command will surface the same provisioning error authoritatively)
// rather than aborting hook discovery over a problem unrelated to any hook.
//
// Only called when the component actually has hooks configured (see
// runUserHooks): every terraform subcommand already provisions its own
// component independently in RunE (ExecuteTerraform), so calling this
// unconditionally for every invocation would race a second, independent
// provisioning attempt against that one — observed to intermittently wipe
// or fail to (re)populate the directory for components with no hooks at all,
// which have nothing to gain from provisioning this early.
func ensureComponentSourceProvisioned(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) {
	fallbackPath, err := u.GetComponentPath(atmosConfig, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		log.Debug("hook source provisioning: failed to resolve fallback component path", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), componentSourceProvisionTimeout)
	defer cancel()
	if _, _, err := component.ProvisionAndResolveComponentPath(ctx, provisioner.OutputWriters{}, atmosConfig, info, cfg.TerraformComponentType, fallbackPath); err != nil {
		log.Debug("hook source provisioning failed; the Terraform command will report this authoritatively", "component", info.ComponentFromArg, "error", err)
	}
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
	// A `before.terraform.*` hook (other than before.terraform.init, which is
	// the provisioner's own hook) is a "run before Terraform" event, not a
	// "run before the component's source is provisioned" event. Ensure a
	// configured JIT `source:` is provisioned before firing hooks for this
	// component, so ComponentPath (env var and hook subprocess cwd) points at
	// a real, populated directory instead of one that would not exist until
	// Terraform's own execution provisions it moments later.
	//
	// `init` is excluded: before.terraform.init IS the provisioner's own
	// lifecycle event (see pkg/provisioner/source's HookEventBeforeTerraformInit
	// registration) — pre-provisioning here would fire it after provisioning
	// already happened, inverting the one event whose whole point is to run
	// before the source exists.
	if hctx.info.ComponentFromArg != "" && cmd_.Name() != "init" {
		ensureComponentSourceProvisioned(&hctx.atmosConfig, &hctx.info)
	}
	hooks.SetOutcome(outcome)
	// The outcome status describes the terraform operation, not the hooks. On
	// before-events the operation has not run yet, so logging "status=success"
	// there reads as a hook result and misleads when a hook then fails.
	if strings.HasPrefix(string(event), "before.") {
		log.Info("Running hooks", "event", event)
	} else {
		log.Info("Running hooks", "event", event, "status", outcome.Status)
	}
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
	subCommand    string
	beforeEvent   h.HookEvent
	afterEvent    h.HookEvent
	skipPerNodeCI bool

	// mu guards results, accumulated concurrently as the scheduler dispatches
	// graph nodes (research.md Decision 11/17).
	mu      sync.Mutex
	results []any
}

// execNodeResult is one {action, address} resource-change entry within a
// single component's TerraformExecData.changes list (FR-006, data-model.md
// Decision 17) — populated by terraformResourceChanges from a parsed
// terraform plan/apply/deploy output. Not to be confused with a
// multi-component graph node's own identity/outcome, which since FR-006a's
// restructure (spec.md Session 2026-08-21) is a full TerraformExecData entry
// in terraformNodeHooks.results, not this type.
type execNodeResult struct {
	Action  string `json:"action,omitempty"`
	Address string `json:"address,omitempty"`
}

// recordExecResult accumulates one multi-component graph node's full,
// single-component-shaped TerraformExecData entry (FR-006a's restructured
// {"components": [TerraformExecData, ...]} shape) for the aggregate
// exec-metadata record built after the whole graph run completes. Safe for
// concurrent use by multiple in-flight scheduler nodes. Skips nodes whose
// subCommand isn't covered by TerraformExecData at all (buildTerraformExecData
// returns nil) — terraformNodeHooks fires for every graph node regardless of
// subcommand, but only plan/apply/deploy runs reach this shape's coverage.
func (n *terraformNodeHooks) recordExecResult(info *schema.ConfigAndStacksInfo, output string, execErr error) {
	exitCode := 0
	if execErr != nil {
		exitCode = errUtils.GetExitCode(execErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	data := buildTerraformExecData(n.subCommand, output, info.Component, info.Stack, exitCode)
	if data == nil {
		return
	}
	data = stripComponentVersion(data)

	n.mu.Lock()
	n.results = append(n.results, data)
	n.mu.Unlock()
}

// terraformOutputDataMirror locally mirrors the JSON shape of
// pkg/ci/internal/plugin.TerraformOutputData so cmd/terraform can decode
// citerraform.ParseOutput's result without importing that internal package
// directly (which would reintroduce the cmd/terraform -> internal/exec ->
// pkg/ci/plugins/terraform -> internal/exec import cycle research.md
// Decision 12 identified). Calling the parser itself is safe: its return
// type is only ever accessed here via a JSON round-trip, never named.
type terraformOutputDataMirror struct {
	ResourceCounts struct {
		Create  int `json:"Create"`
		Change  int `json:"Change"`
		Replace int `json:"Replace"`
		Destroy int `json:"Destroy"`
	} `json:"ResourceCounts"`
	CreatedResources  []string `json:"CreatedResources"`
	UpdatedResources  []string `json:"UpdatedResources"`
	ReplacedResources []string `json:"ReplacedResources"`
	DeletedResources  []string `json:"DeletedResources"`
	MovedResources    []struct {
		From string `json:"From"`
		To   string `json:"To"`
	} `json:"MovedResources"`
	ImportedResources []string                   `json:"ImportedResources"`
	Outputs           map[string]json.RawMessage `json:"Outputs"`
	HasOutputChanges  bool                       `json:"HasOutputChanges"`
	ChangedResult     string                     `json:"ChangedResult"`
	Warnings          []string                   `json:"Warnings"`
}

// terraformOutputResultMirror mirrors pkg/ci/internal/plugin.OutputResult's
// JSON shape. HasChanges/HasErrors/Errors are the parser's own top-level
// status fields (research.md Decision 20) — previously decoded and then
// silently discarded, since only Data was read from this struct.
type terraformOutputResultMirror struct {
	HasChanges bool                       `json:"HasChanges"`
	HasErrors  bool                       `json:"HasErrors"`
	Errors     []string                   `json:"Errors"`
	Data       *terraformOutputDataMirror `json:"Data"`
}

// parseTerraformOutputMirror calls citerraform.ParseOutput (safe from
// cmd/terraform per research.md Decision 17) and decodes its result via a
// JSON round-trip into terraformOutputResultMirror, without ever importing or
// naming pkg/ci/internal/plugin directly (which would reintroduce the
// cmd/terraform -> internal/exec -> pkg/ci/plugins/terraform -> internal/exec
// import cycle, research.md Decisions 12/17/18). Used by buildTerraformExecData,
// itself shared by both the single-component (US3, Decisions 18/20) and, via
// recordExecResult, the multi-component (US2/US3, FR-006a) exec-metadata
// paths. "deploy"
// is parsed as "apply", matching pkg/ci/plugins/terraform's own
// onAfterDeploy override ("deploy is semantically apply for CI purposes").
// Returns (nil, false) for subcommands with no terraform-shaped output,
// empty output, or when parsing finds nothing.
// These constants name the two subcommands this feature's structured Data
// shape covers, shared by parseTerraformOutputMirror and
// terraformCoveredSubcommand to avoid repeating the "plan"/"apply" string
// literals (golangci-lint revive add-constant).
const (
	terraformSubCommandPlan  = "plan"
	terraformSubCommandApply = "apply"
)

func parseTerraformOutputMirror(subCommand, output string) (*terraformOutputResultMirror, bool) {
	if output == "" {
		return nil, false
	}

	parseCommand := subCommand
	if parseCommand == "deploy" {
		parseCommand = terraformSubCommandApply
	}
	if parseCommand != terraformSubCommandPlan && parseCommand != terraformSubCommandApply {
		return nil, false
	}

	raw, err := json.Marshal(citerraform.ParseOutput(output, parseCommand))
	if err != nil {
		return nil, false
	}

	var parsed terraformOutputResultMirror
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Data == nil {
		return nil, false
	}

	return &parsed, true
}

// terraformResourceChanges flattens a terraformOutputDataMirror's per-action
// resource slices into one {action, address} entry per resource change, for
// buildTerraformExecData's changes field.
func terraformResourceChanges(data *terraformOutputDataMirror) []execNodeResult {
	var changes []execNodeResult
	appendAll := func(action string, addresses []string) {
		for _, addr := range addresses {
			changes = append(changes, execNodeResult{Action: action, Address: addr})
		}
	}
	appendAll("created", data.CreatedResources)
	appendAll("updated", data.UpdatedResources)
	appendAll("replaced", data.ReplacedResources)
	appendAll("deleted", data.DeletedResources)
	appendAll("imported", data.ImportedResources)
	for _, m := range data.MovedResources {
		changes = append(changes, execNodeResult{Action: "moved", Address: m.To})
	}
	return changes
}

// terraformExecDataVersion is TerraformExecData's own schema version
// (research.md Decision 24, FR-005a) — a plain integer, independent of the
// Atmos release version and of every other structured-Data shape's version.
// Bump only when this shape's own fields change in a way Atmos Pro needs to
// branch on.
const terraformExecDataVersion = 1

// terraformOutputMirror locally mirrors the JSON shape of a single
// plugin.TerraformOutput entry ({Value, Type, Sensitive}) so
// maskSensitiveOutputs can decode each Outputs map entry without importing
// pkg/ci/internal/plugin directly (same import-cycle rationale as
// terraformOutputDataMirror).
type terraformOutputMirror struct {
	Value     any    `json:"Value"`
	Type      string `json:"Type"`
	Sensitive bool   `json:"Sensitive"`
}

// maskSensitiveOutputs masks any output Terraform itself marks sensitive
// with pkg/io.MaskReplacement, independent of and prior to the separate
// Gitleaks-pattern masking pkg/proexec/envelope.go's maskedDataJSON applies
// to the whole Data blob afterward (FR-010a, research.md Decision 19). A
// Sensitive:true entry's Value is replaced; Type/Sensitive and every
// non-sensitive entry's Value pass through unchanged. An entry that fails to
// decode into the expected shape defaults to masked (fail-safe), consistent
// with FR-010's "exclude/mask on doubt" posture.
func maskSensitiveOutputs(outputs map[string]json.RawMessage) map[string]any {
	result := make(map[string]any, len(outputs))
	for key, raw := range outputs {
		var out terraformOutputMirror
		if err := json.Unmarshal(raw, &out); err != nil {
			result[key] = map[string]any{"value": iolib.MaskReplacement, "sensitive": true}
			continue
		}

		value := out.Value
		if out.Sensitive {
			value = iolib.MaskReplacement
		}
		result[key] = map[string]any{
			"value":     value,
			"type":      out.Type,
			"sensitive": out.Sensitive,
		}
	}
	return result
}

// redactSensitiveOutputsFromRawOutput replaces every literal occurrence of a
// Terraform-sensitive-flagged output's own value within text with
// iolib.MaskReplacement (FR-010a's extension to the logs field) — independent
// of, and prior to, encodeLogs's own Gitleaks-pattern masking pass (below).
// Only string-valued sensitive outputs are redacted this way; non-string
// values (numbers, lists, maps) don't have a single unambiguous literal-text
// form to search for and are left to the Gitleaks pass. Malformed entries are
// skipped — they already default to masked in the outputs map itself
// (maskSensitiveOutputs), and there is no decoded value here to redact.
//
// NOTE: the production parser this function's caller feeds from
// (pkg/ci/plugins/terraform's regex-based extractApplyOutputs) never sets
// Sensitive: true on any entry it produces — Terraform's own human-readable
// console output already prints "<sensitive>" in place of a sensitive
// output's real value, so extractApplyOutputs has no real value to detect
// sensitivity from or redact in the first place. This function and
// maskSensitiveOutputs both still exist and run per FR-010a's requirement
// (defense-in-depth against any future/alternate output source that does
// carry a genuine Sensitive flag with a real value), but with today's
// regex-based extraction they are effectively a no-op in practice — not a
// bug in this function, but a limitation inherited from the shared parser
// that predates this feature and is out of scope to change here (see
// research.md Decisions 33/34's retraction of a JSON-stream parser rewrite
// for the same shared-parser risk/blast-radius reasoning).
func redactSensitiveOutputsFromRawOutput(text string, outputs map[string]json.RawMessage) string {
	for _, raw := range outputs {
		var out terraformOutputMirror
		if err := json.Unmarshal(raw, &out); err != nil || !out.Sensitive {
			continue
		}
		strVal, ok := out.Value.(string)
		if !ok || strVal == "" {
			continue
		}
		text = strings.ReplaceAll(text, strVal, iolib.MaskReplacement)
	}
	return text
}

// encodeLogs masks text (Gitleaks-pattern secret masking, the same pass
// pkg/proexec/envelope.go's maskedDataJSON applies to the rest of Data) and
// base64-encodes the result for TerraformExecData's logs field. Masking MUST
// happen here, on the plaintext, before encoding — once base64-encoded,
// envelope.go's later whole-blob Gitleaks pass can no longer pattern-match
// any secret embedded inside this field's encoded bytes, so that later pass
// alone is not sufficient for this field the way it is for plain-string
// fields elsewhere in Data. Callers that already know which output values
// are Terraform-sensitive-flagged MUST also run
// redactSensitiveOutputsFromRawOutput first and pass its result here.
func encodeLogs(text string) string {
	return base64.StdEncoding.EncodeToString([]byte(iolib.MaskString(text)))
}

// terraformCoveredSubcommand reports whether subCommand is one this feature's
// structured Data shape covers at all ("plan"/"apply", with "deploy" parsed
// as "apply" per parseTerraformOutputMirror's own mapping) — distinct from
// "covered, but this specific run's output couldn't be parsed" (research.md
// Decision 29), which still gets a (defaulted) Data payload.
func terraformCoveredSubcommand(subCommand string) bool {
	parseCommand := subCommand
	if parseCommand == "deploy" {
		parseCommand = terraformSubCommandApply
	}
	return parseCommand == terraformSubCommandPlan || parseCommand == terraformSubCommandApply
}

// nonNilStrings returns v unchanged if non-nil, otherwise a non-nil empty
// slice — so encoding/json marshals an empty list as [], never null
// (research.md Decision 26).
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// nonNilChanges is nonNilStrings' execNodeResult counterpart (research.md
// Decision 26) — the "changes" field is a []execNodeResult, not []string.
func nonNilChanges(v []execNodeResult) []execNodeResult {
	if v == nil {
		return []execNodeResult{}
	}
	return v
}

// buildTerraformExecData parses one component's captured terraform
// plan/apply/deploy invocation output into the per-component TerraformExecData
// object data-model.md specifies (resource_counts/outputs/warnings/changes/
// has_changes/has_errors/errors/exit_code/component/stack/version/logs).
// Its return value is never sent as Data on its own — every call site wraps
// it (directly, or via terraformNodeHooks.recordExecResult's accumulation)
// into the single, unified {"version": ..., "components": [...]} shape via
// wrapComponentsData, whether the invocation targeted one component or many
// (spec.md FR-006a, 2026-08-21 clarification — single- and multi-component
// invocations are never structurally different). For internal/exec's
// captureExecMetadataSync (via WithExecMetadataParser, research.md
// Decision 18) to attach to the execution record. Component/stack are the
// invocation's already-resolved identity (research.md Decision 21) —
// included only when non-empty, never as an empty string, per FR-006.
// The exitCode parameter is the terraform/tofu subprocess's own exit code
// (research.md Decision 27) — the authoritative pass/fail/parse-completeness
// signal, always included, distinct from the base execution record's own
// exit code.
// The logs field is the same already-scoped, ANSI-stripped output string
// this function parses (FR-006f/Decision 32) — only the final plan/apply
// subprocess's own output, never terraform init's or workspace select's —
// base64-encoded (encodeLogs), always included even when parsing fails, so
// Atmos Pro retains the original text. Masking happens on the plaintext,
// before encoding: when parsing succeeds, any literal occurrence of a
// Terraform-sensitive-flagged output's own value is redacted first
// (FR-010a's extension, redactSensitiveOutputsFromRawOutput), then the same
// Gitleaks-pattern masking pkg/proexec/envelope.go applies to the rest of
// Data is applied directly here (encodeLogs) — required because once
// base64-encoded, envelope.go's later whole-blob pass can no longer
// pattern-match secrets embedded inside this field's encoded bytes.
// List-typed fields (changes/warnings/errors) always marshal as [], never
// null, when empty (research.md Decision 26).
//
// Returns nil only when subCommand isn't one this shape covers at all (e.g.
// "output"/"refresh"). For a covered subcommand whose output can't be parsed
// (empty output, or output the parser doesn't recognize), a defaulted
// TerraformExecData is still returned with version/exit_code/component/stack
// populated and every other field at its empty/zero/false default, rather
// than Data being omitted (research.md Decision 29) — exit_code must remain
// available precisely in the case it's needed most.
func buildTerraformExecData(subCommand, output, component, stack string, exitCode int) any {
	if !terraformCoveredSubcommand(subCommand) {
		return nil
	}

	result, ok := parseTerraformOutputMirror(subCommand, output)

	execData := map[string]any{
		"version":   terraformExecDataVersion,
		"exit_code": exitCode,
		"logs":      encodeLogs(output),
		"resource_counts": map[string]any{
			"create":  0,
			"change":  0,
			"replace": 0,
			"destroy": 0,
		},
		"outputs":     map[string]any{},
		"warnings":    []string{},
		"changes":     []execNodeResult{},
		"has_changes": false,
		"has_errors":  false,
		"errors":      []string{},
	}

	if ok {
		data := result.Data
		execData["resource_counts"] = map[string]any{
			"create":  data.ResourceCounts.Create,
			"change":  data.ResourceCounts.Change,
			"replace": data.ResourceCounts.Replace,
			"destroy": data.ResourceCounts.Destroy,
		}
		execData["outputs"] = maskSensitiveOutputs(data.Outputs)
		execData["logs"] = encodeLogs(redactSensitiveOutputsFromRawOutput(output, data.Outputs))
		execData["warnings"] = nonNilStrings(data.Warnings)
		execData["changes"] = nonNilChanges(terraformResourceChanges(data))
		execData["has_changes"] = result.HasChanges
		execData["has_errors"] = result.HasErrors
		execData["errors"] = nonNilStrings(result.Errors)
	}

	if component != "" {
		execData["component"] = component
	}
	if stack != "" {
		execData["stack"] = stack
	}

	return execData
}

// terraformCaptureShellOpts builds the stdout/stderr capture
// ShellCommandOptions shared by plan/apply/deploy's RunE. Capture always
// runs now, decoupled from ciMode (research.md Decision 18) — it's a cheap
// in-memory buffer append, and the WithExecMetadataParser closure needs the
// buffers populated regardless of whether Native CI job summaries are also
// enabled. Callers read the returned buffers themselves for their own
// ciMode-gated CI-job-summary post-processing (e.g. capturedPlanOutput).
// Component/stack are the invocation's already-resolved identity (research.md
// Decision 21), threaded through to buildTerraformExecData's Data payload —
// component is typically args[0], stack the already-parsed --stack value;
// both empty for the multi-component path, which never reaches this closure.
func terraformCaptureShellOpts(component, stack string) (opts []e.ShellCommandOption, stdoutBuf, stderrBuf *bytes.Buffer) {
	stdoutBuf = &bytes.Buffer{}
	stderrBuf = &bytes.Buffer{}
	opts = []e.ShellCommandOption{
		e.WithStdoutCapture(stdoutBuf),
		e.WithStderrCapture(stderrBuf),
		e.WithExecMetadataParser(terraformExecMetadataParserFunc(component, stack)),
	}
	return opts, stdoutBuf, stderrBuf
}

// terraformExecMetadataParserFunc builds the closure passed to
// WithExecMetadataParser. The output parameter is supplied by the caller at
// invocation time — internal/exec's executeCommandPipeline captures it scoped
// to only the final plan/apply/deploy subprocess invocation (FR-006f,
// research.md Decision 32), not the combined init+workspace-select+main
// buffer stdoutBuf/stderrBuf (above) accumulate for other consumers. Split
// out from terraformCaptureShellOpts so it's directly unit-testable. The
// exitCode parameter is likewise supplied by the caller at invocation time
// (captureExecMetadataSync already computes it from
// info.ExecMetadataRawExitCode, research.md Decision 27) — not captured
// here, since neither value is known when this closure is created.
func terraformExecMetadataParserFunc(component, stack string) func(subCommand string, exitCode int, output string) any {
	return func(subCommand string, exitCode int, output string) any {
		data := buildTerraformExecData(subCommand, ansi.Strip(output), component, stack, exitCode)
		if data == nil {
			return nil
		}
		return wrapComponentsData(stripComponentVersion(data))
	}
}

// stripComponentVersion deletes the "version" key from a buildTerraformExecData
// result before it becomes one entry in a components list — redundant with
// the outer {"version": ..., "components": [...]} wrapper's own version
// (spec.md FR-006a, Decision 38). No-op if data isn't a map (shouldn't
// happen — buildTerraformExecData always returns map[string]any or nil).
func stripComponentVersion(data any) any {
	if m, ok := data.(map[string]any); ok {
		delete(m, "version")
	}
	return data
}

// wrapComponentsData wraps one or more per-component TerraformExecData
// entries in the single, unified Data shape every terraform plan/apply/deploy
// invocation now uses — {"version": terraformExecDataVersion, "components":
// [...]} — whether the invocation targeted one component or many (spec.md
// FR-006a, 2026-08-21 clarification: "there should not be a difference
// between single- and multi-component invocations"). A single-component
// invocation's Data is this same shape with a one-element components list,
// not a bare TerraformExecData object.
func wrapComponentsData(entries ...any) any {
	return proexec.VersionedData(terraformExecDataVersion, "components", entries)
}

// Before implements schema.ComponentNodeHooks.
func (n *terraformNodeHooks) Before(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	return n.BeforeWithWriters(ctx, info, schema.ComponentNodeHookWriters{})
}

// BeforeWithWriters implements schema.ComponentNodeHooksWithOutput.
func (n *terraformNodeHooks) BeforeWithWriters(_ context.Context, info *schema.ConfigAndStacksInfo, writers schema.ComponentNodeHookWriters) error {
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
	return n.runUserHooksForNodeWithWriters(&atmosConfig, info, n.beforeEvent, h.Outcome{Status: h.RunSuccess}, writers)
}

// After implements schema.ComponentNodeHooks.
func (n *terraformNodeHooks) After(ctx context.Context, info *schema.ConfigAndStacksInfo, output string, execErr error) error {
	return n.AfterWithWriters(ctx, info, output, execErr, schema.ComponentNodeHookWriters{})
}

// AfterWithWriters implements schema.ComponentNodeHooksWithOutput.
func (n *terraformNodeHooks) AfterWithWriters(_ context.Context, info *schema.ConfigAndStacksInfo, output string, execErr error, writers schema.ComponentNodeHookWriters) error {
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
	hookErr := n.runUserHooksForNodeWithWriters(&atmosConfig, info, n.afterEvent, outcome, writers)

	if !n.skipPerNodeCI {
		n.runCIHooksForNode(&atmosConfig, info, output, execErr)
	}

	n.recordExecResult(info, output, execErr)

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
	return n.runUserHooksForNodeWithWriters(atmosConfig, info, event, outcome, schema.ComponentNodeHookWriters{})
}

func (n *terraformNodeHooks) runUserHooksForNodeWithWriters(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, event h.HookEvent, outcome h.Outcome, writers schema.ComponentNodeHookWriters) error {
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
		Stdout:      writers.Stdout,
		Stderr:      writers.Stderr,
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
	case "output":
		return h.BeforeTerraformOutput, h.AfterTerraformOutput, true
	case "refresh":
		return h.BeforeTerraformRefresh, h.AfterTerraformRefresh, true
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
		subCommand:  subCommand,
		beforeEvent: before,
		afterEvent:  after,
		// deploy never uses the aggregate CI handler above (it isn't in that
		// switch), so it always gets a per-node CI hook regardless of CI mode —
		// matching the CI-hook behavior this wiring replaces.
		skipPerNodeCI: ciAggregate && subCommand != "deploy",
	}
}

// captureMultiComponentExecMetadata reports exactly one execution record for
// a whole multi-component graph run (FR-006a), once it has fully completed,
// wrapping each node's accumulated full TerraformExecData entry
// (terraformNodeHooks.recordExecResult) into the single aggregate record's
// Data field as {"version": terraformExecDataVersion, "components": [...]}
// (spec.md Session 2026-08-21 restructure — supersedes the prior flat
// {component, stack, exitCode, action, address} shape). No-op if no
// NodeHooks were wired (e.g. the subcommand isn't in terraformHookEvents) or
// if subCommand is not on the synchronous exec-metadata allowlist (research.md
// Decisions 11/17) — matching internal/exec's per-node skip
// (captureExecMetadataSync's info.NodeHooks != nil guard) so a multi-component
// run produces exactly one record, never zero and never N.
func captureMultiComponentExecMetadata(info *schema.ConfigAndStacksInfo, subCommand string, runErr error) {
	hooks, ok := info.NodeHooks.(*terraformNodeHooks)
	if !ok || hooks == nil {
		return
	}

	commandPath := "atmos terraform " + subCommand
	if !proexec.IsSyncCommand(commandPath) {
		return
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		log.Debug("Skipping multi-component exec-metadata capture: failed to init Atmos config.", "error", err)
		return
	}

	exitCode := 0
	if runErr != nil {
		exitCode = errUtils.GetExitCode(runErr)
		if exitCode == 0 {
			exitCode = 1
		}
	}

	flags := proexec.FlagsFromCommand(hooks.cmd)

	hooks.mu.Lock()
	components := hooks.results
	hooks.mu.Unlock()

	var data any
	if len(components) > 0 {
		data = wrapComponentsData(components...)
	}

	in := &proexec.ExecRecordInput{Command: "terraform " + subCommand, Flags: flags, ExitCode: exitCode, Data: data}
	if syncErr := proexec.CaptureSync(&atmosConfig, in); syncErr != nil {
		log.Debug("Exec-metadata sync capture returned an error.", "error", syncErr)
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
		runErr := executeAffectedCommand(ctx, parentCmd, args, &info)
		captureMultiComponentExecMetadata(&info, subCommand, runErr)
		return runErr
	}
	// --all routes to ExecuteTerraformAll for dependency-ordered execution.
	// --components / --query / bare `-s stack` continue to route to ExecuteTerraformQuery.
	if info.All {
		wasMultiComponentExecution = true
		log.Debug("Routing to ExecuteTerraformAll (dependency-ordered)")
		wirePerComponentHook(&info, subCommand, actualCmd, args)
		ctx, stop := terraformSignalContext(actualCmd)
		defer stop()
		runErr := e.ExecuteTerraformAllWithContext(ctx, &info)
		captureMultiComponentExecMetadata(&info, subCommand, runErr)
		return runErr
	}
	if isMultiComponentExecution(&info) {
		wasMultiComponentExecution = true
		log.Debug("Routing to ExecuteTerraformQuery (multi-component)")
		wirePerComponentHook(&info, subCommand, actualCmd, args)
		ctx, stop := terraformSignalContext(actualCmd)
		defer stop()
		runErr := e.ExecuteTerraformQueryWithContext(ctx, &info)
		captureMultiComponentExecMetadata(&info, subCommand, runErr)
		return runErr
	}
	wasMultiComponentExecution = false

	// Verify the stored planfile matches current state before deploying.
	if verifyErr := verifyStoredPlanForDeploy(subCommand, &info); verifyErr != nil {
		return verifyErr
	}

	// WithInvokingCommand lets ExecuteTerraform's exec-metadata sync capture
	// derive Flags from actualCmd's own record of explicitly-set flags
	// (FR-003b), instead of a pass-through-args collection that cannot
	// represent atmos-recognized flags like -s/--stack (research.md Decision 14).
	shellOpts = append(shellOpts, e.WithInvokingCommand(actualCmd))

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
			logKeyComponent, info.ComponentFromArg, logKeyStack, info.Stack)
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
		logKeyComponent, info.ComponentFromArg, logKeyStack, info.Stack)
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
		if err = handlePromptError(err, logKeyComponent); err != nil {
			return err
		}
		info.ComponentFromArg = component
	}

	// Prompt for stack if missing.
	if info.Stack == "" {
		stack, err := promptForStack(cmd, info.ComponentFromArg)
		if err = handlePromptError(err, logKeyStack); err != nil {
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
