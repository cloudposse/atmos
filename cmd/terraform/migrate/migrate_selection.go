package migrate

import (
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func executeAffectedMigrateCommand(cmd *cobra.Command, args []string, info *schema.ConfigAndStacksInfo, opts tfmigrate.Options) error {
	flags := cmd.PersistentFlags()
	if flags.Lookup("file") == nil {
		flags.String("file", "", "")
	}
	if flags.Lookup("format") == nil {
		flags.String("format", "yaml", "")
	}
	if flags.Lookup("verbose") == nil {
		flags.Bool("verbose", false, "")
	}
	if flags.Lookup("include-spacelift-admin-stacks") == nil {
		flags.Bool("include-spacelift-admin-stacks", false, "")
	}
	if flags.Lookup("include-settings") == nil {
		flags.Bool("include-settings", false, "")
	}
	if flags.Lookup("upload") == nil {
		flags.Bool("upload", false, "")
	}
	if err := flags.Set("format", "yaml"); err != nil {
		return err
	}

	a, err := e.ParseDescribeAffectedCliArgs(cmd, args)
	if err != nil {
		return err
	}

	a.IncludeSpaceliftAdminStacks = false
	a.IncludeSettings = false
	a.Upload = false
	a.OutputFile = ""

	return executeTfmigrateAffected(&a, info, opts)
}

func executeTfmigrateQuery(info *schema.ConfigAndStacksInfo, opts tfmigrate.Options) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}
	authManager, err := e.SetupComponentAuthForCLI(&atmosConfig, info)
	if err != nil {
		return err
	}
	stacks, err := e.ExecuteDescribeStacksWithAuthDisabled(
		&atmosConfig,
		info.Stack,
		info.Components,
		[]string{cfg.TerraformComponentType},
		nil,
		false,
		info.ProcessTemplates,
		info.ProcessFunctions,
		false,
		info.Skip,
		authManager,
		info.Identity == cfg.IdentityFlagDisabledValue,
	)
	if err != nil {
		return err
	}
	return walkTfmigrateComponents(stacks, func(stackName, componentName string, componentSection map[string]any) error {
		if shouldSkipTfmigrateComponent(componentSection, info.Query) {
			return nil
		}
		next := *info
		next.Component = componentName
		next.ComponentFromArg = componentName
		next.Stack = stackName
		next.StackFromArg = stackName
		return executeTfmigrateSingle(&next, opts)
	})
}

func executeTfmigrateAffected(args *e.DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo, opts tfmigrate.Options) error {
	if args.IncludeDependents {
		return errUtils.Build(errUtils.ErrInvalidConfig).
			WithExplanation("tfmigrate --affected does not support --include-dependents yet").
			WithHint("Use --all, --components, or --query when dependents must be included").
			Err()
	}

	affected, err := describeAffectedForTfmigrate(args)
	if err != nil {
		return err
	}
	for i := range affected {
		item := &affected[i]
		if item.ComponentType != cfg.TerraformComponentType || item.Deleted {
			continue
		}
		next := *info
		next.Component = item.Component
		next.ComponentFromArg = item.Component
		next.Stack = item.Stack
		next.StackFromArg = item.Stack
		if err := executeTfmigrateSingle(&next, opts); err != nil {
			return err
		}
	}
	return nil
}

func describeAffectedForTfmigrate(args *e.DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	switch {
	case args.RepoPath != "":
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			args.CLIConfig,
			args.RepoPath,
			args.IncludeSpaceliftAdminStacks,
			args.IncludeSettings,
			args.Stack,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			args.Skip,
			args.ExcludeLocked,
			args.AuthManager,
			args.AuthDisabled,
		)
		return affected, err
	case args.CloneTargetRef:
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefClone(
			args.CLIConfig,
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
			args.AuthManager,
			args.AuthDisabled,
		)
		return affected, err
	default:
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRefCheckout(
			args.CLIConfig,
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
			args.AuthManager,
			args.AuthDisabled,
		)
		return affected, err
	}
}

func shouldSkipTfmigrateComponent(componentSection map[string]any, query string) bool {
	metadata, _ := componentSection[cfg.MetadataSectionName].(map[string]any)
	if metadataType, ok := metadata["type"].(string); ok && metadataType == "abstract" {
		return true
	}
	if enabled, ok := metadata["enabled"].(bool); ok && !enabled {
		return true
	}
	if query == "" {
		return false
	}
	queryResult, err := u.EvaluateYqExpression(&schema.AtmosConfiguration{}, componentSection, query)
	if err != nil {
		return true
	}
	queryPassed, ok := queryResult.(bool)
	return !ok || !queryPassed
}

func walkTfmigrateComponents(stacks map[string]any, fn func(stackName, componentName string, componentSection map[string]any) error) error {
	for stackName, stackData := range stacks {
		stackSection, ok := stackData.(map[string]any)
		if !ok {
			continue
		}
		componentsSection, ok := stackSection["components"].(map[string]any)
		if !ok {
			continue
		}
		terraformSection, ok := componentsSection[cfg.TerraformSectionName].(map[string]any)
		if !ok {
			continue
		}
		for componentName, compSection := range terraformSection {
			componentSection, ok := compSection.(map[string]any)
			if !ok {
				continue
			}
			if err := fn(stackName, componentName, componentSection); err != nil {
				return err
			}
		}
	}
	return nil
}
