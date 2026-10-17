package cloudformation

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	pkgcfn "github.com/cloudposse/atmos/pkg/component/aws/cloudformation"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// cfnCreateListAuthManager is a seam for testing. `list` has no component, so it
// resolves its identity the way every other component-less command does: an
// explicit --identity wins, otherwise the stack manifests are scanned for a
// single configured default identity (the aws/cloudformation components in the
// example stacks declare theirs at component level, which a global-only lookup
// never sees).
var cfnCreateListAuthManager = auth.CreateAndAuthenticateManagerWithStackScan

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
in the given stack. Without --stack, stacks are annotated against the components
configured in every Atmos stack.`,
		Example: `  # List all stacks in the account, annotated against every Atmos stack's components
  atmos aws cloudformation list

  # Annotate against the "dev" stack's components only
  atmos aws cloudformation list --stack dev

  # Only stacks currently in a *_COMPLETE status
  atmos aws cloudformation list --stack dev --status CREATE_COMPLETE,UPDATE_COMPLETE`,
		Args: cobra.NoArgs,
		RunE: runList,
	}
	parser.RegisterFlags(cmd)
	if err := parser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	return cmd
}

func runList(cmd *cobra.Command, _ []string) error {
	// Validate --status first: a typo must fail before any auth prompt or AWS call.
	statusFilter, _ := cmd.Flags().GetStringSlice("status")
	if err := pkgcfn.ValidateStackStatusFilter(statusFilter); err != nil {
		return err
	}

	info := buildConfigAndStacksInfo(cmd)
	atmosConfig, err := cfnInitCliConfig(info, true)
	if err != nil {
		return err
	}

	authManager, err := cfnCreateListAuthManager(info.Identity, auth.CopyGlobalAuthConfig(&atmosConfig.Auth), cfg.IdentityFlagSelectValue, &atmosConfig)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrFailedToInitializeAuthManager, err)
	}
	e.PropagateAuth(&info, authManager)

	configuredStackNames, err := configuredCloudFormationStackNames(cmd.Context(), &atmosConfig, info.Stack, authManager)
	if err != nil {
		return err
	}

	region, _ := cmd.Flags().GetString("region")

	stacks, err := pkgcfn.ListDeployedStacks(cmd.Context(), &info, region, statusFilter, configuredStackNames)
	if err != nil {
		return withIdentityHint(err, authManager != nil)
	}
	pkgcfn.RenderDeployedStacksList(stacks)
	return nil
}

// credentialFailureMarkers are substrings of the AWS SDK's credential-resolution failures (for
// example, falling all the way through to the EC2 IMDS provider on a laptop).
var credentialFailureMarkers = []string{
	"failed to retrieve credentials",
	"failed to refresh cached credentials",
	"no EC2 IMDS role found",
	"NoCredentialProviders",
}

// withIdentityHint adds a "pass --identity" hint to a credential-resolution failure when no
// identity was resolved, since the SDK's own message (an EC2 IMDS lookup error) doesn't tell
// the user what to do. Any other error, or any failure with an identity in use, passes through.
func withIdentityHint(err error, identityResolved bool) error {
	if identityResolved || !isCredentialFailure(err) {
		return err
	}
	return errUtils.Build(err).
		WithHint("No identity could be resolved, so the AWS SDK's default credential chain was used. Pass --identity=<name>, or declare exactly one `default: true` identity in your auth configuration.").
		Err()
}

// isCredentialFailure reports whether err is an AWS SDK credential-resolution failure.
func isCredentialFailure(err error) bool {
	message := err.Error()
	for _, marker := range credentialFailureMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// configuredCloudFormationStackNames returns the set of stack_name values
// configured on aws/cloudformation components, used to annotate `list`'s output
// as "managed" vs. "unmanaged". With a non-empty stack the set covers that Atmos
// stack only (and a stack that does not exist is an ErrStackNotFound error);
// with an empty stack it covers every Atmos stack. Templates are processed (so a
// component's stack_name may reference {{ .vars.stage }}) but YAML functions are
// not — they aren't needed to resolve stack_name and some require their own
// authentication, which would slow this down unnecessarily.
func configuredCloudFormationStackNames(ctx context.Context, atmosConfig *schema.AtmosConfiguration, stack string, authManager auth.AuthManager) (map[string]bool, error) {
	stacksMap, err := cfnDescribeStacks(atmosConfig, stack, nil, []string{cfg.CloudFormationComponentType}, nil, false, true, false, false, nil, authManager)
	if err != nil {
		return nil, err
	}
	if stack != "" {
		if _, ok := stacksMap[stack]; !ok {
			return nil, stackNotFoundError(stack)
		}
	}

	names := make(map[string]bool)
	for stackName, stackSection := range stacksMap {
		if stack != "" && stackName != stack {
			continue
		}
		// Each stack is listed on its own so the provider's own component filtering
		// (abstract and disabled components are excluded) applies per stack.
		componentNames, err := cfnListAllComponents(ctx, cfg.CloudFormationComponentType, map[string]any{stackName: stackSection})
		if err != nil {
			return nil, err
		}
		for _, componentName := range componentNames {
			if name, ok := cloudFormationComponentStackName(stacksMap, stackName, componentName); ok {
				names[name] = true
			}
		}
	}
	return names, nil
}

// stackNotFoundError reports an unknown --stack value.
func stackNotFoundError(stack string) error {
	return errUtils.Build(errUtils.ErrStackNotFound).
		WithExplanationf("Stack `%s` not found", stack).
		WithHint("Run `atmos list stacks` to see all available stacks.").
		WithContext("stack", stack).
		WithExitCode(2).
		Err()
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
