package scaffold

import (
	"github.com/cloudposse/atmos/pkg/generator/templates"
)

// ProductionTemplateLoader implements TemplateLoader using real template loading logic.
type ProductionTemplateLoader struct{}

// NewProductionTemplateLoader creates a new production template loader.
func NewProductionTemplateLoader() TemplateLoader {
	return &ProductionTemplateLoader{}
}

// LoadTemplates loads all available scaffold templates from embedded sources.
func (l *ProductionTemplateLoader) LoadTemplates() (map[string]templates.Configuration, error) {
	return templates.GetAvailableConfigurations()
}

// MergeConfiguredTemplates merges templates from atmos.yaml into the configs map.
// It also updates the origins map to track which templates came from atmos.yaml.
func (l *ProductionTemplateLoader) MergeConfiguredTemplates(configs map[string]templates.Configuration, origins map[string]string) error {
	return mergeConfiguredTemplates(configs, origins)
}
