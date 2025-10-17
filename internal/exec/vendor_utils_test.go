package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReadAndProcessComponentVendorConfigFile(t *testing.T) {
	// Create a temporary directory for test fixtures.
	tempDir := t.TempDir()

	// Set up test component directories and config files.
	componentTypes := []struct {
		name     string
		basePath string
	}{
		{cfg.TerraformComponentType, "components/terraform"},
		{cfg.HelmfileComponentType, "components/helmfile"},
		{cfg.PackerComponentType, "components/packer"},
	}

	// Create component directories and component.yaml files.
	for _, ct := range componentTypes {
		componentDir := filepath.Join(tempDir, ct.basePath, "test-component")
		err := os.MkdirAll(componentDir, 0o755)
		require.NoError(t, err, "Failed to create directory for %s", ct.name)

		// Create a valid component.yaml file.
		componentConfig := `kind: ComponentVendorConfig
apiVersion: atmos/v1
metadata:
  name: test-component
  description: Test component for unit testing
spec:
  source:
    uri: github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.0.0
`
		configFile := filepath.Join(componentDir, "component.yaml")
		err = os.WriteFile(configFile, []byte(componentConfig), 0o644)
		require.NoError(t, err, "Failed to write component.yaml for %s", ct.name)
	}

	// Create AtmosConfiguration.
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
		name          string
		componentType string
		component     string
		expectError   bool
		expectedPath  string
	}{
		{
			name:          "terraform component type",
			componentType: cfg.TerraformComponentType,
			component:     "test-component",
			expectError:   false,
			expectedPath:  filepath.Join(tempDir, "components", "terraform", "test-component"),
		},
		{
			name:          "helmfile component type",
			componentType: cfg.HelmfileComponentType,
			component:     "test-component",
			expectError:   false,
			expectedPath:  filepath.Join(tempDir, "components", "helmfile", "test-component"),
		},
		{
			name:          "packer component type",
			componentType: cfg.PackerComponentType,
			component:     "test-component",
			expectError:   false,
			expectedPath:  filepath.Join(tempDir, "components", "packer", "test-component"),
		},
		{
			name:          "unsupported component type",
			componentType: "unsupported",
			component:     "test-component",
			expectError:   true,
			expectedPath:  "",
		},
		{
			name:          "non-existent component",
			componentType: cfg.TerraformComponentType,
			component:     "non-existent",
			expectError:   true,
			expectedPath:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, path, err := ReadAndProcessComponentVendorConfigFile(
				atmosConfig,
				tt.component,
				tt.componentType,
			)

			if tt.expectError {
				assert.Error(t, err, "Expected an error for %s", tt.name)
				assert.Empty(t, path, "Path should be empty on error")
			} else {
				assert.NoError(t, err, "Should not return error for %s", tt.name)
				assert.Equal(t, tt.expectedPath, path, "Component path mismatch")
				assert.Equal(t, "ComponentVendorConfig", config.Kind, "Config kind should match")
				assert.Equal(t, "test-component", config.Metadata.Name, "Component name should match")
				assert.Contains(t, config.Spec.Source.Uri, "github.com/cloudposse", "Source URI should be populated")
			}
		})
	}
}

func TestReadAndProcessComponentVendorConfigFile_PackerIntegration(t *testing.T) {
	// Integration test using the real Packer test fixture.
	// This complements the unit test with a real-world scenario.

	basePath := "../../tests/fixtures/scenarios/packer"
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: basePath,
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}

	// Test reading the Packer component vendor config.
	config, path, err := ReadAndProcessComponentVendorConfigFile(
		atmosConfig,
		"aws/consul",
		cfg.PackerComponentType,
	)

	require.NoError(t, err, "Should successfully read Packer component vendor config")
	assert.Equal(t, "ComponentVendorConfig", config.Kind, "Config kind should be ComponentVendorConfig")
	assert.Equal(t, "consul", config.Metadata.Name, "Component name should match")
	assert.Contains(t, config.Spec.Source.Uri, "github.com/hashicorp", "Source URI should be from hashicorp")
	assert.Contains(t, path, "components/packer/aws/consul", "Path should point to Packer component")
}

func TestNormalizeVendorURI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "triple-slash with query params converts to double-slash-dot",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "triple-slash with path and query params",
			input:    "github.com/cloudposse/terraform-aws-components.git///modules/vpc?ref=1.398.0",
			expected: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		},
		{
			name:     "double-slash pattern unchanged",
			input:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
			expected: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		},
		{
			name:     "no subdirectory pattern gets double-slash-dot added",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "OCI registry URL unchanged",
			input:    "oci://public.ecr.aws/cloudposse/terraform-aws-components:latest",
			expected: "oci://public.ecr.aws/cloudposse/terraform-aws-components:latest",
		},
		{
			name:     "local file path unchanged",
			input:    "file:///path/to/local/components",
			expected: "file:///path/to/local/components",
		},
		{
			name:     "triple-slash without query params",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.",
		},
		{
			name:     "multiple triple-slash patterns (only first is processed)",
			input:    "github.com/repo.git///path///subpath?ref=v1.0",
			expected: "github.com/repo.git//path///subpath?ref=v1.0",
		},
		{
			name:     "https scheme with triple-slash at root",
			input:    "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket.git///?ref=v5.7.0",
			expected: "https://github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "https scheme without subdirectory",
			input:    "https://github.com/cloudposse/terraform-aws-components.git?ref=v1.0.0",
			expected: "https://github.com/cloudposse/terraform-aws-components.git//.?ref=v1.0.0",
		},
		{
			name:     "git protocol with triple-slash",
			input:    "git::https://github.com/example/repo.git///?ref=main",
			expected: "git::https://github.com/example/repo.git//.?ref=main",
		},
		{
			name:     "SCP-style Git URL",
			input:    "git@github.com:cloudposse/atmos.git",
			expected: "git@github.com:cloudposse/atmos.git//.",
		},
		{
			name:     "git URL without .git extension and no subdir",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket//.?ref=v5.7.0",
		},
		{
			name:     "git URL with .git and existing double-slash-dot",
			input:    "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
			expected: "github.com/terraform-aws-modules/terraform-aws-s3-bucket.git//.?ref=v5.7.0",
		},
		{
			name:     "https git URL without subdir",
			input:    "https://github.com/cloudposse/atmos.git?ref=main",
			expected: "https://github.com/cloudposse/atmos.git//.?ref=main",
		},
		{
			name:     "git:: prefix URL without subdir",
			input:    "git::https://github.com/cloudposse/atmos.git?ref=main",
			expected: "git::https://github.com/cloudposse/atmos.git//.?ref=main",
		},
		{
			name:     "git:: prefix URL with subdir unchanged",
			input:    "git::https://github.com/cloudposse/atmos.git//examples?ref=main",
			expected: "git::https://github.com/cloudposse/atmos.git//examples?ref=main",
		},
		{
			name:     "local relative path unchanged",
			input:    "../../../components/terraform",
			expected: "../../../components/terraform",
		},
		{
			name:     "s3 URL unchanged",
			input:    "s3::https://s3.amazonaws.com/bucket/path",
			expected: "s3::https://s3.amazonaws.com/bucket/path",
		},
		{
			name:     "http URL (non-git) unchanged",
			input:    "https://example.com/archive.tar.gz",
			expected: "https://example.com/archive.tar.gz",
		},
		{
			name:     "Azure DevOps with triple-slash root",
			input:    "dev.azure.com/organization/project/_git/repository///?ref=main",
			expected: "dev.azure.com/organization/project/_git/repository//.?ref=main",
		},
		{
			name:     "Azure DevOps with triple-slash path",
			input:    "dev.azure.com/organization/project/_git/repository///terraform/modules?ref=main",
			expected: "dev.azure.com/organization/project/_git/repository//terraform/modules?ref=main",
		},
		{
			name:     "self-hosted Git with triple-slash root",
			input:    "git.company.com/team/repository.git///?ref=v1.0.0",
			expected: "git.company.com/team/repository.git//.?ref=v1.0.0",
		},
		{
			name:     "self-hosted Git with triple-slash path",
			input:    "git.company.com/team/repository.git///infrastructure/terraform?ref=v1.0.0",
			expected: "git.company.com/team/repository.git//infrastructure/terraform?ref=v1.0.0",
		},
		{
			name:     "Gitea with triple-slash root",
			input:    "gitea.company.io/owner/repo///?ref=master",
			expected: "gitea.company.io/owner/repo//.?ref=master",
		},
		{
			name:     "self-hosted without .git extension",
			input:    "git.company.com/team/repository///?ref=v1.0.0",
			expected: "git.company.com/team/repository//.?ref=v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeVendorURI(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
