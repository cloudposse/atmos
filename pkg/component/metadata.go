package component

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MetadataResult contains the effective metadata and component identity flags.
type MetadataResult struct {
	Fields        map[string]any
	BaseComponent string
	Abstract      bool
	Enabled       bool
	Locked        bool
}

// ProcessMetadata resolves the base component and abstract, enabled, and locked flags.
// The metadata.component field takes precedence over component; self references have no base.
func ProcessMetadata(component string, componentSection map[string]any) MetadataResult {
	defer perf.Track(nil, "component.ProcessMetadata")()

	// Find base component in the component attribute.
	base, _ := componentSection[cfg.ComponentSectionName].(string)
	var metadata map[string]any
	if value, exists := componentSection["metadata"]; exists {
		metadata = value.(map[string]any)
	}
	// The metadata.component attribute overrides component, including an explicit empty value.
	if override, ok := metadata[cfg.ComponentSectionName].(string); ok {
		base = override
	}
	// A reference to the Atmos component itself does not establish a base component.
	if component == base {
		base = ""
	}
	kind, _ := metadata["type"].(string)
	enabled := true
	if value, ok := metadata["enabled"].(bool); ok {
		enabled = value
	}
	locked, _ := metadata["locked"].(bool)
	return MetadataResult{Fields: metadata, BaseComponent: base, Abstract: kind == "abstract", Enabled: enabled, Locked: locked}
}
