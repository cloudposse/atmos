package yaml

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// ComponentPath returns a yq path with the component type and name quoted as literal keys.
func ComponentPath(componentType, componentName, relPath string) string {
	defer perf.Track(nil, "yaml.ComponentPath")()

	parts := []string{
		"components",
		QuotePathSegment(componentType),
		QuotePathSegment(componentName),
	}
	joined := strings.Join(parts, ".")
	if relPath != "" {
		joined += "." + relPath
	}
	return joined
}
