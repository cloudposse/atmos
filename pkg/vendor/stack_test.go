package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestResolveComponentPath(t *testing.T) {
	tests := []struct {
		name          string
		componentName string
		componentData any
		expected      string
	}{
		{
			name:          "non-map data returns component name",
			componentName: "vpc",
			componentData: "string-value",
			expected:      "vpc",
		},
		{
			name:          "nil data returns component name",
			componentName: "vpc",
			componentData: nil,
			expected:      "vpc",
		},
		{
			name:          "empty map returns component name",
			componentName: "vpc",
			componentData: map[string]any{},
			expected:      "vpc",
		},
		{
			name:          "map without metadata returns component name",
			componentName: "vpc",
			componentData: map[string]any{
				"vars": map[string]any{"key": "value"},
			},
			expected: "vpc",
		},
		{
			name:          "map with empty metadata returns component name",
			componentName: "vpc",
			componentData: map[string]any{
				"metadata": map[string]any{},
			},
			expected: "vpc",
		},
		{
			name:          "map with metadata.component set",
			componentName: "vpc/us-east-1",
			componentData: map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
			expected: "vpc",
		},
		{
			name:          "map with empty metadata.component returns component name",
			componentName: "vpc/us-east-1",
			componentData: map[string]any{
				"metadata": map[string]any{
					"component": "",
				},
			},
			expected: "vpc/us-east-1",
		},
		{
			name:          "metadata is not a map returns component name",
			componentName: "vpc",
			componentData: map[string]any{
				"metadata": "not-a-map",
			},
			expected: "vpc",
		},
		{
			name:          "metadata.component is not a string returns component name",
			componentName: "vpc",
			componentData: map[string]any{
				"metadata": map[string]any{
					"component": 123,
				},
			},
			expected: "vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveComponentPath(tt.componentName, tt.componentData)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractVendorableComponents(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test component directory with component.yaml.
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	componentConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(componentDir, "component.yaml"), []byte(componentConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}

	tests := []struct {
		name           string
		stacksMap      map[string]any
		expectedCount  int
		expectedSkip   int
		expectError    bool
		setupComponent bool
	}{
		{
			name:           "empty stacks map",
			stacksMap:      map[string]any{},
			expectedCount:  0,
			expectedSkip:   0,
			setupComponent: false,
		},
		{
			name: "stack without components key",
			stacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"vars": map[string]any{"region": "us-east-1"},
				},
			},
			expectedCount:  0,
			expectedSkip:   0,
			setupComponent: false,
		},
		{
			name: "stack with non-map components",
			stacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": "not-a-map",
				},
			},
			expectedCount:  0,
			expectedSkip:   0,
			setupComponent: false,
		},
		{
			name: "stack with terraform component with vendor config",
			stacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"vars": map[string]any{"region": "us-east-1"},
							},
						},
					},
				},
			},
			expectedCount:  1,
			expectedSkip:   0,
			setupComponent: true,
		},
		{
			name: "stack with component missing vendor config",
			stacksMap: map[string]any{
				"dev-us-east-1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"non-existent-component": map[string]any{
								"vars": map[string]any{"region": "us-east-1"},
							},
						},
					},
				},
			},
			expectedCount:  0,
			expectedSkip:   1,
			setupComponent: false,
		},
		{
			name: "non-map stack data is skipped",
			stacksMap: map[string]any{
				"dev-us-east-1": "not-a-map",
			},
			expectedCount:  0,
			expectedSkip:   0,
			setupComponent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, skipped, err := extractVendorableComponents(atmosConfig, tt.stacksMap)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, packages, tt.expectedCount)
				assert.Equal(t, tt.expectedSkip, skipped)
			}
		})
	}
}

func TestProcessStackComponents(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directories.
	terraformDir := filepath.Join(tempDir, "components", "terraform")
	helmfileDir := filepath.Join(tempDir, "components", "helmfile")
	packerDir := filepath.Join(tempDir, "components", "packer")

	err := os.MkdirAll(terraformDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(helmfileDir, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(packerDir, 0o755)
	require.NoError(t, err)

	// Create a terraform component with valid vendor config.
	vpcDir := filepath.Join(terraformDir, "vpc")
	err = os.MkdirAll(vpcDir, 0o755)
	require.NoError(t, err)

	validConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(vpcDir, "component.yaml"), []byte(validConfig), 0o644)
	require.NoError(t, err)

	// Create a component with invalid kind.
	invalidKindDir := filepath.Join(terraformDir, "invalid-kind")
	err = os.MkdirAll(invalidKindDir, 0o755)
	require.NoError(t, err)

	invalidKindConfig := `kind: SomeOtherKind
apiVersion: atmos/v1
metadata:
  name: invalid-kind
spec:
  source:
    uri: github.com/example/repo.git
`
	err = os.WriteFile(filepath.Join(invalidKindDir, "component.yaml"), []byte(invalidKindConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	tests := []struct {
		name            string
		components      map[string]any
		componentType   string
		expectedCount   int
		expectedSkipped int
		expectError     bool
	}{
		{
			name: "terraform component with valid config",
			components: map[string]any{
				"vpc": map[string]any{
					"vars": map[string]any{"key": "value"},
				},
			},
			componentType:   cfg.TerraformComponentType,
			expectedCount:   1,
			expectedSkipped: 0,
		},
		{
			name: "component without vendor config skipped",
			components: map[string]any{
				"no-vendor-config": map[string]any{
					"vars": map[string]any{"key": "value"},
				},
			},
			componentType:   cfg.TerraformComponentType,
			expectedCount:   0,
			expectedSkipped: 1,
		},
		{
			name: "component with invalid kind skipped",
			components: map[string]any{
				"invalid-kind": map[string]any{
					"vars": map[string]any{"key": "value"},
				},
			},
			componentType:   cfg.TerraformComponentType,
			expectedCount:   0,
			expectedSkipped: 1,
		},
		{
			name: "helmfile component type uses helmfile base path",
			components: map[string]any{
				"no-config": map[string]any{},
			},
			componentType:   cfg.HelmfileComponentType,
			expectedCount:   0,
			expectedSkipped: 1,
		},
		{
			name: "packer component type uses packer base path",
			components: map[string]any{
				"no-config": map[string]any{},
			},
			componentType:   cfg.PackerComponentType,
			expectedCount:   0,
			expectedSkipped: 1,
		},
		{
			name: "unknown component type defaults to terraform",
			components: map[string]any{
				"vpc": map[string]any{},
			},
			componentType:   "unknown",
			expectedCount:   1,
			expectedSkipped: 0,
		},
		{
			name: "duplicate component processed only once",
			components: map[string]any{
				"vpc": map[string]any{},
			},
			componentType:   cfg.TerraformComponentType,
			expectedCount:   1,
			expectedSkipped: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processedComponents := make(map[string]bool)
			packages, skipped, err := processStackComponents(
				atmosConfig,
				"test-stack",
				tt.components,
				tt.componentType,
				processedComponents,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, packages, tt.expectedCount)
				assert.Equal(t, tt.expectedSkipped, skipped)
			}
		})
	}
}

func TestProcessStackComponents_DuplicateHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directory with vendor config.
	vpcDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(vpcDir, 0o755)
	require.NoError(t, err)

	validConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(vpcDir, "component.yaml"), []byte(validConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Process the same component twice.
	processedComponents := make(map[string]bool)

	// First call should process the component.
	packages1, _, err := processStackComponents(
		atmosConfig,
		"stack1",
		map[string]any{"vpc": map[string]any{}},
		cfg.TerraformComponentType,
		processedComponents,
	)
	require.NoError(t, err)
	assert.Len(t, packages1, 1)

	// Second call should skip (already processed).
	packages2, _, err := processStackComponents(
		atmosConfig,
		"stack2",
		map[string]any{"vpc": map[string]any{}},
		cfg.TerraformComponentType,
		processedComponents,
	)
	require.NoError(t, err)
	assert.Len(t, packages2, 0)
}

func TestProcessStackComponents_MetadataComponent(t *testing.T) {
	tempDir := t.TempDir()

	// Create component directory with vendor config.
	vpcDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	err := os.MkdirAll(vpcDir, 0o755)
	require.NoError(t, err)

	validConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: vpc
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
	err = os.WriteFile(filepath.Join(vpcDir, "component.yaml"), []byte(validConfig), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Test with metadata.component set.
	processedComponents := make(map[string]bool)
	packages, _, err := processStackComponents(
		atmosConfig,
		"test-stack",
		map[string]any{
			"vpc-us-east-1": map[string]any{
				"metadata": map[string]any{
					"component": "vpc",
				},
			},
		},
		cfg.TerraformComponentType,
		processedComponents,
	)
	require.NoError(t, err)
	assert.Len(t, packages, 1)
	// The package should use the resolved component path "vpc", not "vpc-us-east-1".
	assert.Equal(t, "vpc", packages[0].name)
}

func TestCreateComponentPackages(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	tests := []struct {
		name           string
		componentName  string
		componentPath  string
		spec           *schema.VendorComponentSpec
		expectedCount  int
		expectError    bool
		checkFirstPkg  func(t *testing.T, pkg pkgComponentVendor)
	}{
		{
			name:          "empty URI returns nil",
			componentName: "vpc",
			componentPath: filepath.Join(tempDir, "components", "terraform", "vpc"),
			spec: &schema.VendorComponentSpec{
				Source: schema.VendorComponentSource{
					Uri: "",
				},
			},
			expectedCount: 0,
		},
		{
			name:          "simple remote URI",
			componentName: "vpc",
			componentPath: filepath.Join(tempDir, "components", "terraform", "vpc"),
			spec: &schema.VendorComponentSpec{
				Source: schema.VendorComponentSource{
					Uri:     "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
					Version: "1.0.0",
				},
			},
			expectedCount: 1,
			checkFirstPkg: func(t *testing.T, pkg pkgComponentVendor) {
				assert.True(t, pkg.IsComponent)
				assert.Equal(t, pkgTypeRemote, pkg.pkgType)
				assert.Equal(t, "vpc", pkg.name)
				assert.Equal(t, "1.0.0", pkg.version)
			},
		},
		{
			name:          "OCI scheme URI",
			componentName: "vpc",
			componentPath: filepath.Join(tempDir, "components", "terraform", "vpc"),
			spec: &schema.VendorComponentSpec{
				Source: schema.VendorComponentSource{
					Uri:     "oci://public.ecr.aws/cloudposse/components/terraform:latest",
					Version: "latest",
				},
			},
			expectedCount: 1,
			checkFirstPkg: func(t *testing.T, pkg pkgComponentVendor) {
				assert.Equal(t, pkgTypeOci, pkg.pkgType)
				// OCI scheme should be stripped.
				assert.NotContains(t, pkg.uri, "oci://")
			},
		},
		{
			name:          "with mixins",
			componentName: "vpc",
			componentPath: filepath.Join(tempDir, "components", "terraform", "vpc"),
			spec: &schema.VendorComponentSpec{
				Source: schema.VendorComponentSource{
					Uri:     "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0",
					Version: "1.0.0",
				},
				Mixins: []schema.VendorComponentMixins{
					{
						Uri:      "github.com/cloudposse/mixins.git//context.tf?ref=1.0.0",
						Filename: "context.tf",
						Version:  "1.0.0",
					},
				},
			},
			expectedCount: 2, // 1 component + 1 mixin
			checkFirstPkg: func(t *testing.T, pkg pkgComponentVendor) {
				assert.True(t, pkg.IsComponent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packages, err := createComponentPackages(
				atmosConfig,
				tt.componentName,
				tt.componentPath,
				tt.spec,
				cfg.TerraformComponentType,
			)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, packages, tt.expectedCount)
				if tt.checkFirstPkg != nil && len(packages) > 0 {
					tt.checkFirstPkg(t, packages[0])
				}
			}
		})
	}
}

func TestCreateComponentPackages_MixinErrors(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	// Test missing mixin URI.
	_, err := createComponentPackages(
		atmosConfig,
		"vpc",
		filepath.Join(tempDir, "vpc"),
		&schema.VendorComponentSpec{
			Source: schema.VendorComponentSource{
				Uri: "github.com/example/repo.git",
			},
			Mixins: []schema.VendorComponentMixins{
				{
					Uri:      "",
					Filename: "context.tf",
				},
			},
		},
		cfg.TerraformComponentType,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingMixinURI)

	// Test missing mixin filename.
	_, err = createComponentPackages(
		atmosConfig,
		"vpc",
		filepath.Join(tempDir, "vpc"),
		&schema.VendorComponentSpec{
			Source: schema.VendorComponentSource{
				Uri: "github.com/example/repo.git",
			},
			Mixins: []schema.VendorComponentMixins{
				{
					Uri:      "github.com/example/mixin.git",
					Filename: "",
				},
			},
		},
		cfg.TerraformComponentType,
	)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingMixinFilename)
}
