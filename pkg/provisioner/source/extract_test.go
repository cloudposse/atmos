package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestExtractSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expectError     error
		expectNil       bool // If true, result should be nil (no source configured).
		expectedURI     string
		expectedVersion string
	}{
		{
			name: "top-level string source",
			componentConfig: map[string]any{
				"source": "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			expectedVersion: "",
		},
		{
			name: "top-level map source",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri":     "github.com/cloudposse/terraform-aws-components//modules/vpc",
					"version": "v1.0.0",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v1.0.0",
		},
		{
			name: "top-level source takes precedence over metadata.source",
			componentConfig: map[string]any{
				"source": "github.com/org/top-level//module",
				"metadata": map[string]any{
					"source": "github.com/org/metadata//module",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/org/top-level//module",
			expectedVersion: "",
		},
		{
			name: "falls back to metadata.source when no top-level source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			expectedVersion: "",
		},
		{
			name: "map with included_paths and excluded_paths",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
					"version":        "v2.0.0",
					"included_paths": []any{"*.tf", "*.tfvars"},
					"excluded_paths": []any{"*.md", "tests/*"},
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v2.0.0",
		},
		{
			name: "no source returns nil (not an error)",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"foo": "bar",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name: "metadata but no source field returns nil",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name:            "empty component config returns nil",
			componentConfig: map[string]any{},
			expectError:     nil,
			expectNil:       true,
		},
		{
			name:            "nil component config returns nil",
			componentConfig: nil,
			expectError:     nil,
			expectNil:       true,
		},
		{
			name: "map without uri field returns error",
			componentConfig: map[string]any{
				"source": map[string]any{
					"version": "v1.0.0",
				},
			},
			expectError: errUtils.ErrSourceInvalidSpec,
			expectNil:   true,
		},
		{
			name: "empty string URI returns valid spec with empty URI",
			componentConfig: map[string]any{
				"source": "",
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "",
			expectedVersion: "",
		},
		{
			name: "invalid type returns error",
			componentConfig: map[string]any{
				"source": 12345,
			},
			expectError: errUtils.ErrSourceInvalidSpec,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractSource(tt.componentConfig)

			switch {
			case tt.expectError != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, result)
			case tt.expectNil:
				require.NoError(t, err)
				assert.Nil(t, result)
			default:
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedURI, result.Uri)
				assert.Equal(t, tt.expectedVersion, result.Version)
			}
		})
	}
}

func TestExtractMetadataSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expectError     error
		expectNil       bool // If true, result should be nil (no source configured).
		expectedURI     string
		expectedVersion string
	}{
		{
			name: "string URI in metadata.source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc?ref=v1.0.0",
			expectedVersion: "",
		},
		{
			name: "map with uri field in metadata.source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri":     "github.com/cloudposse/terraform-aws-components//modules/vpc",
						"version": "v1.0.0",
					},
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v1.0.0",
		},
		{
			name: "map with included_paths and excluded_paths",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
						"version":        "v2.0.0",
						"included_paths": []any{"*.tf", "*.tfvars"},
						"excluded_paths": []any{"*.md", "tests/*"},
					},
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "github.com/cloudposse/terraform-aws-components//modules/vpc",
			expectedVersion: "v2.0.0",
		},
		{
			name: "no metadata field returns nil (not an error)",
			componentConfig: map[string]any{
				"vars": map[string]any{
					"foo": "bar",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name: "metadata but no source field returns nil",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expectError: nil,
			expectNil:   true,
		},
		{
			name:            "empty component config returns nil",
			componentConfig: map[string]any{},
			expectError:     nil,
			expectNil:       true,
		},
		{
			name:            "nil component config returns nil",
			componentConfig: nil,
			expectError:     nil,
			expectNil:       true,
		},
		{
			name: "map without uri field returns error",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"version": "v1.0.0",
					},
				},
			},
			expectError: errUtils.ErrSourceInvalidSpec,
			expectNil:   true,
		},
		{
			name: "empty string URI returns valid spec with empty URI",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "",
				},
			},
			expectError:     nil,
			expectNil:       false,
			expectedURI:     "",
			expectedVersion: "",
		},
		{
			name: "invalid type returns error",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": 12345,
				},
			},
			expectError: errUtils.ErrSourceInvalidSpec,
			expectNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractMetadataSource(tt.componentConfig)

			switch {
			case tt.expectError != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, result)
			case tt.expectNil:
				require.NoError(t, err)
				assert.Nil(t, result)
			default:
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tt.expectedURI, result.Uri)
				assert.Equal(t, tt.expectedVersion, result.Version)
			}
		})
	}
}

func TestExtractSource_IncludedExcludedPaths(t *testing.T) {
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
			"included_paths": []any{"*.tf", "*.tfvars"},
			"excluded_paths": []any{"*.md", "tests/*"},
		},
	}

	result, err := ExtractSource(componentConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"*.tf", "*.tfvars"}, result.IncludedPaths)
	assert.Equal(t, []string{"*.md", "tests/*"}, result.ExcludedPaths)
}

func TestExtractMetadataSource_IncludedExcludedPaths(t *testing.T) {
	componentConfig := map[string]any{
		"metadata": map[string]any{
			"source": map[string]any{
				"uri":            "github.com/cloudposse/terraform-aws-components//modules/vpc",
				"included_paths": []any{"*.tf", "*.tfvars"},
				"excluded_paths": []any{"*.md", "tests/*"},
			},
		},
	}

	result, err := ExtractMetadataSource(componentConfig)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, []string{"*.tf", "*.tfvars"}, result.IncludedPaths)
	assert.Equal(t, []string{"*.md", "tests/*"}, result.ExcludedPaths)
}

func TestHasSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "has top-level string source",
			componentConfig: map[string]any{
				"source": "github.com/example/repo//module",
			},
			expected: true,
		},
		{
			name: "has top-level map source",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri": "github.com/example/repo//module",
				},
			},
			expected: true,
		},
		{
			name: "has metadata.source string",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "github.com/example/repo//module",
				},
			},
			expected: true,
		},
		{
			name: "has metadata.source map",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/repo//module",
					},
				},
			},
			expected: true,
		},
		{
			name: "no source",
			componentConfig: map[string]any{
				"vars": map[string]any{},
			},
			expected: false,
		},
		{
			name: "metadata but no source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: false,
		},
		{
			name: "empty source string is still considered present",
			componentConfig: map[string]any{
				"source": "",
			},
			expected: true,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSource(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasMetadataSource(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "has string source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "github.com/example/repo//module",
				},
			},
			expected: true,
		},
		{
			name: "has map source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": map[string]any{
						"uri": "github.com/example/repo//module",
					},
				},
			},
			expected: true,
		},
		{
			name: "no metadata",
			componentConfig: map[string]any{
				"vars": map[string]any{},
			},
			expected: false,
		},
		{
			name: "metadata but no source",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: false,
		},
		{
			name: "empty source string is still considered present",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"source": "",
				},
			},
			expected: true,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasMetadataSource(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}
