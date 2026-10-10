package exec

import (
	"github.com/cloudposse/atmos/pkg/perf"
)

// stackLocalsForComponent returns the file-level locals of a stack manifest that apply to a
// component type: root-level `locals:` overlaid with the type section's own `locals:`.
// Stack-level locals are stripped from the processed component sections, so the raw manifest
// (rawStackConfigs, keyed by stack file name) is the only place they remain.
// It returns nil when the manifest declares none.
func stackLocalsForComponent(rawStackConfigs map[string]map[string]any, stackFile, componentType string) map[string]any {
	defer perf.Track(nil, "exec.stackLocalsForComponent")()

	stackConfig, ok := rawStackConfigs[stackFile]["stack"].(map[string]any)
	if !ok {
		return nil
	}
	locals := getLocalsForComponentType(stackConfig, componentType)
	if len(locals) == 0 {
		return nil
	}
	return locals
}
