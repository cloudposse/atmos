package helm

import (
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/manifest"
)

// renderNoun is the human-readable object noun used in render status output.
const renderNoun = "Helm"

// resolveRenderOptions resolves the manifest output options from CLI flags and
// the component's render section. CLI flags take precedence.
func resolveRenderOptions(flags map[string]any, componentSection map[string]any) manifest.RenderOptions {
	options := renderOptionsFromComponent(componentSection)
	options.Noun = renderNoun

	if value, ok := flags["output"].(string); ok && value != "" {
		options.Output = value
		options.OutputDir = ""
	}
	if value, ok := flags["output_dir"].(string); ok && value != "" {
		options.OutputDir = value
		options.Output = ""
	}
	if value, ok := flags["split"].(bool); ok && value {
		options.Split = true
	}

	return options
}

func renderOptionsFromComponent(componentSection map[string]any) manifest.RenderOptions {
	renderSection, ok := componentSection[cfg.RenderSectionName].(map[string]any)
	if !ok {
		return manifest.RenderOptions{}
	}
	outputSection, ok := renderSection["output"].(map[string]any)
	if !ok {
		return manifest.RenderOptions{}
	}

	options := manifest.RenderOptions{}
	if split, ok := outputSection["split"].(bool); ok {
		options.Split = split
	}
	if path, ok := outputSection["path"].(string); ok && path != "" {
		if options.Split {
			options.OutputDir = path
		} else {
			options.Output = path
		}
	}

	return options
}
