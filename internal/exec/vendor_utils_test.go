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
	assert.Contains(t, filepath.ToSlash(path), "components/packer/aws/consul", "Path should point to Packer component")
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

// TestVendorYAMLParsingWithNestedQuotes tests that YAML parsing fails when using nested double quotes
// in template functions and succeeds when using single-quoted YAML strings.
// This prevents regression of issue where {{getenv "VAR"}} inside double-quoted YAML strings causes parse errors.
func TestVendorYAMLParsingWithNestedQuotes(t *testing.T) {
	tests := []struct {
		name          string
		vendorContent string
		shouldFail    bool
		description   string
	}{
		{
			name: "double quotes inside double-quoted YAML string fails",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor
spec:
  sources:
    - source: "git::https://{{getenv "GITHUB_TOKEN"}}@github.com/test-org/test-repo.git?ref={{.Version}}"
      version: "main"
      targets:
        - "./"
`,
			shouldFail:  true,
			description: "Nested double quotes break YAML parsing - this is the reported issue",
		},
		{
			name: "single-quoted YAML string with double quotes in template succeeds",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor
spec:
  sources:
    - source: 'git::https://{{getenv "GITHUB_TOKEN"}}@github.com/test-org/test-repo.git?ref={{.Version}}'
      version: "main"
      targets:
        - "./"
`,
			shouldFail:  false,
			description: "Single-quoted YAML strings allow double quotes in templates - this is the solution",
		},
		{
			name: "double-quoted YAML string without nested quotes succeeds",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor
spec:
  sources:
    - component: "vpc"
      source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}"
      version: "1.0.0"
      targets:
        - "components/terraform/vpc"
`,
			shouldFail:  false,
			description: "Standard pattern with no nested quotes works fine",
		},
		{
			name: "YAML folded scalar with double quotes in template succeeds",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor
spec:
  sources:
    - source: >-
        git::https://{{getenv "GITHUB_TOKEN"}}@github.com/test-org/test-repo.git?ref={{.Version}}
      version: "main"
      targets:
        - "./"
`,
			shouldFail:  false,
			description: "YAML folded scalar (>-) allows double quotes in templates - alternative solution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			vendorFile := filepath.Join(tempDir, "vendor.yaml")

			err := os.WriteFile(vendorFile, []byte(tt.vendorContent), 0o644)
			require.NoError(t, err)

			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tempDir,
			}

			_, vendorConfigExists, _, err := ReadAndProcessVendorConfigFile(
				atmosConfig,
				vendorFile,
				false,
			)

			if tt.shouldFail {
				assert.Error(t, err, tt.description)
				assert.False(t, vendorConfigExists)
				if err != nil {
					assert.Contains(t, err.Error(), "yaml", "Error should mention YAML parsing issue")
				}
			} else {
				assert.NoError(t, err, tt.description)
				assert.True(t, vendorConfigExists)
			}
		})
	}
}

// TestVendorTemplateProcessingWithGetenv tests that the getenv Gomplate function works correctly
// in vendor.yaml source fields after YAML parsing.
func TestVendorTemplateProcessingWithGetenv(t *testing.T) {
	testToken := "test_github_token_12345"
	t.Setenv("GITHUB_TOKEN", testToken)

	tempDir := t.TempDir()
	vendorFile := filepath.Join(tempDir, "vendor.yaml")

	// Use single-quoted YAML string (correct syntax)
	vendorContent := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor
spec:
  sources:
    - component: "test-component"
      source: 'git::https://{{getenv "GITHUB_TOKEN"}}@github.com/test-org/test-repo.git?ref={{.Version}}'
      version: "v1.0.0"
      targets:
        - "./"
`

	err := os.WriteFile(vendorFile, []byte(vendorContent), 0o644)
	require.NoError(t, err)

	// Initialize Atmos config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	atmosConfig.BasePath = tempDir

	// Read vendor config
	vendorConfig, vendorConfigExists, _, err := ReadAndProcessVendorConfigFile(
		&atmosConfig,
		vendorFile,
		false,
	)
	require.NoError(t, err)
	require.True(t, vendorConfigExists)

	// Process the template in the source field (simulates what happens during vendor pull)
	source := vendorConfig.Spec.Sources[0]
	tmplData := struct {
		Component string
		Version   string
	}{source.Component, source.Version}

	processedURI, err := ProcessTmpl(&atmosConfig, "test-source", source.Source, tmplData, false)
	require.NoError(t, err, "Template processing should succeed")

	// Verify the template was processed correctly
	expectedURI := "git::https://" + testToken + "@github.com/test-org/test-repo.git?ref=v1.0.0"
	assert.Equal(t, expectedURI, processedURI)
	assert.Contains(t, processedURI, testToken, "Should contain the GitHub token from environment")
	assert.Contains(t, processedURI, "v1.0.0", "Should contain the version from template data")
	assert.NotContains(t, processedURI, "{{", "Should not contain unprocessed template syntax")
}

// TestVendorAutomaticTokenInjection tests that automatic token injection works correctly
// with simple URLs (no manual template-based token injection).
func TestVendorAutomaticTokenInjection(t *testing.T) {
	testToken := "ghp_test_token_automatic_injection_67890"
	t.Setenv("GITHUB_TOKEN", testToken)

	tempDir := t.TempDir()
	vendorFile := filepath.Join(tempDir, "vendor.yaml")

	// Test simple URL without manual token injection - relies on automatic injection
	vendorContent := `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test vendor automatic injection
spec:
  sources:
    - component: "vpc"
      source: "github.com/test-org/test-repo.git?ref={{.Version}}"
      version: "v2.0.0"
      targets:
        - "./"
`

	err := os.WriteFile(vendorFile, []byte(vendorContent), 0o644)
	require.NoError(t, err)

	// Initialize Atmos config
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	atmosConfig.BasePath = tempDir

	// Verify token is loaded in config
	require.NotEmpty(t, atmosConfig.Settings.GithubToken, "GitHub token should be loaded from environment")
	assert.Equal(t, testToken, atmosConfig.Settings.GithubToken, "Token should match GITHUB_TOKEN env var")

	// Read vendor config
	vendorConfig, vendorConfigExists, _, err := ReadAndProcessVendorConfigFile(
		&atmosConfig,
		vendorFile,
		false,
	)
	require.NoError(t, err)
	require.True(t, vendorConfigExists)

	// Process the template in the source field
	source := vendorConfig.Spec.Sources[0]
	tmplData := struct {
		Component string
		Version   string
	}{source.Component, source.Version}

	processedURI, err := ProcessTmpl(&atmosConfig, "test-source", source.Source, tmplData, false)
	require.NoError(t, err, "Template processing should succeed")

	// Verify version was substituted but no token in URL yet (automatic injection happens in go-getter)
	expectedURI := "github.com/test-org/test-repo.git?ref=v2.0.0"
	assert.Equal(t, expectedURI, processedURI)
	assert.Contains(t, processedURI, "v2.0.0", "Should contain the version from template data")
	assert.NotContains(t, processedURI, testToken, "Manual token should not be in URL (automatic injection happens later)")
	assert.NotContains(t, processedURI, "{{", "Should not contain unprocessed template syntax")
}

// TestVendorYAMLQuotingVariations tests different YAML quoting styles with template functions
// to ensure they all parse correctly and produce the same result.
func TestVendorYAMLQuotingVariations(t *testing.T) {
	testToken := "ghp_quoting_test_token_99999"
	t.Setenv("GITHUB_TOKEN", testToken)

	tests := []struct {
		name          string
		vendorContent string
		description   string
		expectedURI   string
	}{
		{
			name: "single-quoted YAML with correct GitHub auth format (RECOMMENDED)",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test correct format
spec:
  sources:
    - component: "test"
      source: 'git::https://x-access-token:{{getenv "GITHUB_TOKEN"}}@github.com/org/repo.git?ref={{.Version}}'
      version: "v1.0.0"
      targets: ["./"]
`,
			description: "Correct GitHub authentication format with x-access-token username",
			expectedURI: "git::https://x-access-token:" + testToken + "@github.com/org/repo.git?ref=v1.0.0",
		},
		{
			name: "single-quoted YAML with token as username (WORKS)",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test legacy format
spec:
  sources:
    - component: "test"
      source: 'git::https://{{getenv "GITHUB_TOKEN"}}@github.com/org/repo.git?ref={{.Version}}'
      version: "v1.0.0"
      targets: ["./"]
`,
			description: "Token as username - works with Git",
			expectedURI: "git::https://" + testToken + "@github.com/org/repo.git?ref=v1.0.0",
		},
		{
			name: "YAML folded scalar with correct GitHub auth format",
			vendorContent: `apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: test folded scalar
spec:
  sources:
    - component: "test"
      source: >-
        git::https://x-access-token:{{getenv "GITHUB_TOKEN"}}@github.com/org/repo.git?ref={{.Version}}
      version: "v1.0.0"
      targets: ["./"]
`,
			description: "Folded scalar (>-) with correct GitHub auth format",
			expectedURI: "git::https://x-access-token:" + testToken + "@github.com/org/repo.git?ref=v1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			vendorFile := filepath.Join(tempDir, "vendor.yaml")

			err := os.WriteFile(vendorFile, []byte(tt.vendorContent), 0o644)
			require.NoError(t, err, "Should write vendor file successfully")

			// Initialize Atmos config
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
			require.NoError(t, err, "Should initialize config")
			atmosConfig.BasePath = tempDir

			// Read and parse vendor config
			vendorConfig, vendorConfigExists, _, err := ReadAndProcessVendorConfigFile(
				&atmosConfig,
				vendorFile,
				false,
			)
			require.NoError(t, err, "YAML should parse successfully: %s", tt.description)
			require.True(t, vendorConfigExists, "Vendor config should exist")
			require.Len(t, vendorConfig.Spec.Sources, 1, "Should have one source")

			// Process templates
			source := vendorConfig.Spec.Sources[0]
			tmplData := struct {
				Component string
				Version   string
			}{source.Component, source.Version}

			processedURI, err := ProcessTmpl(&atmosConfig, "test-source", source.Source, tmplData, false)
			require.NoError(t, err, "Template processing should succeed")

			// Verify the expected URI format
			assert.Equal(t, tt.expectedURI, processedURI, tt.description)
			assert.Contains(t, processedURI, testToken, "Should contain GitHub token")
			assert.Contains(t, processedURI, "v1.0.0", "Should contain version")
			assert.NotContains(t, processedURI, "{{", "Should not have unprocessed templates")
		})
	}
}
