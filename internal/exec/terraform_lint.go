package exec

import (
	"context"
	"errors"
	"fmt"
	"sort"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/scanners/tflint"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const terraformLintWrappedErrorFormat = "%w: %w"

var runTerraformLintTarget = executeTerraformLintTarget

// ExecuteTerraformLint lints the selected Terraform components. When no
// component is provided it selects every Terraform component once, even when a
// component is used by more than one stack.
func ExecuteTerraformLint(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformLint")()
	if info == nil {
		return errUtils.ErrNilParam
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}

	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}
	if authManager != nil {
		injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}

	components := info.Components
	if info.ComponentFromArg != "" {
		components = []string{info.ComponentFromArg}
	}
	stacks, err := ExecuteDescribeStacks(
		&atmosConfig, info.Stack, components, []string{cfg.TerraformComponentType}, nil,
		false, info.ProcessTemplates, info.ProcessFunctions, false, info.Skip, authManager,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrExecuteDescribeStacks, err)
	}

	graph, err := scheduleradapters.BuildTerraformGraph(stacks)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrBuildTerraformLintTargets, err)
	}
	targets := lintTargets(graph, nil)
	if len(targets) == 0 {
		ui.Success("No Terraform components matched")
		return nil
	}
	return executeTerraformLintTargets(&atmosConfig, info, targets, authManager)
}

// ExecuteTerraformLintAffected lints changed Terraform components. A component
// used in several stacks is still linted once because TFLint validates its
// source directory rather than a Terraform stack instance.
func ExecuteTerraformLintAffected(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformLintAffected")()
	if args == nil || info == nil {
		return errUtils.ErrNilParam
	}

	atmosConfig, authManager, err := prepareTerraformLintAffectedAuth(args, info)
	if err != nil {
		return err
	}

	affected, err := getAffectedComponents(args)
	if err != nil {
		return fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAffected, err)
	}
	affected = filterTerraformAffected(affected)
	if len(affected) == 0 {
		ui.Success("No components affected")
		return nil
	}

	targets := make([]*dependency.Node, 0, len(affected))
	for i := range affected {
		item := &affected[i]
		targets = append(targets, &dependency.Node{Component: item.Component, Stack: item.Stack, Type: cfg.TerraformComponentType})
	}
	targets = lintTargets(nil, targets)
	return executeTerraformLintTargets(atmosConfig, info, targets, authManager)
}

// prepareTerraformLintAffectedAuth resolves the CLI config and auth manager
// for an affected-components lint run, honoring an auth-disabled request from
// either the affected-args or the execution info, and wires the resolved
// auth manager back onto args and info so downstream calls observe the same
// state.
func prepareTerraformLintAffectedAuth(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) (*schema.AtmosConfiguration, auth.AuthManager, error) {
	authDisabled := args.AuthDisabled || info.AuthDisabled || info.Identity == cfg.IdentityFlagDisabledValue
	if authDisabled {
		info.Identity = cfg.IdentityFlagDisabledValue
		info.AuthDisabled = true
	}

	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", errUtils.ErrInitializeCLIConfig, err)
	}
	authManager, err := createQueryAuthManager(info, &atmosConfig)
	if err != nil {
		return nil, nil, fmt.Errorf(terraformLintWrappedErrorFormat, errUtils.ErrTerraformLintAuth, err)
	}
	if authManager != nil {
		injectTerraformStoreAuthResolver(&atmosConfig, info, authManager)
	}
	args.CLIConfig = &atmosConfig
	args.AuthManager = authManager
	args.AuthDisabled = authDisabled

	return &atmosConfig, authManager, nil
}

// lintTargets sorts targets for reproducible output and keeps the first stack
// for each component. A stack qualifier intentionally narrows the input before
// this function runs, so a selected component+stack remains exact.
func lintTargets(graph *dependency.Graph, input []*dependency.Node) []*dependency.Node {
	if graph != nil {
		input = make([]*dependency.Node, 0, len(graph.Nodes))
		for _, node := range graph.Nodes {
			input = append(input, node)
		}
	}
	sort.Slice(input, func(i, j int) bool {
		if input[i].Component == input[j].Component {
			return input[i].Stack < input[j].Stack
		}
		return input[i].Component < input[j].Component
	})

	seen := make(map[string]struct{}, len(input))
	targets := make([]*dependency.Node, 0, len(input))
	for _, node := range input {
		if node == nil || node.Component == "" || node.Stack == "" {
			continue
		}
		if _, ok := seen[node.Component]; ok {
			continue
		}
		seen[node.Component] = struct{}{}
		targets = append(targets, node)
	}
	return targets
}

func executeTerraformLintTargets(atmosConfig *schema.AtmosConfiguration, baseInfo *schema.ConfigAndStacksInfo, targets []*dependency.Node, authManager auth.AuthManager) error {
	errs := make([]error, 0)
	for _, target := range targets {
		if err := runTerraformLintTarget(atmosConfig, baseInfo, target, authManager); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func executeTerraformLintTarget(atmosConfig *schema.AtmosConfiguration, baseInfo *schema.ConfigAndStacksInfo, target *dependency.Node, authManager auth.AuthManager) error {
	info := *baseInfo
	info.ComponentType = cfg.TerraformComponentType
	info.ComponentFromArg = target.Component
	info.Component = target.Component
	info.Stack = target.Stack
	info.SubCommand = "lint"

	processed, err := ProcessStacks(atmosConfig, info, true, info.ProcessTemplates, info.ProcessFunctions, info.Skip, authManager)
	if err != nil {
		return terraformLintTargetError(target, "resolving the stack configuration", err)
	}
	if _, err = resolveAndProvisionComponentPath(atmosConfig, &processed); err != nil {
		return terraformLintTargetError(target, "resolving the component path", err)
	}
	tenv, err := resolveAndInstallToolchainDeps(atmosConfig, &processed)
	if err != nil {
		return terraformLintTargetError(target, "resolving the TFLint toolchain", err)
	}
	if _, _, err = tflint.Run(context.Background(), &tflint.Options{AtmosConfig: atmosConfig, Info: &processed, ToolchainPATH: tenv.PATH()}); err != nil {
		return terraformLintTargetError(target, "running TFLint", err)
	}
	return nil
}

func terraformLintTargetError(target *dependency.Node, operation string, err error) error {
	return fmt.Errorf("%w: %s for component %q in stack %q: %w", errUtils.ErrTerraformLint, operation, target.Component, target.Stack, err)
}
