package kubernetes

import (
	"context"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func executeBulk(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	operation Operation,
) error {
	authManager, err := authManagerForBulk(atmosConfig, info)
	if err != nil {
		return err
	}

	stacks, err := executeDescribeStacks(
		atmosConfig,
		info.Stack,
		nil,
		[]string{cfg.KubernetesComponentType},
		nil,
		false,
		true,
		true,
		true,
		info.Skip,
		authManager,
	)
	if err != nil {
		return err
	}

	selection, err := graphSelectionForBulk(ctx, atmosConfig, info)
	if err != nil {
		return err
	}

	return executeGraph(context.Background(), &component.GraphExecutionOptions{
		Provider:      &ComponentProvider{},
		AtmosConfig:   atmosConfig,
		Info:          info,
		Stacks:        stacks,
		ComponentType: cfg.KubernetesComponentType,
		SubCommand:    string(operation),
		Flags:         ctx.Flags,
		Selection:     selection,
	})
}

func authManagerForBulk(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	if info.Identity == "" {
		return nil, nil
	}
	authConfig := auth.CopyGlobalAuthConfig(&atmosConfig.Auth)
	authManager, err := auth.CreateAndAuthenticateManagerWithAtmosConfig(info.Identity, authConfig, cfg.IdentityFlagSelectValue, atmosConfig)
	if err != nil {
		return nil, err
	}
	info.AuthManager = authManager
	return authManager, nil
}

func graphSelectionForBulk(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
) (*component.GraphSelection, error) {
	if !info.Affected {
		return nil, nil
	}

	affected, err := affectedKubernetesComponentsFunc(ctx, atmosConfig, info)
	if err != nil {
		return nil, err
	}

	nodeIDs := make([]string, 0, len(affected))
	for i := range affected {
		item := &affected[i]
		if item.Deleted || item.ComponentType != cfg.KubernetesComponentType {
			continue
		}
		nodeIDs = append(nodeIDs, component.GraphNodeID(item.Component, item.Stack))
	}

	includeDependents, _ := ctx.Flags["include-dependents"].(bool)
	return &component.GraphSelection{
		NodeIDs:             nodeIDs,
		IncludeDependencies: true,
		IncludeDependents:   includeDependents,
	}, nil
}

func affectedKubernetesComponents(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
) ([]schema.Affected, error) {
	args := e.DescribeAffectedCmdArgs{
		CLIConfig:                   atmosConfig,
		Stack:                       info.Stack,
		ProcessTemplates:            true,
		ProcessYamlFunctions:        true,
		Skip:                        info.Skip,
		IncludeSettings:             false,
		IncludeDependents:           false,
		IncludeSpaceliftAdminStacks: false,
	}

	applyAffectedFlags(&args, ctx.Flags)

	var authManager auth.AuthManager
	if manager, ok := info.AuthManager.(auth.AuthManager); ok {
		authManager = manager
	}
	args.AuthManager = authManager
	args.AuthDisabled = info.AuthDisabled

	return dispatchAffected(atmosConfig, &args, authManager)
}

// applyAffectedFlags maps command flags onto the describe-affected arguments.
func applyAffectedFlags(args *e.DescribeAffectedCmdArgs, flags map[string]any) {
	if value, ok := flags["repo-path"].(string); ok {
		args.RepoPath = value
	}
	applyAffectedBaseFlag(args, flags)
	if value, ok := flags["ref"].(string); ok && value != "" {
		args.Ref = value
	}
	if value, ok := flags["sha"].(string); ok && value != "" {
		args.SHA = value
	}
	if value, ok := flags["ssh-key"].(string); ok {
		args.SSHKeyPath = value
	}
	if value, ok := flags["ssh-key-password"].(string); ok {
		args.SSHKeyPassword = value
	}
	if value, ok := flags["clone-target-ref"].(bool); ok {
		args.CloneTargetRef = value
	}
}

// applyAffectedBaseFlag routes the --base flag to either the SHA or Ref argument.
func applyAffectedBaseFlag(args *e.DescribeAffectedCmdArgs, flags map[string]any) {
	value, ok := flags["base"].(string)
	if !ok {
		return
	}
	if ci.IsCommitSHA(value) {
		args.SHA = value
		return
	}
	args.Ref = value
}

// dispatchAffected runs the appropriate describe-affected strategy based on the resolved args.
func dispatchAffected(
	atmosConfig *schema.AtmosConfiguration,
	args *e.DescribeAffectedCmdArgs,
	authManager auth.AuthManager,
) ([]schema.Affected, error) {
	switch {
	case args.RepoPath != "":
		affected, _, _, _, err := executeAffectedWithRepoPath(
			atmosConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	case args.CloneTargetRef:
		affected, _, _, _, err := executeAffectedWithRefClone(
			atmosConfig,
			args.Ref,
			args.SHA,
			args.SSHKeyPath,
			args.SSHKeyPassword,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	default:
		affected, _, _, _, err := executeAffectedWithRefCheckout(
			atmosConfig,
			args.Ref,
			args.SHA,
			args.TargetBranch,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			authManager,
			args.AuthDisabled,
		)
		return affected, err
	}
}
