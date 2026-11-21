package extension

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	ComponentDiscovery   = "component.discovery"
	ComponentValidation  = "component.validation"
	ComponentPreparation = "component.preparation"
	ComponentState       = "component.state"
)

// IsComponent reports whether a capability supplies component configuration.
//
//nolint:lintroller // Pure capability lookup; no I/O.
func IsComponent(capability string) bool {
	switch capability {
	case ComponentDiscovery, ComponentValidation, ComponentPreparation, ComponentState:
		return true
	default:
		return false
	}
}

// ValidateComponent checks configuration-provider metadata without starting code.
func ValidateComponent(binding Binding) error {
	defer perf.Track(nil, "extension.ValidateComponent")()

	for key := range binding.Metadata {
		if key != "component_type" && key != "schema" {
			return invalidMetadata("Unsupported component configuration export metadata.", "Declare only `component_type` and `schema` for this contract version.")
		}
	}
	componentType, ok := binding.Metadata["component_type"].(string)
	if !ok || componentType == "" || strings.TrimSpace(componentType) != componentType {
		return invalidMetadata("The configuration export has no valid `component_type`.", "Name the component family it supplies, for example `terraform`.")
	}
	schemaPath, ok := binding.Metadata["schema"].(string)
	if !ok {
		return invalidMetadata("The configuration export has no `schema` path.", "Declare a package-local JSON Schema for provider settings.")
	}
	_, err := CompileSchema(binding.Directory, schemaPath)
	return err
}
