package cloudformation

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// validateComponentConfig validates aws/cloudformation component configuration.
// A template is required unless the component is abstract, provided either as
// an inline `template` body (string or map) or a `path` file reference — the
// two are mutually exclusive. StackName is always required for non-abstract
// components. File-existence checks for `path` are intentionally left to
// loadTemplateBody's later disk read: this function is called through the
// shared component.ComponentProvider.ValidateComponent(config) interface,
// which receives only the raw component config, not the resolved component
// base path needed to check a relative path on disk.
func validateComponentConfig(config map[string]any) error {
	defer perf.Track(nil, "cloudformation.validateComponentConfig")()

	if config == nil {
		return nil
	}
	if isAbstractComponent(config) {
		return nil
	}

	stackName, _ := config[cfg.StackNameSectionName].(string)
	templateRaw := config[cfg.TemplateSectionName]
	path, _ := config[cfg.TemplatePathSectionName].(string)
	templatePresent := isTemplatePresent(templateRaw)

	switch {
	case templatePresent && path != "":
		return fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationTemplateAndPathMutuallyExclusive, stackName)
	case !templatePresent && path == "":
		return errUtils.ErrMissingAwsCloudFormationTemplate
	}

	if templatePresent {
		if err := sanityCheckInlineTemplate(templateRaw); err != nil {
			return fmt.Errorf("stack %q: %w", stackName, err)
		}
	}

	if stackName == "" {
		return errUtils.ErrMissingAwsCloudFormationStackName
	}

	return nil
}

// isTemplatePresent reports whether the raw `template` value is a non-empty
// inline body — either a non-empty string or a non-empty map.
func isTemplatePresent(raw any) bool {
	switch v := raw.(type) {
	case string:
		return v != ""
	case map[string]any:
		return len(v) > 0
	default:
		return false
	}
}

// sanityCheckInlineTemplate performs a cheap local check that an inline
// `template:` value looks like a CloudFormation template, before ever
// reaching AWS's ValidateTemplate API: it must parse as YAML/JSON and
// contain a top-level Resources section (CFN's one truly-required section).
// String values are parsed with gopkg.in/yaml.v3 so a syntax mistake
// surfaces the parser's own line:column (relative to the block scalar) in
// its error text; map values are already structured, so no parse is needed.
func sanityCheckInlineTemplate(raw any) error {
	var doc map[string]any
	switch v := raw.(type) {
	case string:
		if err := yaml.Unmarshal([]byte(v), &doc); err != nil {
			return fmt.Errorf("%w: %w", errUtils.ErrInvalidAwsCloudFormationSettings, err)
		}
	case map[string]any:
		doc = v
	}

	if _, ok := doc["Resources"]; !ok {
		return errUtils.ErrAwsCloudFormationTemplateMissingResources
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
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	return nil
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
		return fmt.Errorf("%w: %w", errUtils.ErrAwsCloudFormationAPICallFailed, err)
	}
	return nil
}

// validateTemplate calls the server-side ValidateTemplate API (syntax +
// capability discovery) — an API-backed check, not a local linter. Local
// linting (cfn-lint/cfn-guard) is not built into this component type; users
// who want it declare those tools via the toolchain subsystem.
func validateTemplate(ctx context.Context, client CloudFormationClient, stackName, templateBody string) error {
	defer perf.Track(nil, "cloudformation.validateTemplate")()

	_, err := client.ValidateTemplate(ctx, &cloudformation.ValidateTemplateInput{
		TemplateBody: awsString(templateBody),
	})
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInvalidSpecificAwsCloudFormationComponent, err)
	}
	ui.Success(fmt.Sprintf("%s: template is valid", stackName))
	return nil
}
