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
		false, info.ProcessTemplates, info.ProcessFunctions, false, info.Skip, authManager,
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
	return executeTargets(ctx, runtime, &atmosConfig, info, targets, authManager)
}

func executeAffected(ctx context.Context, runtime *Runtime, info *schema.ConfigAndStacksInfo, options *AffectedOptions) error {
	if options == nil {
		return errUtils.ErrNilParam
	}

	authDisabled := options.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
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
	return executeTargets(ctx, runtime, &atmosConfig, info, targetsFor(nil, targets), authManager)
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

func executeTargets(ctx context.Context, runtime *Runtime, atmosConfig *schema.AtmosConfiguration, baseInfo *schema.ConfigAndStacksInfo, targets []*dependency.Node, authManager auth.AuthManager) error {
	errs := make([]error, 0)
	for _, target := range targets {
		if err := runTarget(ctx, runtime, atmosConfig, baseInfo, target, authManager); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func executeTarget(ctx context.Context, runtime *Runtime, atmosConfig *schema.AtmosConfiguration, baseInfo *schema.ConfigAndStacksInfo, target *dependency.Node, authManager auth.AuthManager) error {
	info := *baseInfo
	info.ComponentType = cfg.TerraformComponentType
	info.ComponentFromArg = target.Component
	info.Component = target.Component
	info.Stack = target.Stack
	info.SubCommand = "lint"

	processed, err := runtime.ProcessStacks(atmosConfig, info, true, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
	if err != nil {
		return targetError(target, "resolving the stack configuration", err)
	}
	if _, err = resolveAndProvisionComponentPath(ctx, atmosConfig, &processed); err != nil {
		return targetError(target, "resolving the component path", err)
	}
	tenv, err := dependencies.ForComponent(atmosConfig, cfg.TerraformComponentType, processed.StackSection, processed.ComponentSection)
	if err != nil {
		return targetError(target, "resolving the TFLint toolchain", err)
	}
	if _, _, err = Run(ctx, &Options{AtmosConfig: atmosConfig, Info: &processed, ToolchainPATH: tenv.PATH()}); err != nil {
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
