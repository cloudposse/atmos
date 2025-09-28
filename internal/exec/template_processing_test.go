package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldProcessFileAsTemplate(t *testing.T) {
	tests := []struct {
		name           string
		filePath       string
		context        map[string]any
		skipProcessing bool
		expected       bool
		description    string
	}{
		{
			name:           "yaml.tmpl always processed",
			filePath:       "config.yaml.tmpl",
			context:        nil,
			skipProcessing: false,
			expected:       true,
			description:    "Files with .yaml.tmpl extension should always be processed",
		},
		{
			name:           "yml.tmpl always processed",
			filePath:       "config.yml.tmpl",
			context:        nil,
			skipProcessing: false,
			expected:       true,
			description:    "Files with .yml.tmpl extension should always be processed",
		},
		{
			name:           "yaml.tmpl with context still processed",
			filePath:       "config.yaml.tmpl",
			context:        map[string]any{"env": "prod"},
			skipProcessing: false,
			expected:       true,
			description:    "Files with .yaml.tmpl extension should be processed even with context",
		},
		{
			name:           "plain tmpl always processed",
			filePath:       "defaults.tmpl",
			context:        nil,
			skipProcessing: false,
			expected:       true,
			description:    "Files with .tmpl extension should always be processed",
		},
		{
			name:           "nested path tmpl processed",
			filePath:       "catalog/terraform/service-iam-role/defaults.tmpl",
			context:        nil,
			skipProcessing: false,
			expected:       true,
			description:    "Files with .tmpl extension in nested paths should be processed",
		},
		{
			name:           "plain yaml with context",
			filePath:       "region.yaml",
			context:        map[string]any{"region": "us-west-2"},
			skipProcessing: false,
			expected:       true,
			description:    "Plain YAML files with context should be processed",
		},
		{
			name:           "plain yml with context",
			filePath:       "config.yml",
			context:        map[string]any{"env": "dev"},
			skipProcessing: false,
			expected:       true,
			description:    "Plain YML files with context should be processed",
		},
		{
			name:           "nested yaml with context",
			filePath:       "mixins/region/region_tmpl.yaml",
			context:        map[string]any{"region": "us-west-2", "environment": "uw2"},
			skipProcessing: false,
			expected:       true,
			description:    "Nested YAML files with context should be processed",
		},
		{
			name:           "plain yaml without context",
			filePath:       "static.yaml",
			context:        nil,
			skipProcessing: false,
			expected:       false,
			description:    "Plain YAML files without context should NOT be processed",
		},
		{
			name:           "plain yml without context",
			filePath:       "static.yml",
			context:        nil,
			skipProcessing: false,
			expected:       false,
			description:    "Plain YML files without context should NOT be processed",
		},
		{
			name:           "empty context map",
			filePath:       "config.yaml",
			context:        map[string]any{},
			skipProcessing: false,
			expected:       false,
			description:    "Empty context map should not trigger template processing",
		},
		{
			name:           "skip processing overrides yaml.tmpl",
			filePath:       "config.yaml.tmpl",
			context:        nil,
			skipProcessing: true,
			expected:       false,
			description:    "Skip flag should prevent template file processing",
		},
		{
			name:           "skip processing overrides context",
			filePath:       "config.yaml",
			context:        map[string]any{"env": "prod"},
			skipProcessing: true,
			expected:       false,
			description:    "Skip flag should prevent context-based processing",
		},
		{
			name:           "skip processing overrides tmpl",
			filePath:       "defaults.tmpl",
			context:        nil,
			skipProcessing: true,
			expected:       false,
			description:    "Skip flag should prevent .tmpl file processing",
		},
		{
			name:           "json file with context",
			filePath:       "data.json",
			context:        map[string]any{"key": "value"},
			skipProcessing: false,
			expected:       true,
			description:    "Non-YAML files with context should also be processed",
		},
		{
			name:           "json file without context",
			filePath:       "data.json",
			context:        nil,
			skipProcessing: false,
			expected:       false,
			description:    "Non-YAML files without context should not be processed",
		},
		{
			name:           "arbitrary extension with context",
			filePath:       "config.txt",
			context:        map[string]any{"data": "value"},
			skipProcessing: false,
			expected:       true,
			description:    "Any file with context should be processed as template",
		},
		{
			name:           "file with yaml in name but not extension",
			filePath:       "yaml-config.txt",
			context:        nil,
			skipProcessing: false,
			expected:       false,
			description:    "Files with yaml in name but not extension should not be processed without context",
		},
		{
			name:           "file with tmpl in middle of name",
			filePath:       "config.tmpl.backup",
			context:        nil,
			skipProcessing: false,
			expected:       false,
			description:    "Files with tmpl in middle but not at end should not be processed without context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldProcessFileAsTemplate(tt.filePath, tt.context, tt.skipProcessing)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

// TestShouldProcessFileAsTemplate_EdgeCases tests edge cases and boundary conditions.
func TestShouldProcessFileAsTemplate_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		context     map[string]any
		expected    bool
		description string
	}{
		{
			name:        "nil context",
			filePath:    "config.yaml",
			context:     nil,
			expected:    false,
			description: "Nil context should not trigger processing",
		},
		{
			name:        "context with nil values",
			filePath:    "config.yaml",
			context:     map[string]any{"key": nil},
			expected:    true,
			description: "Context with nil values still counts as having context",
		},
		{
			name:        "empty file path with context",
			filePath:    "",
			context:     map[string]any{"key": "value"},
			expected:    true,
			description: "Even empty filepath with context should return true",
		},
		{
			name:        "dot file yaml.tmpl",
			filePath:    ".hidden.yaml.tmpl",
			context:     nil,
			expected:    true,
			description: "Hidden files with template extension should be processed",
		},
		{
			name:        "uppercase extension YAML.TMPL",
			filePath:    "CONFIG.YAML.TMPL",
			context:     nil,
			expected:    false,
			description: "Uppercase extensions are not recognized (case-sensitive)",
		},
		{
			name:        "mixed case extension",
			filePath:    "config.Yaml.Tmpl",
			context:     nil,
			expected:    false,
			description: "Mixed case extensions are not recognized (case-sensitive)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldProcessFileAsTemplate(tt.filePath, tt.context, false)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}

// TestShouldProcessFileAsTemplate_RealWorldScenarios tests actual scenarios from the codebase.
func TestShouldProcessFileAsTemplate_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name        string
		filePath    string
		context     map[string]any
		expected    bool
		description string
	}{
		{
			name:     "region_tmpl.yaml from test fixtures",
			filePath: "mixins/region/region_tmpl.yaml",
			context: map[string]any{
				"region":      "us-west-1",
				"environment": "uw1",
			},
			expected:    true,
			description: "Real test fixture file should be processed with context",
		},
		{
			name:     "eks_cluster_tmpl_hierarchical.yaml with context",
			filePath: "catalog/terraform/eks_cluster_tmpl_hierarchical.yaml",
			context: map[string]any{
				"flavor":  "blue",
				"enabled": true,
			},
			expected:    true,
			description: "Component template file should be processed with context",
		},
		{
			name:     "service-iam-role defaults.tmpl",
			filePath: "catalog/terraform/service-iam-role/defaults.tmpl",
			context: map[string]any{
				"app_name":                  "test-app",
				"service_environment":       "prod",
				"service_account_namespace": "default",
			},
			expected:    true,
			description: "Service IAM role template should always be processed",
		},
		{
			name:        "static stack without context",
			filePath:    "orgs/cp/tenant1/dev/us-east-2.yaml",
			context:     nil,
			expected:    false,
			description: "Static stack files should not be processed without context",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldProcessFileAsTemplate(tt.filePath, tt.context, false)
			assert.Equal(t, tt.expected, result, tt.description)
		})
	}
}
