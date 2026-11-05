package manifest

import (
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// SettingsByStack extracts the resolved region and timeout defaults of every
// aws/cloudformation component from processed stacks, keyed by stack name and then
// component name. Callers compare two of these maps to prove a configuration
// rewrite left effective CloudFormation settings unchanged.
func SettingsByStack(stacks map[string]any) map[string]any {
	defer perf.Track(nil, "cloudformation.manifest.SettingsByStack")()

	result := make(map[string]any, len(stacks))
	for name, raw := range stacks {
		stack, _ := raw.(map[string]any)
		components, _ := stack[config.ComponentsSectionName].(map[string]any)
		cfn, _ := components[config.CloudFormationComponentType].(map[string]any)
		settings := make(map[string]any, len(cfn))
		for component, raw := range cfn {
			section, _ := raw.(map[string]any)
			settings[component] = DefaultsSection(section)
		}
		result[name] = settings
	}
	return result
}
