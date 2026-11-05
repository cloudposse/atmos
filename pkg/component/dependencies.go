package component

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Dependencies returns normalized component dependencies and whether they use the
// modern declaration format. Graph consumers share the same legacy fallback and
// required-value handling as the single-type graph executor.
func Dependencies(section map[string]any, kind, stack, leftDelim string) ([]schema.ComponentDependency, bool, error) {
	defer perf.Track(nil, "component.Dependencies")()

	return componentDependencies(section, kind, stack, leftDelim)
}
