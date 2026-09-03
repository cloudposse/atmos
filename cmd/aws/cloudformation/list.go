package cloudformation

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	pkgcfn "github.com/cloudposse/atmos/pkg/component/aws/cloudformation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// newListCmd is the `atmos aws cloudformation list` command: an account-wide
// ListStacks, annotated against the queried stack's configured
// aws/cloudformation components by stack_name. Unlike every other verb, this
// is not scoped to one component (there is no `[component]` argument), so it
// doesn't go through newOperationCommand/ComponentProvider.Execute — it calls
// pkg/component/aws/cloudformation.ListDeployedStacks directly, the same way
// cmd/aws/cloudformation/source's verbs bypass Execute for their own
// non-component-scoped inspection commands.
func newListCmd() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithStringSliceFlag("status", "", nil, "Filter by stack status (comma-separated, e.g. CREATE_COMPLETE,UPDATE_COMPLETE)."),
		flags.WithStringFlag("region", "", "", "AWS region to list stacks in. Defaults to the active identity's region, then the SDK's standard resolution chain."),
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List deployed CloudFormation stacks in the account",
		Long: `List the account's deployed CloudFormation stacks (ListStacks), annotated with
whether each one matches an aws/cloudformation component's stack_name configured
in the given stack.`,
		Example: `  # List all stacks in the account, annotated against the "dev" stack's components
  atmos aws cloudformation list --stack dev

  # Only stacks currently in a *_COMPLETE status
  atmos aws cloudformation list --stack dev --status CREATE_COMPLETE,UPDATE_COMPLETE`,
		RunE: runList,
	}
	parser.RegisterFlags(cmd)
	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := cfnInitCliConfig(info, true)
	if err != nil {
		return err
	}

	authManager, err := e.SetupComponentAuthForCLI(&atmosConfig, &info)
	if err != nil {
		return err
	}
	e.PropagateAuth(&info, authManager)

	configuredStackNames, err := configuredCloudFormationStackNames(&atmosConfig, info.Stack, authManager)
	if err != nil {
		return err
	}

	statusFilter, _ := cmd.Flags().GetStringSlice("status")
	region, _ := cmd.Flags().GetString("region")

	stacks, err := pkgcfn.ListDeployedStacks(context.Background(), &info, region, statusFilter, configuredStackNames)
	if err != nil {
		return err
	}
	pkgcfn.RenderDeployedStacksList(stacks)
	return nil
}

// configuredCloudFormationStackNames returns the set of stack_name values
// configured on aws/cloudformation components in the given Atmos stack, used
// to annotate `list`'s output as "managed" vs. "unmanaged". Templates are
// processed (so a component's stack_name may reference {{ .vars.stage }}) but
// YAML functions are not — they aren't needed to resolve stack_name and some
// require their own authentication, which would slow this down unnecessarily.
func configuredCloudFormationStackNames(atmosConfig *schema.AtmosConfiguration, stack string, authManager auth.AuthManager) (map[string]bool, error) {
	stacksMap, err := cfnDescribeStacks(atmosConfig, stack, nil, []string{cfg.CloudFormationComponentType}, nil, false, true, false, false, nil, authManager)
	if err != nil {
		return nil, err
	}
	componentNames, err := cfnListAllComponents(context.Background(), cfg.CloudFormationComponentType, stacksMap)
	if err != nil {
		return nil, err
	}

	names := make(map[string]bool, len(componentNames))
	for _, componentName := range componentNames {
		if name, ok := cloudFormationComponentStackName(stacksMap, stack, componentName); ok {
			names[name] = true
		}
	}
	return names, nil
}

// cloudFormationComponentStackName reads components.aws/cloudformation.<component>.stack_name
// out of a stacksMap built by ExecuteDescribeStacks.
func cloudFormationComponentStackName(stacksMap map[string]any, stack, componentName string) (string, bool) {
	stackSection, ok := stacksMap[stack].(map[string]any)
	if !ok {
		return "", false
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return "", false
	}
	typeSection, ok := componentsSection[cfg.CloudFormationComponentType].(map[string]any)
	if !ok {
		return "", false
	}
	componentSection, ok := typeSection[componentName].(map[string]any)
	if !ok {
		return "", false
	}
	name, ok := componentSection[cfg.StackNameSectionName].(string)
	return name, ok && name != ""
}
