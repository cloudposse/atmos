package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTemplateFile(t *testing.T) {
	testCases := []struct {
		name       string
		filePath   string
		isTemplate bool
	}{
		{
			name:       "yaml template file",
			filePath:   "config.yaml.tmpl",
			isTemplate: true,
		},
		{
			name:       "yml template file",
			filePath:   "config.yml.tmpl",
			isTemplate: true,
		},
		{
			name:       "yaml template with path",
			filePath:   "path/to/config.yaml.tmpl",
			isTemplate: true,
		},
		{
			name:       "yml template with path",
			filePath:   "path/to/config.yml.tmpl",
			isTemplate: true,
		},
		{
			name:       "plain yaml file",
			filePath:   "config.yaml",
			isTemplate: false,
		},
		{
			name:       "plain yml file",
			filePath:   "config.yml",
			isTemplate: false,
		},
		{
			name:       "plain yaml with path",
			filePath:   "path/to/config.yaml",
			isTemplate: false,
		},
		{
			name:       "plain yml with path",
			filePath:   "path/to/config.yml",
			isTemplate: false,
		},
		{
			name:       "just tmpl extension",
			filePath:   "config.tmpl",
			isTemplate: false,
		},
		{
			name:       "json template",
			filePath:   "config.json.tmpl",
			isTemplate: false,
		},
		{
			name:       "empty path",
			filePath:   "",
			isTemplate: false,
		},
		{
			name:       "complex nested path yaml template",
			filePath:   "stacks/orgs/acme/prod/catalog/component.yaml.tmpl",
			isTemplate: true,
		},
		{
			name:       "complex nested path yml template",
			filePath:   "stacks/orgs/acme/prod/catalog/component.yml.tmpl",
			isTemplate: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsTemplateFile(tc.filePath)
			assert.Equal(t, tc.isTemplate, result, "IsTemplateFile(%q) should return %v", tc.filePath, tc.isTemplate)
		})
	}
}
