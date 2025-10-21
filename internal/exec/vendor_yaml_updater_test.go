package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestUpdateYAMLVersion(t *testing.T) {
	tests := []struct {
		name          string
		inputYAML     string
		componentName string
		newVersion    string
		expectError   bool
	}{
		{
			name: "update version in simple vendor.yaml",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			componentName: "vpc",
			newVersion:    "1.2.3",
			expectError:   false,
		},
		{
			name: "update version with comments preserved",
			inputYAML: `# Vendor configuration
apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    # VPC component
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0  # current stable version
      targets:
        - components/terraform/vpc
`,
			componentName: "vpc",
			newVersion:    "2.0.0",
			expectError:   false,
		},
		{
			name: "update specific component in multi-component config",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
    - component: eks
      source: github.com/cloudposse/terraform-aws-components
      version: 2.0.0
      targets:
        - components/terraform/eks
`,
			componentName: "eks",
			newVersion:    "2.5.0",
			expectError:   false,
		},
		{
			name: "component not found",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			componentName: "nonexistent",
			newVersion:    "1.0.0",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tempDir := t.TempDir()
			tempFile := filepath.Join(tempDir, "vendor.yaml")
			err := os.WriteFile(tempFile, []byte(tt.inputYAML), 0o644)
			require.NoError(t, err)

			// Update version
			atmosConfig := &schema.AtmosConfiguration{}
			err = updateYAMLVersion(atmosConfig, tempFile, tt.componentName, tt.newVersion)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Read back and verify version was updated
			version, err := findComponentVersion(atmosConfig, tempFile, tt.componentName)
			require.NoError(t, err)
			assert.Equal(t, tt.newVersion, version)
		})
	}
}

func TestFindComponentVersion(t *testing.T) {
	tests := []struct {
		name            string
		inputYAML       string
		componentName   string
		expectedVersion string
		expectError     bool
	}{
		{
			name: "find version in simple config",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.2.3
      targets:
        - components/terraform/vpc
`,
			componentName:   "vpc",
			expectedVersion: "1.2.3",
			expectError:     false,
		},
		{
			name: "find version in multi-component config",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
    - component: eks
      source: github.com/cloudposse/terraform-aws-components
      version: 2.5.0
      targets:
        - components/terraform/eks
`,
			componentName:   "eks",
			expectedVersion: "2.5.0",
			expectError:     false,
		},
		{
			name: "component not found",
			inputYAML: `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-components
      version: 1.0.0
      targets:
        - components/terraform/vpc
`,
			componentName: "nonexistent",
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tempDir := t.TempDir()
			tempFile := filepath.Join(tempDir, "vendor.yaml")
			err := os.WriteFile(tempFile, []byte(tt.inputYAML), 0o644)
			require.NoError(t, err)

			// Find version
			atmosConfig := &schema.AtmosConfiguration{}
			version, err := findComponentVersion(atmosConfig, tempFile, tt.componentName)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedVersion, version)
		})
	}
}
