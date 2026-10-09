package cloudformation

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cfntypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
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

	if config == nil || isAbstractComponent(config) {
		return nil
	}

	stackName, _ := config[cfg.StackNameSectionName].(string)
	templateRaw := config[cfg.TemplateSectionName]
	path, _ := config[cfg.TemplatePathSectionName].(string)
	templatePresent := isTemplatePresent(templateRaw)

	switch {
	case templatePresent && path != "":
		return fmt.Errorf("%w: stack %q", errUtils.ErrAwsCloudFormationTemplateAndPathMutuallyExclusive, stackName)
	case !templatePresent && !hasFileTemplate(config):
		return errUtils.ErrMissingAwsCloudFormationTemplate
	}

	if templatePresent {
		if err := sanityCheckInlineTemplate(templateRaw); err != nil {
			return inlineTemplateError(stackName, templateRaw, err)
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
// DescribeChangeSet does not expose its type, so named executions check stack
// status only when a policy is configured. REVIEW_IN_PROGRESS has no resources.
func prepareStackPolicy(ctx context.Context, client CloudFormationClient, spec *stackSpec, result *changeSetResult) (bool, error) {
	if spec.StackPolicyBody == "" {
		return false, nil
	}
	if result.ChangeSetType == cfntypes.ChangeSetTypeCreate {
		return true, nil
	}
	if result.ChangeSetType == "" {
		exists, err := stackExists(ctx, client, spec.StackName)
		if err != nil {
			return false, wrapAPICallError(spec.StackName, err)
		}
		if !exists {
			return true, nil
		}
	}
	return false, setStackPolicy(ctx, client, spec)
}

// applyTerminationProtection enables termination protection as a follow-up call
// after a successful apply, when the component opts in via
// termination_protection: true — CreateChangeSet/ExecuteChangeSet have no
// termination-protection parameter, the same "no changeset parameter" shape
// setStackPolicy uses for stack policy. It's a no-op when the component hasn't
// opted in, so a target that doesn't support UpdateTerminationProtection (e.g. an
// AWS emulator) is never touched by components that never asked for the feature.
// Disabling protection is deliberately not reconciled here: it only happens via
// the explicit `--disable-termination-protection` delete flag (see delete.go),
// so a stack's protection is never silently turned off by an apply.
func applyTerminationProtection(ctx context.Context, client CloudFormationClient, spec *stackSpec) error {
	defer perf.Track(nil, "cloudformation.applyTerminationProtection")()

	if !spec.TerminationProtection {
		return nil
	}

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
		return fmt.Errorf(wrapFmt, errUtils.ErrInvalidSpecificAwsCloudFormationComponent, err)
	}
	ui.Success(fmt.Sprintf("%s: template is valid", spec.StackName))
	return nil
}

// inlineTemplateError explains the path/template distinction for file-looking
// values without changing validation or adding hints to malformed inline bodies.
func inlineTemplateError(stackName string, raw any, err error) error {
	value, ok := raw.(string)
	if ok && looksLikeTemplateFileRef(value) {
		return errUtils.Build(errUtils.ErrInvalidAwsCloudFormationSettings).
			WithCause(err).
			WithExplanationf("Stack %q: template is an inline body, not a file path.", stackName).
			WithHintf("Use path: %s to reference a template file.", strings.TrimSpace(value)).
			Err()
	}
	return fmt.Errorf("stack %q: %w", stackName, err)
}

func looksLikeTemplateFileRef(value string) bool {
	if strings.ContainsAny(value, "\n\r:{}") {
		return false
	}
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(value))) {
	case ".yaml", ".yml", ".json", ".template":
		return true
	default:
		return false
	}
}

// hasFileTemplate accepts an explicit path or a source whose filename can be determined after
// provisioning.
func hasFileTemplate(config map[string]any) bool {
	path, _ := config[cfg.TemplatePathSectionName].(string)
	return path != "" || source.HasSource(config)
}
