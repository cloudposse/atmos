package source

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractComponentName(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        string
	}{
		{
			name: "component field present",
			componentConfig: map[string]any{
				"component": "vpc",
			},
			expected: "vpc",
		},
		{
			name: "metadata.component present",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"component": "s3-bucket",
				},
			},
			expected: "s3-bucket",
		},
		{
			name: "component field takes priority over metadata",
			componentConfig: map[string]any{
				"component": "vpc",
				"metadata": map[string]any{
					"component": "s3-bucket",
				},
			},
			expected: "vpc",
		},
		{
			name:            "empty config returns empty string",
			componentConfig: map[string]any{},
			expected:        "",
		},
		{
			name:            "nil config returns empty string",
			componentConfig: nil,
			expected:        "",
		},
		{
			name: "empty component field returns empty string",
			componentConfig: map[string]any{
				"component": "",
			},
			expected: "",
		},
		{
			name: "metadata without component field",
			componentConfig: map[string]any{
				"metadata": map[string]any{
					"other": "value",
				},
			},
			expected: "",
		},
		{
			name: "metadata is not a map",
			componentConfig: map[string]any{
				"metadata": "not-a-map",
			},
			expected: "",
		},
		{
			name: "component is not a string",
			componentConfig: map[string]any{
				"component": 12345,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractComponentName(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorkdirEnabled(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		expected        bool
	}{
		{
			name: "workdir enabled",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "workdir disabled",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": false,
					},
				},
			},
			expected: false,
		},
		{
			name:            "no provision section",
			componentConfig: map[string]any{},
			expected:        false,
		},
		{
			name: "no workdir section",
			componentConfig: map[string]any{
				"provision": map[string]any{},
			},
			expected: false,
		},
		{
			name: "workdir without enabled field",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"other": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "provision is not a map",
			componentConfig: map[string]any{
				"provision": "not-a-map",
			},
			expected: false,
		},
		{
			name: "workdir is not a map",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": "not-a-map",
				},
			},
			expected: false,
		},
		{
			name: "enabled is not a bool",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": "true",
					},
				},
			},
			expected: false,
		},
		{
			name:            "nil config",
			componentConfig: nil,
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkdirEnabled(tt.componentConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: needsProvisioning is tested via TestNeedsVendoring in source_test.go
// as it shares the same underlying logic.

func TestDetermineSourceTargetDirectory(t *testing.T) {
	tests := []struct {
		name            string
		atmosConfig     *schema.AtmosConfiguration
		componentType   string
		component       string
		componentConfig map[string]any
		expectedDir     string
		expectedWorkdir bool
		expectError     bool
	}{
		{
			name: "standard terraform component path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType:   "terraform",
			component:       "vpc",
			componentConfig: map[string]any{},
			expectedDir:     "/base/components/terraform/vpc",
			expectedWorkdir: false,
			expectError:     false,
		},
		{
			name: "workdir enabled with stack",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"atmos_stack": "dev-us-east-1",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     "/base/.workdir/terraform/dev-us-east-1-vpc",
			expectedWorkdir: true,
			expectError:     false,
		},
		{
			name: "workdir enabled without stack returns error",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     "",
			expectedWorkdir: false,
			expectError:     true,
		},
		{
			name: "empty base path defaults to current dir",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			componentType: "terraform",
			component:     "vpc",
			componentConfig: map[string]any{
				"atmos_stack": "dev",
				"provision": map[string]any{
					"workdir": map[string]any{
						"enabled": true,
					},
				},
			},
			expectedDir:     ".workdir/terraform/dev-vpc",
			expectedWorkdir: true,
			expectError:     false,
		},
		{
			name: "helmfile component type",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			componentType:   "helmfile",
			component:       "nginx",
			componentConfig: map[string]any{},
			expectedDir:     "/base/components/helmfile/nginx",
			expectedWorkdir: false,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, isWorkdir, err := determineSourceTargetDirectory(tt.atmosConfig, tt.componentType, tt.component, tt.componentConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, filepath.FromSlash(tt.expectedDir), dir)
				assert.Equal(t, tt.expectedWorkdir, isWorkdir)
			}
		})
	}
}

func TestExtractSourceAndComponent(t *testing.T) {
	tests := []struct {
		name              string
		componentConfig   map[string]any
		expectedSource    bool
		expectedComponent string
		expectError       bool
	}{
		{
			name: "valid source and component",
			componentConfig: map[string]any{
				"component": "vpc",
				"source": map[string]any{
					"uri":     "github.com/cloudposse/terraform-aws-vpc",
					"version": "1.0.0",
				},
			},
			expectedSource:    true,
			expectedComponent: "vpc",
			expectError:       false,
		},
		{
			name: "no source returns nil without error",
			componentConfig: map[string]any{
				"component": "vpc",
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       false,
		},
		{
			name: "source but no component returns error",
			componentConfig: map[string]any{
				"source": map[string]any{
					"uri": "github.com/cloudposse/terraform-aws-vpc",
				},
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       true,
		},
		{
			name: "invalid source type returns error",
			componentConfig: map[string]any{
				"component": "vpc",
				"source":    12345,
			},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       true,
		},
		{
			name:              "empty config",
			componentConfig:   map[string]any{},
			expectedSource:    false,
			expectedComponent: "",
			expectError:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, component, err := extractSourceAndComponent(tt.componentConfig)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectedSource {
					assert.NotNil(t, source)
					assert.Equal(t, tt.expectedComponent, component)
				} else {
					assert.Nil(t, source)
				}
			}
		})
	}
}

func TestWrapProvisionError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		explanation string
		component   string
	}{
		{
			name:        "basic error wrapping",
			err:         assert.AnError,
			explanation: "Failed to provision",
			component:   "vpc",
		},
		{
			name:        "nil error",
			err:         nil,
			explanation: "No underlying error",
			component:   "test-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapProvisionError(tt.err, tt.explanation, tt.component)
			require.Error(t, result)
			// Verify error is of expected type.
			assert.ErrorIs(t, result, errUtils.ErrSourceProvision)
		})
	}
}
