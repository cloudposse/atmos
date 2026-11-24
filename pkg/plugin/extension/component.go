package extension

import (
	"strings"

	"github.com/cloudposse/atmos/pkg/component/provideroptions"
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

	schemaPath, err := componentExportSchema(binding.Metadata)
	if err != nil {
		return err
	}
	if _, err := CompileSchema(binding.Directory, schemaPath); err != nil {
		return err
	}
	if _, err := ComponentModes(binding.Metadata); err != nil {
		return err
	}
	_, err = provideroptions.Parse(binding.Name, binding.Metadata["options"])
	return err
}

// componentExportSchema checks the metadata keys and the `component_type` of a configuration export
// and returns the package-local path of its provider-settings schema.
func componentExportSchema(metadata map[string]any) (string, error) {
	for key := range metadata {
		if key != "component_type" && key != "schema" && key != "modes" && key != "options" {
			return "", invalidMetadata("Unsupported component configuration export metadata.", "Declare only `component_type`, `schema`, `modes`, and `options` for this contract version.")
		}
	}
	componentType, ok := metadata["component_type"].(string)
	if !ok || componentType == "" || strings.TrimSpace(componentType) != componentType {
		return "", invalidMetadata("The configuration export has no valid `component_type`.", "Name the component family it supplies, for example `terraform`.")
	}
	schemaPath, ok := metadata["schema"].(string)
	if !ok {
		return "", invalidMetadata("The configuration export has no `schema` path.", "Declare a package-local JSON Schema for provider settings.")
	}
	return schemaPath, nil
}

// ComponentModes returns the execution modes a configuration export declares. An export that declares
// none supports only the native mode.
func ComponentModes(metadata map[string]any) ([]string, error) {
	defer perf.Track(nil, "extension.ComponentModes")()

	raw, declared := metadata["modes"]
	if !declared {
		return []string{provideroptions.ModeNative}, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, invalidMetadata("The configuration export declares `modes` incorrectly.", "List one or more of `native` and `executable`.")
	}
	modes := make([]string, 0, len(items))
	for _, item := range items {
		mode, _ := item.(string)
		if mode != provideroptions.ModeNative && mode != provideroptions.ModeExecutable {
			return nil, invalidMetadata("The configuration export declares an unknown mode.", "List one or more of `native` and `executable`.")
		}
		modes = append(modes, mode)
	}
	return modes, nil
}
