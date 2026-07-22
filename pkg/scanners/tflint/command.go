package tflint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const terraformLintWrappedErrorFormat = "%w: %w"

// checkTFLintAvailableImpl fails fast, before any stack discovery, when tflint is not
// resolvable via the project's .tool-versions pin or the ambient PATH — the two things
// knowable without resolving any specific component's own stack config. This deliberately
// does NOT check component-level `dependencies.tools.tflint` overrides (only known after
// resolving each component), so it can under-report availability for a project that pins
// tflint per component only, with no project-wide pin and nothing on PATH — a rare pattern.
// In that case the per-target check in executeTargets (which does see the fully resolved
// toolchain) remains the correctness backstop; this is purely a fast-path optimization for
// the common case (a single project-wide pin, or nothing pinned anywhere) so a
// completely-missing tflint fails in milliseconds instead of after a full describe-stacks
// pass across the whole repo.
//
// The dependency map handed to dependencies.NewEnvironmentFromDeps is deliberately scoped
// down to tflint's own entry, if any. NewEnvironmentFromDeps installs every tool in the
// map it's given, so passing the full .tool-versions map would trigger a real install pass
// for every other pinned tool, such as terraform or kubectl, just to answer whether tflint
// is available, turning this fast-path check into a multi-minute one. See
// docs/fixes/2026-07-22-terraform-lint-fail-fast-and-progress-ux.md.
//
// On success it also returns the resolved toolchain PATH, empty if tflint came from the
// ambient PATH rather than a .tool-versions-driven install. This matters because
// executeTarget's own toolchain resolution via dependencies.ForComponent only sees
// stack/component-declared `dependencies:` blocks; it never reads .tool-versions, so a
// project-wide-only pin would otherwise be found here but invisible again by the time
// each target actually runs tflint. Callers combine this PATH with each target's own
// via combineToolchainPATH.
func checkTFLintAvailableImpl(atmosConfig *schema.AtmosConfiguration) (string, error) {
	deps, err := dependencies.LoadToolVersionsDependencies(atmosConfig)
	if err != nil {
		// Not fatal to the check itself — fall through and let the per-target path surface
		// any real .tool-versions problem with full context.
		deps = nil
	}
	env, err := dependencies.NewEnvironmentFromDeps(atmosConfig, scopeToTFLint(deps))
	if err != nil {
		return "", nil //nolint:nilerr // best-effort optimization; let the per-target path handle it.
	}
	if resolved := env.Resolve(Command); resolved != Command {
		return env.PATH(), nil
	}
	return "", errUtils.Build(errUtils.ErrCommandNotFound).
		WithCausef("%q is not declared in .tool-versions and not found on PATH", Command).
		WithHintf("Declare it in .tool-versions (e.g. `%s <version>`), or in a component's dependencies.tools, or install it and add it to PATH", Command).
		WithContext("command", Command).
		Err()
}

// combineToolchainPATH prepends the project-wide (.tool-versions-resolved) toolchain PATH
// in front of a target's own component-resolved toolchain PATH, so a project-wide pin for
// a tool no component itself declares (e.g. tflint via .tool-versions with no matching
// `dependencies:` block anywhere in the stack) still gets found at execution time — see
// checkTFLintAvailableImpl's doc comment for why dependencies.ForComponent alone can't see it.
func combineToolchainPATH(projectPATH, componentPATH string) string {
	switch {
	case projectPATH == "":
		return componentPATH
	case componentPATH == "":
		return projectPATH
	default:
		return projectPATH + string(os.PathListSeparator) + componentPATH
	}
}

// scopeToTFLint narrows a full .tool-versions dependency map down to tflint's own entry,
// if declared. It exists because dependencies.NewEnvironmentFromDeps installs every tool
// in the map it's handed, so passing the unscoped map would trigger a real install pass
// for every other pinned tool just to answer whether tflint is available.
func scopeToTFLint(deps map[string]string) map[string]string {
	scoped := map[string]string{}
	if version, ok := deps[Command]; ok {
		scoped[Command] = version
	}
	return scoped
}

// AffectedOptions contains the affected-component inputs accepted by the
// Terraform lint command. It deliberately belongs to this package so callers
// do not need to expose internal/exec command arguments.
type AffectedOptions struct {
	CloneTargetRef       bool
	ExcludeLocked        bool
	IncludeSettings      bool
	RepoPath             string
	Ref                  string
	SHA                  string
	SSHKeyPath           string
	SSHKeyPassword       string
	Stack                string
	TargetBranch         string
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	AuthDisabled         bool
	// ErrorMode is the raw --error-mode flag/env value ("", "strict", "warn", or "silent").
	// Resolved against atmos.yaml's components.terraform.lint.error_mode (then "warn") by
	// the Runtime.AffectedComponents implementation, not by this package — see
	// cmd/terraform/lint.go's resolveTerraformLintAffectedComponents.
	ErrorMode string
}

// Runtime supplies the existing stack and affected-component operations needed
// to prepare a component for linting. Keeping these adapters at the command
// boundary lets this reusable scanner package remain independent of
// internal/exec (which already depends on workflow steps that use TFLint).
type Runtime struct {
	SetupAuth      func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error)
	DescribeStacks func(
		*schema.AtmosConfiguration,
		string,
		[]string,
		[]string,
		[]string,
		bool,
		bool,
		bool,
		bool,
		[]string,
		auth.AuthManager,
		bool,
	) (map[string]any, error)
	ProcessStacks func(
		*schema.AtmosConfiguration,
		schema.ConfigAndStacksInfo,
		bool,
		bool,
		bool,
		[]string,
		auth.AuthManager,
	) (schema.ConfigAndStacksInfo, error)
	AffectedComponents func(*schema.AtmosConfiguration, *AffectedOptions, auth.AuthManager) ([]schema.Affected, error)
	// OutputFormat selects the terminal-only presentation for this command.
	OutputFormat string
}

var (
	initCLIConfig        = cfg.InitCliConfig
	buildTerraformGraph  = scheduleradapters.BuildTerraformGraph
	runTarget            = executeTarget
	checkTFLintAvailable = checkTFLintAvailableImpl
)

// Execute runs the Terraform-aware TFLint command. When affected is non-nil,
// it selects changed Terraform components; otherwise it selects the requested
// component or every Terraform component. The maxFindings argument caps how many
// individual findings are shown per component (0 uses sarif's own default); see
// cmd/terraform/lint.go's --max-findings flag.
func Execute(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, affected *AffectedOptions, maxFindings int) error {
	defer perf.Track(nil, "scanners.tflint.Execute")()

	if runtime == nil || runtime.SetupAuth == nil || runtime.DescribeStacks == nil || runtime.ProcessStacks == nil || info == nil {
		return errUtils.ErrNilParam
	}
	if affected != nil {
		if runtime.AffectedComponents == nil {
			return errUtils.ErrNilParam
		}
		return executeAffected(ctx, runtime, info, affected, maxFindings)
	}
	return execute(ctx, runtime, info, maxFindings)
}

func execute(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, maxFindings int) error {
	if info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
	}
	// Lint only needs static HCL/backend config, not resolved remote values — it never reads
	// a component's !terraform.state/!terraform.output-resolved vars, only its .tf files and
	// .tflint.hcl. Leaving YAML functions on (--process-functions defaults to true) makes
	// every cross-component reference attempt a real backend read for every discovered
	// component, unconditionally — including the slow IMDS/STS retry cascade when credentials
	// aren't available, which can take minutes across a large repo even with --error-mode=warn
	// degrading the eventual failure gracefully. Skipping it here avoids paying that cost at
	// all, matching "lint reads static HCL only" literally rather than just recovering from it
	// after the fact. See docs/fixes/2026-07-22-terraform-lint-error-mode.md.
	info.ProcessFunctions = false

	atmosConfig, err := initCLIConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}

	projectToolchainPATH, err := checkTFLintAvailable(&atmosConfig)
	if err != nil {
		return err
	}

	authManager, err := runtime.SetupAuth(&atmosConfig, info)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}

	components := info.Components
	if info.ComponentFromArg != "" {
		components = []string{info.ComponentFromArg}
	}
	// The real Runtime.DescribeStacks implementation (cmd/terraform/lint.go's
	// terraformLintDescribeStacks) reports its own per-stack-file progress; this package
	// stays decoupled from that UI and just calls through.
	stacks, err := runtime.DescribeStacks(
		&atmosConfig, info.Stack, components, []string{cfg.TerraformComponentType}, nil,
		false, info.ProcessTemplates, info.ProcessFunctions, false, info.Skip, authManager, info.AuthDisabled,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	graph, err := buildTerraformGraph(stacks)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrBuildTerraformLintTargets, err)
	}
	targets := targetsFor(graph, nil)
	if len(targets) == 0 {
		ui.Success("No Terraform components matched")
		return nil
	}
	return executeTargets(ctx, &targetExecution{Runtime: runtime, AtmosConfig: &atmosConfig, BaseInfo: info, AuthManager: authManager, ProjectToolchainPATH: projectToolchainPATH, MaxFindings: maxFindings, OutputFormat: runtime.OutputFormat}, targets)
}

func executeAffected(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, options *AffectedOptions, maxFindings int) error {
	if options == nil {
		return errUtils.ErrNilParam
	}

	authDisabled := options.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
	}
	// See the matching comment in execute(): lint never needs YAML-function-resolved
	// values, so skip them unconditionally rather than paying for (and then recovering
	// from) real backend reads — including the slow IMDS/STS retry cascade when
	// credentials aren't available.
	info.ProcessFunctions = false
	options.ProcessYamlFunctions = false

	atmosConfig, err := initCLIConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}

	projectToolchainPATH, err := checkTFLintAvailable(&atmosConfig)
	if err != nil {
		return err
	}

	authManager, err := runtime.SetupAuth(&atmosConfig, info)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}
	options.AuthDisabled = authDisabled

	targets, err := discoverAffectedTargets(runtime, &atmosConfig, options, authManager)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		ui.Success("No components affected")
		return nil
	}

	return executeTargets(ctx, &targetExecution{Runtime: runtime, AtmosConfig: &atmosConfig, BaseInfo: info, AuthManager: authManager, ProjectToolchainPATH: projectToolchainPATH, MaxFindings: maxFindings, OutputFormat: runtime.OutputFormat}, targetsFor(nil, targets))
}

// discoverAffectedTargets runs runtime.AffectedComponents under a spinner, filters the
// result down to non-deleted Terraform components, and converts it into dependency
// nodes for executeTargets. Extracted from executeAffected to keep its own cyclomatic
// complexity under the repo's threshold.
func discoverAffectedTargets(runtime *Runtime, atmosConfig *schema.AtmosConfiguration, options *AffectedOptions, authManager auth.AuthManager) ([]*dependency.Node, error) {
	var affected []schema.Affected
	err := spinner.ExecWithSpinner("Discovering affected Terraform lint targets", "Discovered affected Terraform lint targets", func() error {
		var affectedErr error
		affected, affectedErr = runtime.AffectedComponents(atmosConfig, options, authManager)
		return affectedErr
	})
	if err != nil {
		return nil, fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAffected, err)
	}
	affected = filterAffected(affected)

	targets := make([]*dependency.Node, 0, len(affected))
	for i := range affected {
		item := &affected[i]
		targets = append(targets, &dependency.Node{Component: item.Component, Stack: item.Stack, Type: cfg.TerraformComponentType})
	}
	return targets, nil
}

func filterAffected(input []schema.Affected) []schema.Affected {
	filtered := input[:0]
	for i := range input {
		item := &input[i]
		if item.ComponentType != cfg.TerraformComponentType || item.Deleted {
			continue
		}
		filtered = append(filtered, *item)
	}
	return filtered
}

func targetsFor(graph *dependency.Graph, input []*dependency.Node) []*dependency.Node {
	if graph != nil {
		input = make([]*dependency.Node, 0, len(graph.Nodes))
		for _, node := range graph.Nodes {
			input = append(input, node)
		}
	}

	valid := make([]*dependency.Node, 0, len(input))
	for _, node := range input {
		if node != nil && node.Component != "" && node.Stack != "" {
			valid = append(valid, node)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		if valid[i].Component == valid[j].Component {
			return valid[i].Stack < valid[j].Stack
		}
		return valid[i].Component < valid[j].Component
	})

	seen := make(map[string]struct{}, len(valid))
	targets := make([]*dependency.Node, 0, len(valid))
	for _, node := range valid {
		if _, ok := seen[node.Component]; ok {
			continue
		}
		seen[node.Component] = struct{}{}
		targets = append(targets, node)
	}
	return targets
}

// targetExecution bundles the runtime/config/auth values shared by every
// target in a single lint run, keeping executeTargets/executeTarget under
// the 5-argument limit instead of threading each value through separately.
type targetExecution struct {
	Runtime     *Runtime
	AtmosConfig *schema.AtmosConfiguration
	BaseInfo    *schema.ConfigAndStacksInfo
	AuthManager auth.AuthManager
	// Progress, when set, is updated by executeTarget as it moves through its own
	// sub-steps (component prep vs. the actual TFLint invocation) so the single label
	// executeTargets shows per target reflects what's actually happening, not just "Linting"
	// for what is mostly stack/component-path resolution and file generation.
	Progress *spinner.Spinner
	// ProjectToolchainPATH is the toolchain PATH resolved once upfront from the project's
	// .tool-versions file (see checkTFLintAvailableImpl), combined into each target's own
	// component-resolved PATH by executeTarget via combineToolchainPATH.
	ProjectToolchainPATH string
	// MaxFindings caps how many individual findings are shown per component (0 uses
	// sarif's own default of 10); see cmd/terraform/lint.go's --max-findings flag.
	MaxFindings int
	// OutputFormat selects the terminal-only presentation of TFLint findings.
	OutputFormat string
}

func executeTargets(ctx context.Context, exec *targetExecution, targets []*dependency.Node) error {
	progress := spinner.New(fmt.Sprintf("Processing %d Terraform component(s)", len(targets)))
	progress.Start()
	exec.Progress = progress

	errs := make([]error, 0)
	for i, target := range targets {
		progress.Update(fmt.Sprintf("Preparing `%s` in `%s` (%d/%d)", target.Component, target.Stack, i+1, len(targets)))
		err := runTarget(ctx, exec, target)
		if err != nil {
			errs = append(errs, err)
		}
		// A missing tflint binary fails identically for every remaining target (same
		// toolchain/PATH resolution applies repo-wide in the overwhelmingly common case) —
		// stop immediately instead of repeating the same expensive prep-then-fail cycle for
		// every other target. See docs/fixes/2026-07-22-terraform-lint-fail-fast-and-progress-ux.md.
		if errors.Is(err, errUtils.ErrCommandNotFound) {
			progress.Error(fmt.Sprintf("Stopped after %d/%d — tflint is not available", i+1, len(targets)))
			return err
		}
	}
	if len(errs) > 0 {
		progress.Error(fmt.Sprintf("Failed to lint %d of %d component(s)", len(errs), len(targets)))
		return &lintAggregateError{errs: errs, failed: len(errs), total: len(targets)}
	}
	progress.Success(fmt.Sprintf("Linted %d component(s)", len(targets)))
	return nil
}

// lintAggregateError reports how many targets failed, without repeating every
// individual target's message in its own printed text.
//
// A plain joined error would still let callers match against each individual
// cause, but its own printed text would be every joined message concatenated.
// The top-level CLI error formatter can only walk a single-error chain, not a
// joined multi-error, so it stops at the join and prints the whole
// concatenation verbatim as the message. That produced the wall of text a
// 122-component run used to leave behind.
//
// This type keeps the joined-error shape so callers can still match against
// each individual cause, but its own printed text stays a short, count-based
// summary, so the formatter stops here with something readable instead. Each
// target's own error is already shown in detail during the run, so nothing is
// lost by not repeating it here.
type lintAggregateError struct {
	errs          []error
	failed, total int
}

func (e *lintAggregateError) Error() string {
	defer perf.Track(nil, "tflint.lintAggregateError.Error")()

	return fmt.Sprintf("%d of %d component(s) failed to lint", e.failed, e.total)
}

func (e *lintAggregateError) Unwrap() []error {
	defer perf.Track(nil, "tflint.lintAggregateError.Unwrap")()

	return e.errs
}

func executeTarget(ctx context.Context, exec *targetExecution, target *dependency.Node) error {
	info := *exec.BaseInfo
	info.ComponentType = cfg.TerraformComponentType
	info.ComponentFromArg = target.Component
	info.Component = target.Component
	info.Stack = target.Stack
	info.SubCommand = "lint"

	processed, err := exec.Runtime.ProcessStacks(exec.AtmosConfig, info, true, info.ProcessTemplates, info.ProcessFunctions, info.Skip, exec.AuthManager)
	if err != nil {
		return targetError(target, "resolving the stack configuration", err)
	}
	if _, err = resolveAndProvisionComponentPath(ctx, exec.AtmosConfig, &processed); err != nil {
		return targetError(target, "resolving the component path", err)
	}
	tenv, err := dependencies.ForComponent(exec.AtmosConfig, cfg.TerraformComponentType, processed.StackSection, processed.ComponentSection)
	if err != nil {
		return targetError(target, "resolving the TFLint toolchain", err)
	}
	if exec.Progress != nil {
		exec.Progress.Update(fmt.Sprintf("Linting `%s` in `%s`", target.Component, target.Stack))
	}
	toolchainPATH := combineToolchainPATH(exec.ProjectToolchainPATH, tenv.PATH())
	// OnFailureFail (not the shared scanner default of "warn"): `atmos terraform lint` is a
	// dedicated lint command, so a component with real findings must make the overall command
	// report failure — unlike a `hooks:`-embedded tflint run during plan/apply, where findings
	// are informational and must never block infrastructure changes. executeTargets still
	// processes every remaining target after a findings-based failure (only ErrCommandNotFound
	// stops the loop early), so one component's findings don't hide another's.
	if _, _, err = Run(ctx, &Options{AtmosConfig: exec.AtmosConfig, Info: &processed, ToolchainPATH: toolchainPATH, OnFailure: scanners.OnFailureFail, MaxFindings: exec.MaxFindings, OutputFormat: exec.OutputFormat}); err != nil {
		return targetError(target, "running TFLint", err)
	}
	return nil
}

func resolveAndProvisionComponentPath(ctx context.Context, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, error) {
	componentPath, err := u.GetComponentPath(atmosConfig, cfg.TerraformComponentType, info.ComponentFolderPrefix, info.FinalComponent)
	if err != nil {
		return "", fmt.Errorf("failed to resolve component path: %w", err)
	}

	provisionCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	componentPath, exists, err := component.ProvisionAndResolveComponentPath(provisionCtx, atmosConfig, info, cfg.TerraformComponentType, componentPath)
	if err != nil {
		return "", err
	}
	if err = autoGenerateComponentFiles(atmosConfig, info, componentPath); err != nil {
		return "", err
	}
	if !exists {
		basePath, _ := u.GetComponentBasePath(atmosConfig, cfg.TerraformComponentType)
		return "", fmt.Errorf(
			"%w: '%s' points to the Terraform component '%s', but it does not exist in '%s'",
			errUtils.ErrInvalidTerraformComponent, info.ComponentFromArg, info.FinalComponent, basePath,
		)
	}
	return componentPath, nil
}

func autoGenerateComponentFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string) error {
	if !atmosConfig.Components.Terraform.AutoGenerateFiles || info.DryRun || tfgenerate.GetGenerateSectionFromComponent(info.ComponentSection) == nil {
		return nil
	}
	return tfgenerate.NewService(nil).GenerateFilesForComponent(atmosConfig, info, componentPath)
}

// targetError wraps a per-target failure via the error builder (not plain fmt.Errorf) so
// hints/explanations already attached to err — e.g. resolveBinaryOnPath's "command not
// found" hint to declare the tool in dependencies.tools — survive instead of being reduced
// to a bare .Error() string chain. The component/stack/operation context is folded into the
// wrapped cause's own text (via WithCausef, not just WithContext/WithExplanationf) so it
// also survives a plain .Error() read — e.g. after multiple target errors are combined with
// errors.Join, which does not carry cockroachdb's structured hint/detail metadata through.
// See docs/fixes/2026-07-22-terraform-lint-fail-fast-and-progress-ux.md.
func targetError(target *dependency.Node, operation string, err error) error {
	return errUtils.Build(errUtils.ErrTerraformLint).
		WithCausef("%s for component %q in stack %q: %w", operation, target.Component, target.Stack, err).
		WithContext("component", target.Component).
		WithContext("stack", target.Stack).
		Err()
}
