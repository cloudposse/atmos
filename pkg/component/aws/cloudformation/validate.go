package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"

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
// stack via SetStackPolicy — CreateChangeSet/ExecuteChangeSet have no stack-policy
// parameter, so this runs as a follow-up call after a successful apply.
func setStackPolicy(ctx context.Context, client CloudFormationClient, spec *stackSpec) error {
	defer perf.Track(nil, "cloudformation.setStackPolicy")()

	_, err := client.SetStackPolicy(ctx, &cloudformation.SetStackPolicyInput{
		StackName:       awsString(spec.StackName),
		StackPolicyBody: awsString(spec.StackPolicyBody),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationChangeSetFailed, err)
	}
	return nil
}

// validateTemplate calls the server-side ValidateTemplate API (syntax +
// capability discovery) — an API-backed check, not a local linter. Local
// linting (cfn-lint/cfn-guard) is not built into this component type; users
// who want it declare those tools via the toolchain subsystem.
func validateTemplate(ctx context.Context, client CloudFormationClient, templateBody string) error {
	defer perf.Track(nil, "cloudformation.validateTemplate")()

	_, err := client.ValidateTemplate(ctx, &cloudformation.ValidateTemplateInput{
		TemplateBody: awsString(templateBody),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, err)
	}
	return nil
}
