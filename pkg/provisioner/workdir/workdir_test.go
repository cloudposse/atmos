package workdir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWorkdirEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "no provision",
			config:   map[string]any{},
			expected: false,
		},
		{
			name: "provision without workdir",
			config: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "workdir without enabled",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{},
				},
			},
			expected: false,
		},
		{
			name: "enabled false",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name: "enabled true",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "enabled as string (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name: "workdir as bool instead of map (invalid)",
			config: map[string]any{
				"provision": map[string]any{
					"workdir": true,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkdirEnabled(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name     string
		config   map[string]any
		expected string
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: "",
		},
		{
			name: "component in root",
			config: map[string]any{
				"component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "component in metadata",
			config: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "component in vars (fallback)",
			config: map[string]any{
				"vars": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name: "root takes precedence",
			config: map[string]any{
				"component": "root-vpc",
				"metadata": map[string]any{
					"component": "metadata-vpc",
				},
			},
			expected: "root-vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDefaultPathFilter_Match(t *testing.T) {
	filter := NewDefaultPathFilter()

	tests := []struct {
		name     string
		path     string
		included []string
		excluded []string
		expected bool
	}{
		{
			name:     "no patterns includes all",
			path:     "main.tf",
			included: nil,
			excluded: nil,
			expected: true,
		},
		{
			name:     "matches include pattern",
			path:     "main.tf",
			included: []string{"*.tf"},
			excluded: nil,
			expected: true,
		},
		{
			name:     "does not match include pattern",
			path:     "README.md",
			included: []string{"*.tf"},
			excluded: nil,
			expected: false,
		},
		{
			name:     "matches exclude pattern",
			path:     "test.tf",
			included: []string{"*.tf"},
			excluded: []string{"test*"},
			expected: false,
		},
		{
			name:     "no include but matches exclude",
			path:     "test.tf",
			included: nil,
			excluded: []string{"test*"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.Match(tt.path, tt.included, tt.excluded)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
