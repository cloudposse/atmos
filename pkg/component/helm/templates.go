package helm

import (
	"fmt"
	"strings"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// renderInputTemplates renders Go templates in the component input sections so
// fields that reference component values (e.g. chart version, a git target's
// path/commit.message, value file paths) are materialized before use.
func renderInputTemplates(atmosConfig *schema.AtmosConfiguration, componentSection map[string]any) error {
	for _, key := range []string{
		cfg.ChartSectionName,
		cfg.ValuesSectionName,
		cfg.ValuesFilesSectionName,
		cfg.RepositoriesSectionName,
		cfg.RenderSectionName,
		cfg.ProvisionSectionName,
		"version",
		"repository",
		"namespace",
		"name",
	} {
		value, ok := componentSection[key]
		if !ok {
			continue
		}
		rendered, err := renderTemplateValue(atmosConfig, key, value, componentSection)
		if err != nil {
			return err
		}
		componentSection[key] = rendered
	}
	return nil
}

func renderTemplateValue(atmosConfig *schema.AtmosConfiguration, name string, value any, data map[string]any) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderTemplateString(atmosConfig, name, typed, data)
	case []any:
		return renderTemplateSlice(atmosConfig, name, typed, data)
	case []string:
		items := make([]any, len(typed))
		for i, item := range typed {
			items[i] = item
		}
		return renderTemplateSlice(atmosConfig, name, items, data)
	case map[string]any:
		return renderTemplateMap(atmosConfig, name, typed, data)
	default:
		return typed, nil
	}
}

func renderTemplateString(atmosConfig *schema.AtmosConfiguration, name, value string, data map[string]any) (any, error) {
	if !strings.Contains(value, "{{") && !strings.Contains(value, "}}") {
		return value, nil
	}
	rendered, err := e.ProcessTmpl(atmosConfig, "helm-"+name, value, data, false)
	if err != nil {
		return nil, fmt.Errorf("failed to render Helm %s template: %w", name, err)
	}
	return rendered, nil
}

func renderTemplateSlice(atmosConfig *schema.AtmosConfiguration, name string, items []any, data map[string]any) (any, error) {
	rendered := make([]any, len(items))
	for i, item := range items {
		itemRendered, err := renderTemplateValue(atmosConfig, fmt.Sprintf("%s[%d]", name, i), item, data)
		if err != nil {
			return nil, err
		}
		rendered[i] = itemRendered
	}
	return rendered, nil
}

func renderTemplateMap(atmosConfig *schema.AtmosConfiguration, name string, items map[string]any, data map[string]any) (any, error) {
	rendered := make(map[string]any, len(items))
	for key, item := range items {
		itemRendered, err := renderTemplateValue(atmosConfig, name+"."+key, item, data)
		if err != nil {
			return nil, err
		}
		rendered[key] = itemRendered
	}
	return rendered, nil
}
