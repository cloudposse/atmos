// Package manifest validates stack manifest sections that are specific to aws/cloudformation components.
package manifest

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RejectTypeGenerate fails when the stack's type-level `aws/cloudformation:` section sets `generate`.
func RejectTypeGenerate(stackName string, typeSection map[string]any) error {
	defer perf.Track(nil, "cloudformation.manifest.RejectTypeGenerate")()

	return rejectGenerate(fmt.Sprintf("the '%s' section in the manifest '%s'", config.CloudFormationSectionName, stackName), typeSection)
}

// RejectComponentGenerate fails when a component, or its `overrides`, sets `generate`.
func RejectComponentGenerate(stackName, component string, componentMap map[string]any) error {
	defer perf.Track(nil, "cloudformation.manifest.RejectComponentGenerate")()

	where := fmt.Sprintf("'components.%s.%s' in the manifest '%s'", config.CloudFormationComponentType, component, stackName)
	if err := rejectGenerate(where, componentMap); err != nil {
		return err
	}
	overrides, _ := componentMap[config.OverridesSectionName].(map[string]any)
	return rejectGenerate(fmt.Sprintf("'overrides' of %s", where), overrides)
}

// rejectGenerate returns the unsupported-generate error when the section contains a `generate` key.
// The root-level stack `generate` is Terraform's global scope and is never passed here.
func rejectGenerate(where string, section map[string]any) error {
	if _, ok := section[config.GenerateSectionName]; !ok {
		return nil
	}
	return errUtils.Build(errUtils.ErrAwsCloudFormationGenerateUnsupported).
		WithExplanationf("Found a 'generate' key in %s.", where).
		WithHint("Remove 'generate'. Put the template in an inline 'template:', which Atmos renders with Go templates and YAML functions, or reference a file with 'path:'.").
		Err()
}
