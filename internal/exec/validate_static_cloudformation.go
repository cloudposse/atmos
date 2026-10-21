package exec

import (
	"context"

	cfnmanifest "github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// StaticCloudFormationSettings resolves regions and timeouts through the production import and
// inheritance pipeline without evaluating templates, authenticating, or calling cloud APIs.
func StaticCloudFormationSettings(ctx context.Context, config *schema.AtmosConfiguration, inputs StaticStackInputs) (map[string]any, error) {
	defer perf.Track(config, "exec.StaticCloudFormationSettings")()

	stacks, err := processStaticStacks(ctx, config, inputs)
	if err != nil {
		return nil, err
	}
	result := map[string]any{}
	for name, raw := range stacks {
		stack, _ := raw.(map[string]any)
		components, _ := stack[cfg.ComponentsSectionName].(map[string]any)
		cfn, _ := components[cfg.CloudFormationComponentType].(map[string]any)
		settings := map[string]any{}
		for component, raw := range cfn {
			section, _ := raw.(map[string]any)
			settings[component] = cfnmanifest.DefaultsSection(section)
		}
		result[name] = settings
	}
	return result, nil
}
