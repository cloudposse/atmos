package tflint

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	tfgenerate "github.com/cloudposse/atmos/pkg/terraform/generate"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const terraformLintWrappedErrorFormat = "%w: %w"

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
}

var (
	initCLIConfig       = cfg.InitCliConfig
	buildTerraformGraph = scheduleradapters.BuildTerraformGraph
	runTarget           = executeTarget
)

// Execute runs the Terraform-aware TFLint command. When affected is non-nil,
// it selects changed Terraform components; otherwise it selects the requested
// component or every Terraform component.
func Execute(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, affected *AffectedOptions) error {
	defer perf.Track(nil, "scanners.tflint.Execute")()

	if runtime == nil || runtime.SetupAuth == nil || runtime.DescribeStacks == nil || runtime.ProcessStacks == nil || info == nil {
		return errUtils.ErrNilParam
	}
	if affected != nil {
		if runtime.AffectedComponents == nil {
			return errUtils.ErrNilParam
		}
		return executeAffected(ctx, runtime, info, affected)
	}
	return execute(ctx, runtime, info)
}

func execute(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo) error {
	if info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
		// Lint only needs static HCL/backend config, not resolved remote values.
		// With auth disabled there's no AuthManager to reach a real backend with,
		// so leaving YAML functions on lets !terraform.state/!terraform.output
		// fall through to an unauthenticated AWS call instead of being skipped —
		// and their `//` fallback only covers "not yet provisioned" errors, not
		// credential failures. See docs/fixes/2026-07-21-terraform-lint-auth-disabled-yaml-functions.md.
		info.ProcessFunctions = false
	}

	atmosConfig, err := initCLIConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}

	authManager, err := runtime.SetupAuth(&atmosConfig, info)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}

	components := info.Components
	if info.ComponentFromArg != "" {
		components = []string{info.ComponentFromArg}
	}
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
	return executeTargets(ctx, &targetExecution{Runtime: runtime, AtmosConfig: &atmosConfig, BaseInfo: info, AuthManager: authManager}, targets)
}

func executeAffected(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, options *AffectedOptions) error {
	if options == nil {
		return errUtils.ErrNilParam
	}

	authDisabled := options.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
		// See the matching comment in execute(): with auth disabled, YAML
		// functions that read remote state must be skipped rather than left
		// to hit AWS unauthenticated.
		info.ProcessFunctions = false
		options.ProcessYamlFunctions = false
	}

	atmosConfig, err := initCLIConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}
	authManager, err := runtime.SetupAuth(&atmosConfig, info)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}
	options.AuthDisabled = authDisabled

	affected, err := runtime.AffectedComponents(&atmosConfig, options, authManager)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAffected, err)
	}
	affected = filterAffected(affected)
	if len(affected) == 0 {
		ui.Success("No components affected")
		return nil
	}

	targets := make([]*dependency.Node, 0, len(affected))
	for i := range affected {
		item := &affected[i]
		targets = append(targets, &dependency.Node{Component: item.Component, Stack: item.Stack, Type: cfg.TerraformComponentType})
	}
	return executeTargets(ctx, &targetExecution{Runtime: runtime, AtmosConfig: &atmosConfig, BaseInfo: info, AuthManager: authManager}, targetsFor(nil, targets))
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
}

func executeTargets(ctx context.Context, exec *targetExecution, targets []*dependency.Node) error {
	errs := make([]error, 0)
	for _, target := range targets {
		if err := runTarget(ctx, exec, target); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
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
	if _, _, err = Run(ctx, &Options{AtmosConfig: exec.AtmosConfig, Info: &processed, ToolchainPATH: tenv.PATH()}); err != nil {
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

func targetError(target *dependency.Node, operation string, err error) error {
	return fmt.Errorf("%w: %s for component %q in stack %q: %w", errUtils.ErrTerraformLint, operation, target.Component, target.Stack, err)
}
