package kubernetes

import (
	"fmt"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func renderManifestInputTemplates(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any) error {
	// The provision section is rendered so target fields that reference component
	// vars (e.g. a git target's path and commit.message) are materialized before
	// delivery, the same way paths/manifests/render templates are.
	for _, key := range []string{cfg.PathsSectionName, cfg.ManifestsSectionName, cfg.RenderSectionName, cfg.ProvisionSectionName} {
		value, ok := componentSection[key]
		if !ok {
			continue
		}
		rendered, err := renderManifestTemplateValue(atmosConfig, key, value, componentSection)
		if err != nil {
			return err
		}
		componentSection[key] = rendered
	}
	return nil
}

func renderManifestTemplateValue(atmosConfig *schema.AtmosConfiguration, name string, value any, data map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderManifestTemplateString(atmosConfig, name, typed, data)
	case []any:
		return renderManifestTemplateSlice(atmosConfig, name, typed, data)
	case []string:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = item
		}
		return renderManifestTemplateSlice(atmosConfig, name, items, data)
	case map[string]any:
		return renderManifestTemplateMap(atmosConfig, name, typed, data)
	default:
		return typed, nil
	}
}

// renderManifestTemplateString renders a single string value, skipping non-template strings.
func renderManifestTemplateString(atmosConfig *schema.AtmosConfiguration, name, value string, data map[string]any) (any, error) {
	if !strings.Contains(value, "{{") && !strings.Contains(value, "}}") {
		return value, nil
	}
	rendered, err := e.ProcessTmpl(atmosConfig, "kubernetes-"+name, value, data, false)
	if err != nil {
		return nil, fmt.Errorf("failed to render Kubernetes %s template: %w", name, err)
	}
	return rendered, nil
}

// renderManifestTemplateSlice renders each element of a slice value.
func renderManifestTemplateSlice(atmosConfig *schema.AtmosConfiguration, name string, items []any, data map[string]any) (any, error) {
	rendered := make([]any, len(items))
	for i, item := range items {
		itemRendered, err := renderManifestTemplateValue(atmosConfig, fmt.Sprintf("%s[%d]", name, i), item, data)
		if err != nil {
			return nil, err
		}
		rendered[i] = itemRendered
	}
	return rendered, nil
}

// renderManifestTemplateMap renders each value of a map.
func renderManifestTemplateMap(atmosConfig *schema.AtmosConfiguration, name string, items map[string]any, data map[string]any) (any, error) {
	rendered := make(map[string]any, len(items))
	for key, item := range items {
		itemRendered, err := renderManifestTemplateValue(atmosConfig, name+"."+key, item, data)
		if err != nil {
			return nil, err
		}
		rendered[key] = itemRendered
	}
	return rendered, nil
}
