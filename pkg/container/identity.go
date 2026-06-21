package container

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Canonical runtime labels for Atmos-managed container component instances.
// These derive from the component instance address
// (<stack>/<component_type>/<component>) and are used for label-based discovery
// instead of local state files.
const (
	// LabelStack records the stack of the component instance.
	LabelStack = "tools.atmos.stack"
	// LabelComponentType records the component kind (e.g. "container").
	LabelComponentType = "tools.atmos.component_type"
	// LabelComponent records the component name.
	LabelComponent = "tools.atmos.component"
	// LabelInstance records the full canonical instance address.
	LabelInstance = "tools.atmos.instance"
)

// maxRuntimeNameLength bounds the sanitized runtime container name length.
// Docker/Podman allow long names; this keeps generated names readable and
// within conservative limits.
const maxRuntimeNameLength = 200

// nameSanitizePattern matches any run of characters that are invalid in a
// container name. Valid characters are letters, digits, underscore, dot, and
// dash.
var nameSanitizePattern = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// InstanceAddress builds the canonical component instance address
// "<stack>/<component_type>/<component>" (e.g. "dev/container/api").
func InstanceAddress(stack, componentType, component string) string {
	defer perf.Track(nil, "container.InstanceAddress")()

	return fmt.Sprintf("%s/%s/%s", stack, componentType, component)
}

// RuntimeName builds the sanitized runtime container name for an instance,
// projecting the canonical address onto a name safe for Docker/Podman
// (e.g. "atmos-dev-container-api").
func RuntimeName(stack, componentType, component string) string {
	defer perf.Track(nil, "container.RuntimeName")()

	raw := fmt.Sprintf("atmos-%s-%s-%s", stack, componentType, component)
	return sanitizeName(raw, "atmos", maxRuntimeNameLength)
}

// InstanceLabels builds the canonical label map used both to create and to
// discover an Atmos-managed container component instance.
func InstanceLabels(stack, componentType, component string) map[string]string {
	defer perf.Track(nil, "container.InstanceLabels")()

	return map[string]string{
		LabelStack:         stack,
		LabelComponentType: componentType,
		LabelComponent:     component,
		LabelInstance:      InstanceAddress(stack, componentType, component),
	}
}

// DiscoveryFilter returns the runtime List() filter that matches a specific
// instance by its canonical instance label.
func DiscoveryFilter(stack, componentType, component string) map[string]string {
	defer perf.Track(nil, "container.DiscoveryFilter")()

	return map[string]string{
		"label": fmt.Sprintf("%s=%s", LabelInstance, InstanceAddress(stack, componentType, component)),
	}
}

// IsContainerRunning reports whether a runtime status string indicates a
// running container. It accepts both Docker's "running"/"Up ..." forms.
func IsContainerRunning(status string) bool {
	defer perf.Track(nil, "container.IsContainerRunning")()

	status = strings.ToLower(status)
	return strings.Contains(status, "running") || strings.HasPrefix(status, "up ")
}

// sanitizeName replaces characters invalid in container names with "-", trims
// leading/trailing separators, applies a fallback when the result is empty, and
// bounds the length to maxLen.
func sanitizeName(value, fallback string, maxLen int) string {
	value = strings.Trim(nameSanitizePattern.ReplaceAllString(value, "-"), "-.")
	if value == "" {
		return fallback
	}
	if len(value) > maxLen {
		return value[:maxLen]
	}
	return value
}
