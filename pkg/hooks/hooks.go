package hooks

import (
	"fmt"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v2"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	_ "github.com/cloudposse/atmos/pkg/ci/plugins/terraform" // Register terraform CI plugin.
	_ "github.com/cloudposse/atmos/pkg/ci/providers/generic" // Register generic CI provider.
	_ "github.com/cloudposse/atmos/pkg/ci/providers/github"  // Register GitHub Actions CI provider.
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

type Hooks struct {
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
	items  map[string]Hook

	// sections is the full component section returned by
	// ExecuteDescribeComponent during GetHooks. We hold onto it because
	// preflight needs dependencies.tools, which is sibling to `hooks` in
	// the same section — and info.ComponentSection isn't necessarily
	// populated by the time RunAll fires for terraform/helmfile callers.
	sections map[string]any

	// preflightDone is set after the first RunAll has installed component
	// dependencies and verified that each hook's command resolves on the
	// resulting PATH. Subsequent RunAll calls skip the preflight so we
	// don't redo work for every lifecycle event in a single command.
	preflightDone bool
	// toolchainPATH is the PATH fragment containing toolchain-installed
	// binary directories. Populated by preflight; consumed by CommandEngine.
	toolchainPATH string
}

func (h Hooks) HasHooks() bool {
	return len(h.items) > 0
}

func GetHooks(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*Hooks, error) {
	if info.ComponentFromArg == "" || info.Stack == "" {
		return &Hooks{
			config: atmosConfig,
			info:   info,
			items:  nil,
		}, nil
	}

	// ProcessYamlFunctions must be false here. GetHooks runs in PreRunE before
	// auth credentials are provisioned (AuthManager is nil). YAML functions like
	// !terraform.state need AWS credentials to read S3 state — processing them
	// here would fail. The hooks section itself is static config (event names,
	// commands, store names) and does not use YAML functions.
	sections, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            info.ComponentFromArg,
		Stack:                info.Stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: false,
		Skip:                 []string{},
		AuthManager:          nil,
	})
	if err != nil {
		return &Hooks{}, err
	}

	hooksSection, ok := sections["hooks"].(map[string]any)
	if !ok {
		// No hooks defined or wrong type, return empty hooks.
		return &Hooks{
			config: atmosConfig,
			info:   info,
			items:  nil,
		}, nil
	}

	yamlData, err := yaml.Marshal(hooksSection)
	if err != nil {
		return &Hooks{}, fmt.Errorf("failed to marshal hooksSection: %w", err)
	}

	var items map[string]Hook
	err = yaml.Unmarshal(yamlData, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Hooks: %w", err)
	}

	hooks := Hooks{
		config:   atmosConfig,
		info:     info,
		items:    items,
		sections: sections,
	}

	return &hooks, nil
}

func (h *Hooks) RunAll(event HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	log.Debug("Running hooks", "count", len(h.items))
	skipPredicate := newSkipPredicate(viper.GetString("skip-hooks"))

	// Preflight runs once per command lifecycle: install component
	// dependencies up front and verify every hook's binary resolves before
	// any terraform action runs. Failures here surface BEFORE terraform —
	// users find out their hook is misconfigured before plan/apply takes
	// time, not after.
	if err := h.preflight(atmosConfig, info, skipPredicate); err != nil {
		return err
	}

	for name, hook := range h.items {
		if !hook.MatchesEvent(event) {
			log.Debug("Skipping hook, event not in hook events list", "hook", name, "event", event, "hook_events", hook.Events)
			continue
		}

		if skipPredicate(name) {
			log.Info("Skipping hook (--skip-hooks)", "hook", name, "kind", hook.Kind)
			continue
		}

		// CI commands are deprecated — use RunCIHooks instead, which automatically
		// triggers CI actions based on component provider bindings.
		if isDeprecatedCIKind(hook.Kind) {
			log.Debug("CI hook command deprecated, use RunCIHooks instead", "kind", hook.Kind)
			continue
		}

		kind, ok := GetKind(hook.Kind)
		if !ok {
			log.Debug("Unknown hook kind", "kind", hook.Kind)
			continue
		}

		resolved := kind.ResolveDefaults(&hook)
		if _, err := kind.Engine.Run(&ExecContext{
			Hook:          resolved,
			Kind:          kind,
			Event:         event,
			AtmosConfig:   atmosConfig,
			Info:          info,
			Cmd:           cmd,
			Args:          args,
			ToolchainPATH: h.toolchainPATH,
		}); err != nil {
			return err
		}
	}
	return nil
}

// newSkipPredicate builds the per-hook skip decision from the value of the
// --skip-hooks flag / ATMOS_SKIP_HOOKS env. Empty / "false" runs all hooks;
// "*" / "true" (set when --skip-hooks is passed without a value) skips
// everything; a comma-separated list skips only the named hooks.
func newSkipPredicate(raw string) func(string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "false") {
		return func(string) bool { return false }
	}
	if raw == "*" || strings.EqualFold(raw, "true") {
		return func(string) bool { return true }
	}
	names := make(map[string]struct{})
	for _, n := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(n); trimmed != "" {
			names[trimmed] = struct{}{}
		}
	}
	return func(name string) bool {
		_, ok := names[name]
		return ok
	}
}

// preflight installs the component's declared tool dependencies and
// verifies every hook's command resolves on the resulting PATH. Runs once
// per Hooks instance — subsequent lifecycle events reuse the cached PATH.
// Failures use the error builder so the user sees a friendly message and
// a hint pointing at dependencies.tools.
func (h *Hooks) preflight(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, skipPredicate func(string) bool) error {
	defer perf.Track(atmosConfig, "hooks.Hooks.preflight")()

	if h.preflightDone {
		return nil
	}
	h.preflightDone = true

	if len(h.items) == 0 || atmosConfig == nil || info == nil {
		return nil
	}
	if !h.hasUnskippedHooks(skipPredicate) {
		return nil
	}

	deps, err := h.resolveDeps(atmosConfig, info)
	if err != nil {
		return err
	}
	if err := h.installDeps(atmosConfig, info, deps); err != nil {
		return err
	}
	return h.verifyAllBinaries(skipPredicate)
}

func (h *Hooks) hasUnskippedHooks(skipPredicate func(string) bool) bool {
	for name := range h.items {
		if skipPredicate == nil || !skipPredicate(name) {
			return true
		}
	}
	return false
}

// resolveDeps walks the global → component-type → component-instance
// scopes (the same scopes ansible/custom-commands use) and returns the
// merged tool dependency map. We pass the section captured in GetHooks
// because info.ComponentSection isn't necessarily populated by the time
// RunAll fires for terraform callers.
func (h *Hooks) resolveDeps(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (map[string]string, error) {
	componentSection := h.sections
	if componentSection == nil {
		componentSection = info.ComponentSection
	}
	stackSection := info.StackSection
	if stackSection == nil {
		stackSection = componentSection
	}

	resolver := dependencies.NewResolver(atmosConfig)
	deps, err := resolver.ResolveComponentDependencies(
		cfg.TerraformComponentType,
		stackSection,
		componentSection,
	)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrDependencyResolution).
			WithCause(err).
			WithExplanation("Failed to resolve component dependencies for hooks").
			WithHint("Check that dependencies.tools in your stack manifest names valid registry entries").
			Err()
	}
	return deps, nil
}

// installDeps installs missing tools and updates h.toolchainPATH to
// point at the installed pinned versions. No-op when deps is empty.
func (h *Hooks) installDeps(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, deps map[string]string) error {
	if len(deps) == 0 {
		return nil
	}
	log.Debug(
		"Installing hook dependencies",
		"component", info.ComponentFromArg,
		"stack", info.Stack,
		"tools", deps,
	)
	installer := dependencies.NewInstaller(atmosConfig)
	if err := installer.EnsureTools(deps); err != nil {
		return errUtils.Build(errUtils.ErrToolInstall).
			WithCause(err).
			WithExplanationf("Failed to install dependencies for hooks on component %q", info.ComponentFromArg).
			WithHint("Run `atmos toolchain install <tool>@<version>` manually to diagnose").
			Err()
	}

	path, err := dependencies.BuildToolchainPATH(atmosConfig, deps)
	if err != nil {
		return errUtils.Build(errUtils.ErrPathResolution).
			WithCause(err).
			WithExplanation("Failed to build toolchain PATH after installing hook dependencies").
			Err()
	}
	h.toolchainPATH = path
	return nil
}

// verifyAllBinaries checks every hook's command resolves on the
// toolchain-augmented PATH so failures surface before terraform runs.
// Store-kind hooks have no Command and are skipped naturally.
func (h *Hooks) verifyAllBinaries(skipPredicate func(string) bool) error {
	for name := range h.items {
		if skipPredicate != nil && skipPredicate(name) {
			continue
		}
		hook := h.items[name]
		if isDeprecatedCIKind(hook.Kind) {
			continue
		}
		kind, ok := GetKind(hook.Kind)
		if !ok {
			continue
		}
		resolved := kind.ResolveDefaults(&hook)
		if resolved.Command == "" {
			continue
		}
		if err := verifyCommandAvailable(resolved.Command, h.toolchainPATH); err != nil {
			return errUtils.Build(errUtils.ErrCommandNotFound).
				WithCause(err).
				WithExplanationf("Hook %q (kind %s) requires %q, which is not installed and not on PATH", name, hook.Kind, resolved.Command).
				WithHintf("Declare it in dependencies.tools (e.g. `%s: \"<version>\"`) to auto-install before terraform runs", resolved.Command).
				WithHint("Or install it manually so it appears on PATH").
				WithContext("hook", name).
				WithContext("kind", hook.Kind).
				WithContext("command", resolved.Command).
				Err()
		}
	}
	return nil
}

// verifyCommandAvailable returns nil if `name` resolves to an executable
// on the supplied toolchain PATH or on the process PATH. The same logic
// runs at hook-exec time inside exec.LookPath, but doing it here at
// preflight surfaces the failure before terraform.
func verifyCommandAvailable(name, toolchainPATH string) error {
	if name == "" {
		return nil
	}
	_, err := resolveBinaryOnPath(name, toolchainPATH)
	return err
}

// isDeprecatedCIKind reports whether the given kind name was one of the
// pre-deprecation `ci.*` hook commands. These continue to parse but no-op;
// modern CI handling lives in RunCIHooks (driven by component provider bindings).
func isDeprecatedCIKind(kind string) bool {
	switch kind {
	case "ci.check", "ci.output", "ci.summary", "ci.upload", "ci.download":
		return true
	}
	return false
}

// RunCIHooksOptions configures a RunCIHooks invocation.
type RunCIHooksOptions struct {
	// Event is the hook event (e.g., "after.terraform.deploy").
	Event HookEvent

	// AtmosConfig is the Atmos configuration.
	AtmosConfig *schema.AtmosConfiguration

	// Info contains component and stack information.
	Info *schema.ConfigAndStacksInfo

	// Output is the captured command output to process.
	Output string

	// ForceCIMode forces CI mode even when environment detection fails (--ci flag).
	ForceCIMode bool

	// CommandError is the error from the command execution, if any (nil on success).
	CommandError error

	// ExitCode is the exit code from the command execution. This is the
	// authoritative signal plugins use to determine success/failure and (for
	// `terraform plan` with -detailed-exitcode) change detection. Pass 0 on success.
	ExitCode int
}

// RunCIHooks executes CI actions based on provider bindings.
// This is called automatically during command execution if CI is enabled.
func RunCIHooks(opts *RunCIHooksOptions) error {
	defer perf.Track(opts.AtmosConfig, "hooks.RunCIHooks")()

	log.Debug("Running CI hooks", "event", opts.Event, "force_ci", opts.ForceCIMode)

	// ci.enabled in atmos.yaml is the authority — if not set or false, CI is off.
	// The --ci flag / ATMOS_CI env var only controls provider fallback (generic vs auto-detect),
	// it cannot override a disabled config.
	if opts.AtmosConfig != nil && !opts.AtmosConfig.CI.Enabled {
		log.Debug("CI integration disabled in config (ci.enabled is not true)")
		return nil
	}

	// CI integration is experimental. Check settings.experimental to decide
	// whether to proceed, warn, or block — mirroring command-level behavior.
	// This runs after the ci.enabled check so the warning only appears when CI is active.
	if opts.AtmosConfig != nil {
		if err := checkExperimental(opts.AtmosConfig); err != nil {
			return err
		}
	}

	// Execute CI actions based on provider bindings.
	return ci.Execute(ci.ExecuteOptions{
		Event:        string(opts.Event),
		AtmosConfig:  opts.AtmosConfig,
		Info:         opts.Info,
		Output:       opts.Output,
		ForceCIMode:  opts.ForceCIMode,
		CommandError: opts.CommandError,
		ExitCode:     opts.ExitCode,
	})
}

// ciExperimentalFeature is the feature name used in experimental warnings for CI hooks.
const ciExperimentalFeature = "ci"

// checkExperimental checks settings.experimental and returns an error if CI
// hooks should not run. Mirrors the command-level experimental gating in cmd/root.go.
func checkExperimental(atmosConfig *schema.AtmosConfiguration) error {
	mode := atmosConfig.Settings.Experimental
	if mode == "" {
		mode = "warn" // Default matches command-level behavior.
	}

	switch mode {
	case "silence":
		// Proceed without any warning.
		return nil
	case "disable":
		log.Debug("CI hooks disabled by settings.experimental=disable")
		return errUtils.Build(errUtils.ErrExperimentalDisabled).
			WithContext("feature", ciExperimentalFeature).
			WithHint("Enable with settings.experimental: warn").
			Err()
	case "warn":
		ui.Experimental(ciExperimentalFeature)
		return nil
	case "error":
		ui.Experimental(ciExperimentalFeature)
		return errUtils.Build(errUtils.ErrExperimentalRequiresIn).
			WithContext("feature", ciExperimentalFeature).
			WithHint("Enable with settings.experimental: warn").
			Err()
	default:
		// Unknown mode — treat as warn for forward compatibility.
		ui.Experimental(ciExperimentalFeature)
		return nil
	}
}
