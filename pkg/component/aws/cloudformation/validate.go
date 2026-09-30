package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// validateComponentConfig validates aws/cloudformation component configuration.
// Template is required unless the component is abstract; stack_name is always
// required for non-abstract components.
func validateComponentConfig(config map[string]any) error {
	defer perf.Track(nil, "cloudformation.validateComponentConfig")()

	if config == nil {
		return nil
	}
	if isAbstractComponent(config) {
		return nil
	}

	template, _ := config[cfg.TemplateSectionName].(string)
	if template == "" {
		return errUtils.ErrMissingAwsCloudFormationTemplate
	}

	stackName, _ := config[cfg.StackNameSectionName].(string)
	if stackName == "" {
		return errUtils.ErrMissingAwsCloudFormationStackName
	}

	return nil
}

// setStackPolicy applies the component's stack_policy document to the deployed
// stack via SetStackPolicy. Existing stacks receive it before execution so it
// protects the current update; new stacks receive it after successful creation.
func setStackPolicy(ctx context.Context, client CloudFormationClient, spec *stackSpec) error {
	defer perf.Track(nil, "cloudformation.setStackPolicy")()

	_, err := client.SetStackPolicy(ctx, &cloudformation.SetStackPolicyInput{
		StackName:       awsString(spec.StackName),
		StackPolicyBody: awsString(spec.StackPolicyBody),
	})
	if err != nil {
		return wrapAPICallError(spec.StackName, err)
	}
	return nil
}

// prepareStackPolicy protects existing resources before a changeset executes.
// Its result says whether policy installation must wait for successful creation.
func prepareStackPolicy(ctx context.Context, client CloudFormationClient, spec *stackSpec, result *changeSetResult) (bool, error) {
	if spec.StackPolicyBody == "" {
		return false, nil
	}
	if result.ChangeSetType == cfntypes.ChangeSetTypeCreate {
		return true, nil
	}
	return false, setStackPolicy(ctx, client, spec)
}

// applyTerminationProtection reconciles the stack's actual termination-protection
// state with spec.TerminationProtection. CreateChangeSet/ExecuteChangeSet have no
// termination-protection parameter, so this runs as a follow-up UpdateTerminationProtection
// call after every successful apply — the same shape setStackPolicy uses for stack
// policy. Called unconditionally (not just when true) so config stays the source of
// truth: removing termination_protection from a component actually disables it on the
// next apply, instead of only stopping enforcement by Atmos's own delete command.
func applyTerminationProtection(ctx context.Context, client CloudFormationClient, spec *stackSpec) error {
	defer perf.Track(nil, "cloudformation.applyTerminationProtection")()

	_, err := client.UpdateTerminationProtection(ctx, &cloudformation.UpdateTerminationProtectionInput{
		StackName:                   awsString(spec.StackName),
		EnableTerminationProtection: awsBool(spec.TerminationProtection),
	})
	if err != nil {
		return wrapAPICallError(spec.StackName, err)
	}
	return nil
}

// runValidate prepares large templates before server-side validation.
func runValidate(octx *opContext, client CloudFormationClient, spec *stackSpec, summary map[string]any) (map[string]any, error) {
	if err := prepareTemplateForAPI(octx, spec, summary); err != nil {
		return summary, err
	}
	return summary, validateTemplate(octx.Ctx, client, spec)
}

// validateTemplate calls the server-side ValidateTemplate API (syntax +
// capability discovery) — an API-backed check, not a local linter. Local
// linting (cfn-lint/cfn-guard) is not built into this component type; users
// who want it declare those tools via the toolchain subsystem.
func validateTemplate(ctx context.Context, client CloudFormationClient, spec *stackSpec) error {
	defer perf.Track(nil, "cloudformation.validateTemplate")()

	input := &cloudformation.ValidateTemplateInput{}
	if spec.TemplateURL != "" {
		input.TemplateURL = awsString(spec.TemplateURL)
	} else {
		input.TemplateBody = awsString(spec.TemplateBody)
	}
	_, err := client.ValidateTemplate(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, err)
	}
	return nil
}
