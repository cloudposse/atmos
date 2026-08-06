package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
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

func TestProcessTargets_BackwardCompatible(t *testing.T) {
	// Plain string targets (no per-target version) should work exactly as before.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
		Version:   "1.398.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/vpc"},
			{Path: "components/terraform/vpc-backup"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.398.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	// Both packages should use the source-level URI and version.
	assert.Equal(t, "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0", pkgs[0].URI())
	assert.Equal(t, "1.398.0", pkgs[0].Version)
	assert.Equal(t, "vpc", pkgs[0].Name)
	assert.Equal(t, install.PkgTypeRemote, pkgs[0].PkgType())
	assert.False(t, pkgs[0].SourceIsLocalFile())
	assert.Contains(t, filepath.ToSlash(pkgs[0].Target()), "components/terraform/vpc")

	assert.Equal(t, "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0", pkgs[1].URI())
	assert.Equal(t, "1.398.0", pkgs[1].Version)
	assert.Equal(t, install.PkgTypeRemote, pkgs[1].PkgType())
	assert.Contains(t, filepath.ToSlash(pkgs[1].Target()), "components/terraform/vpc-backup")
}

func TestProcessTargets_PerTargetVersionOverride(t *testing.T) {
	// Target with its own version should re-resolve the source URI with that version.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref={{.Version}}",
		Version:   "1.398.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/vpc/{{.Version}}", Version: "2.0.0"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.398.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// URI should be re-resolved with the target's version.
	assert.Contains(t, pkgs[0].URI(), "ref=2.0.0")
	assert.NotContains(t, pkgs[0].URI(), "ref=1.398.0")
	// Version should be the target override.
	assert.Equal(t, "2.0.0", pkgs[0].Version)
	// Target path should use the target's version.
	assert.Contains(t, filepath.ToSlash(pkgs[0].Target()), "vpc/2.0.0")
	assert.Equal(t, "vpc", pkgs[0].Name)
	// Package type should be recomputed as remote.
	assert.Equal(t, install.PkgTypeRemote, pkgs[0].PkgType())
	assert.False(t, pkgs[0].SourceIsLocalFile())
}

func TestProcessTargets_MixedTargets(t *testing.T) {
	// Mix of plain targets and targets with per-target version overrides.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/cloudposse/terraform-aws-vpc.git///?ref={{.Version}}",
		Version:   "2.1.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/vpc"},
			{Path: "components/terraform/vpc/{{.Version}}", Version: "3.0.0"},
			{Path: "components/terraform/vpc-legacy"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "2.1.0"}
	// Pre-resolved URI (what processAtmosVendorSource would produce after normalizeVendorURI).
	resolvedURI := "github.com/cloudposse/terraform-aws-vpc.git//.?ref=2.1.0"

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  resolvedURI,
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 3)

	// First target: plain string, uses source-level URI and version.
	assert.Equal(t, resolvedURI, pkgs[0].URI())
	assert.Equal(t, "2.1.0", pkgs[0].Version)
	assert.Contains(t, filepath.ToSlash(pkgs[0].Target()), "components/terraform/vpc")

	// Second target: per-target version override, URI re-resolved with 3.0.0.
	assert.Contains(t, pkgs[1].URI(), "ref=3.0.0")
	assert.NotContains(t, pkgs[1].URI(), "ref=2.1.0")
	assert.Equal(t, "3.0.0", pkgs[1].Version)
	assert.Contains(t, filepath.ToSlash(pkgs[1].Target()), "vpc/3.0.0")

	// Third target: plain string, uses source-level URI and version.
	assert.Equal(t, resolvedURI, pkgs[2].URI())
	assert.Equal(t, "2.1.0", pkgs[2].Version)
	assert.Contains(t, filepath.ToSlash(pkgs[2].Target()), "components/terraform/vpc-legacy")
}

func TestProcessTargets_TargetPathTemplating(t *testing.T) {
	// Verify that {{.Component}} and {{.Version}} are expanded in target paths.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/org/repo.git//modules/vpc?ref={{.Version}}",
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/{{.Component}}/{{.Version}}"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  "github.com/org/repo.git//modules/vpc?ref=1.0.0",
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Path should have templates expanded.
	assert.Contains(t, filepath.ToSlash(pkgs[0].Target()), "components/terraform/vpc/1.0.0")
}

func TestProcessTargets_EmptyComponentFallsBackToURI(t *testing.T) {
	// When component is empty, package name should fall back to URI.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Source:  "github.com/org/repo.git//modules/vpc?ref=v1.0.0",
		Version: "v1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/vpc"},
		},
	}
	tmplData := struct{ Component, Version string }{"", "v1.0.0"}
	uri := "github.com/org/repo.git//modules/vpc?ref=v1.0.0"

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  uri,
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Name should be the URI since Component is empty.
	assert.Equal(t, uri, pkgs[0].Name)
}

func TestProcessTargets_LocalFileTarget(t *testing.T) {
	// Verify that source classification is recomputed per-target when the effective URI
	// resolves to a local file system path.
	atmosConfig := &schema.AtmosConfiguration{}
	tempDir := t.TempDir()

	// Create a local file to serve as the vendor source.
	localFile := filepath.Join(tempDir, "module.tar.gz")
	err := os.WriteFile(localFile, []byte("fake-archive"), 0o644)
	require.NoError(t, err)

	source := schema.AtmosVendorSource{
		Component: "local-mod",
		Source:    localFile,
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/local-mod"},
		},
	}
	tmplData := struct{ Component, Version string }{"local-mod", "1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  localFile,
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeLocal,
		SourceIsLocalFile:    true,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Source classification should detect the local file.
	assert.Equal(t, install.PkgTypeLocal, pkgs[0].PkgType())
	assert.True(t, pkgs[0].SourceIsLocalFile())
}

func TestProcessTargets_PerTargetVersionRecomputesClassification(t *testing.T) {
	// When a target has a version override, source classification should be recomputed
	// from the re-resolved URI. This verifies the fix for the issue where pkgType and
	// sourceIsLocalFile were only computed once at the source level.
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/org/terraform-aws-vpc.git//modules/vpc?ref={{.Version}}",
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			// First target: no override, uses source-level classification.
			{Path: "components/terraform/vpc"},
			// Second target: version override, URI should be re-resolved and reclassified.
			{Path: "components/terraform/vpc/{{.Version}}", Version: "2.0.0"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  "github.com/org/terraform-aws-vpc.git//modules/vpc?ref=1.0.0",
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	// First target: uses source-level defaults.
	assert.Equal(t, install.PkgTypeRemote, pkgs[0].PkgType())
	assert.False(t, pkgs[0].SourceIsLocalFile())
	assert.Equal(t, "1.0.0", pkgs[0].Version)

	// Second target: version override, classification recomputed (still remote in this case).
	assert.Equal(t, install.PkgTypeRemote, pkgs[1].PkgType())
	assert.False(t, pkgs[1].SourceIsLocalFile())
	assert.Equal(t, "2.0.0", pkgs[1].Version)
	assert.Contains(t, pkgs[1].URI(), "ref=2.0.0")
	assert.Contains(t, filepath.ToSlash(pkgs[1].Target()), "vpc/2.0.0")
}

// fakeTagLister is a version.RemoteLister returning canned tags for unit tests, letting
// vendor_utils_test.go exercise a semver-range `version:`'s end-to-end resolution (source-URI and
// target-path templating alike) without any real network access.
type fakeTagLister struct {
	tags []string
}

func (f *fakeTagLister) ListTags(context.Context, string) ([]string, error) {
	return f.tags, nil
}

// TestProcessAtmosVendorSourceEntry_SemverRangeResolvesAndTemplatesConcreteVersion is the positive
// end-to-end counterpart to the non-Git-source error tests below: a source-level `version:` that is
// a semver range resolves (via the injected fakeTagLister, so no real network access) to a concrete
// version, that concrete version -- not the literal range string -- is what's templated into both
// the source URI and the target path, and the resolution is recorded in vendor.lock.yaml.
func TestProcessAtmosVendorSourceEntry_SemverRangeResolvesAndTemplatesConcreteVersion(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	lister := &fakeTagLister{tags: []string{"v1.0.0", "v1.2.3", "v1.5.0", "v2.0.0"}}

	params := &vendorSourceParams{
		atmosConfig: atmosConfig,
		sources: []schema.AtmosVendorSource{
			{
				Component: "vpc",
				Source:    "github.com/cloudposse/terraform-aws-vpc.git//modules/vpc?ref={{.Version}}",
				Version:   "^1.0.0",
				Targets:   schema.AtmosVendorTargets{{Path: "components/terraform/vpc/{{.Version}}"}},
			},
		},
		vendorConfigFileName: "vendor.yaml",
		lister:               lister,
	}

	pkgs, skip, err := processAtmosVendorSourceEntry(params, 0)

	require.NoError(t, err)
	require.False(t, skip)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "v1.5.0", pkgs[0].Version)
	assert.Equal(t, "^1.0.0", pkgs[0].RawVersion)
	assert.Contains(t, pkgs[0].URI(), "ref=v1.5.0")
	assert.NotContains(t, pkgs[0].URI(), "^1.0.0")
	assert.Contains(t, filepath.ToSlash(pkgs[0].Target()), "vpc/v1.5.0")

	// The resolution is recorded in vendor.lock.yaml, so a second call with an unchanged range
	// reuses it (proven at the pkg/vendoring/install layer's dedicated cache-reuse test; here we
	// only confirm the entry lands as expected).
	lock, err := lockfile.Load(atmosConfig)
	require.NoError(t, err)
	found := false
	for _, artifact := range lock.Artifacts {
		if artifact.Source.VersionConstraint == "^1.0.0" {
			found = true
			assert.Equal(t, "v1.5.0", artifact.Source.ResolvedVersion)
		}
	}
	assert.True(t, found, "expected a version-range resolution artifact in vendor.lock.yaml")
}

// TestProcessTargets_PerTargetSemverRangeResolvesAndTemplatesConcreteVersion proves a per-target
// `targets[].version` override that is a semver range resolves independently of the source-level
// version and templates its own resolved concrete version into that target's URI/path.
func TestProcessTargets_PerTargetSemverRangeResolvesAndTemplatesConcreteVersion(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	lister := &fakeTagLister{tags: []string{"v1.0.0", "v1.5.0", "v2.0.0", "v2.5.0"}}

	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/cloudposse/terraform-aws-vpc.git//modules/vpc?ref={{.Version}}",
		Version:   "v1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/vpc"},
			{Path: "components/terraform/vpc/{{.Version}}", Version: "^2.0.0"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "v1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  "github.com/cloudposse/terraform-aws-vpc.git//modules/vpc?ref=v1.0.0",
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeRemote,
		SourceIsLocalFile:    false,
		Lister:               lister,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	// First target: no override, source-level exact pin unaffected.
	assert.Equal(t, "v1.0.0", pkgs[0].Version)
	assert.Empty(t, pkgs[0].RawVersion)

	// Second target: range override resolves independently to the highest tag satisfying ^2.0.0.
	assert.Equal(t, "v2.5.0", pkgs[1].Version)
	assert.Equal(t, "^2.0.0", pkgs[1].RawVersion)
	assert.Contains(t, pkgs[1].URI(), "ref=v2.5.0")
	assert.Contains(t, filepath.ToSlash(pkgs[1].Target()), "vpc/v2.5.0")
}

// TestProcessTargets_PerTargetSemverRangeOnNonGitSourceReturnsClearError proves a per-target
// version override that is a semver range (e.g. "^2.0.0") on a source with no tag-listing
// mechanism (a local file here) fails resolveTargetOverride with a clear, wrapped error instead of
// silently templating the literal range string into the URI or hanging on a network call.
func TestProcessTargets_PerTargetSemverRangeOnNonGitSourceReturnsClearError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	localFile := filepath.Join(t.TempDir(), "module.tar.gz")
	require.NoError(t, os.WriteFile(localFile, []byte("fake-archive"), 0o644))

	source := schema.AtmosVendorSource{
		Component: "local-mod",
		Source:    localFile,
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/local-mod", Version: "^2.0.0"},
		},
	}
	tmplData := struct{ Component, Version string }{"local-mod", "1.0.0"}

	_, err := processTargets(&processTargetsParams{
		AtmosConfig:          atmosConfig,
		IndexSource:          0,
		Source:               &source,
		TemplateData:         tmplData,
		VendorConfigFilePath: "",
		URI:                  localFile,
		SourceTemplate:       source.Source,
		PkgType:              install.PkgTypeLocal,
		SourceIsLocalFile:    true,
	})

	require.Error(t, err)
	require.ErrorIs(t, err, install.ErrVersionRangeRequiresGitSource)
}

// TestProcessAtmosVendorSourceEntry_SemverRangeOnNonGitSourceReturnsClearError proves the
// source-level (not per-target) semver-range resolution path -- exercised earlier, inside
// processAtmosVendorSourceEntry, before any URI templating -- surfaces the same clear error for a
// non-Git source, network-free (ResolveDeclaredVersion never reaches the Lister for a non-Git
// source, so this never touches the network despite using the real, unmocked default Lister).
func TestProcessAtmosVendorSourceEntry_SemverRangeOnNonGitSourceReturnsClearError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	localFile := filepath.Join(t.TempDir(), "module.tar.gz")
	require.NoError(t, os.WriteFile(localFile, []byte("fake-archive"), 0o644))

	params := &vendorSourceParams{
		atmosConfig: atmosConfig,
		sources: []schema.AtmosVendorSource{
			{
				Component: "local-mod",
				Source:    localFile,
				Version:   "^1.0.0",
				Targets:   schema.AtmosVendorTargets{{Path: "components/terraform/local-mod"}},
			},
		},
		vendorConfigFileName: "vendor.yaml",
		vendorConfigFilePath: "",
	}

	_, _, err := processAtmosVendorSourceEntry(params, 0)

	require.Error(t, err)
	require.ErrorIs(t, err, install.ErrVersionRangeRequiresGitSource)
}

// TestValidateSourceFields_SemverRangeConflictsWithConstraintsVersion proves a vendor.yaml source
// combining a semver-range version: with a constraints.version ceiling is rejected at
// validateSourceFields -- before URI templating, resolution, or any network access is attempted.
func TestValidateSourceFields_SemverRangeConflictsWithConstraintsVersion(t *testing.T) {
	s := &schema.AtmosVendorSource{
		Component:   "vpc",
		Source:      "github.com/cloudposse/terraform-aws-vpc.git?ref={{.Version}}",
		Version:     "^1.0.0",
		Targets:     schema.AtmosVendorTargets{{Path: "components/terraform/vpc"}},
		Constraints: &schema.VendorConstraints{Version: "<2.0.0"},
	}

	err := validateSourceFields(s, "vendor.yaml")

	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrVersionRangeConflictsWithConstraints)
}

// TestValidateSourceFields_SemverRangeWithExcludedVersionsAndNoPrereleasesIsAccepted proves
// constraints.excluded_versions/no_prereleases (unlike constraints.version) remain valid alongside
// a range-declared version:.
func TestValidateSourceFields_SemverRangeWithExcludedVersionsAndNoPrereleasesIsAccepted(t *testing.T) {
	s := &schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/cloudposse/terraform-aws-vpc.git?ref={{.Version}}",
		Version:   "^1.0.0",
		Targets:   schema.AtmosVendorTargets{{Path: "components/terraform/vpc"}},
		Constraints: &schema.VendorConstraints{
			ExcludedVersions: []string{"1.9.0"},
			NoPrereleases:    true,
		},
	}

	require.NoError(t, validateSourceFields(s, "vendor.yaml"))
}

func TestValidateTagsAndComponents(t *testing.T) {
	sources := []schema.AtmosVendorSource{
		{Component: "vpc", Tags: []string{"network"}},
		{Component: "eks", Tags: []string{"compute"}},
	}

	tests := []struct {
		name      string
		sources   []schema.AtmosVendorSource
		component string
		tags      []string
		wantErr   error
	}{
		{
			name:      "requested component is defined",
			sources:   sources,
			component: "vpc",
			wantErr:   nil,
		},
		{
			name:      "requested component is not defined",
			sources:   sources,
			component: "missing",
			wantErr:   ErrComponentNotDefined,
		},
		{
			name:    "tag matches at least one source",
			sources: sources,
			tags:    []string{"network"},
			wantErr: nil,
		},
		{
			name:    "no source matches the requested tags",
			sources: sources,
			tags:    []string{"nonexistent"},
			wantErr: ErrNoComponentsWithTags,
		},
		{
			name:    "duplicate component names are rejected",
			sources: []schema.AtmosVendorSource{{Component: "dup"}, {Component: "dup"}},
			wantErr: ErrDuplicateComponents,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTagsAndComponents(tt.sources, "vendor.yaml", tt.component, tt.tags)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestFilterMaterializedVendorPackages_SkipsMaterializedPackage proves a package with an existing,
// matching vendor lock receipt is filtered out (skipped), while a sibling package with no receipt
// yet is kept pending.
func TestFilterMaterializedVendorPackages_SkipsMaterializedPackage(t *testing.T) {
	base := t.TempDir()
	sourceDir := filepath.Join(base, "source")
	require.NoError(t, os.MkdirAll(sourceDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("resource"), 0o644))

	config := &schema.AtmosConfiguration{BasePath: base}
	materialized := install.NewAtmosVendorPackage(&install.AtmosPackageParams{
		Name: "vpc-materialized", URI: sourceDir, TargetPath: filepath.Join(base, "target"), PkgType: install.PkgTypeLocal,
	})
	result, err := install.Install(config, materialized, install.InstallOptions{})
	require.NoError(t, err)
	require.NoError(t, result.Err)

	pending := install.NewAtmosVendorPackage(&install.AtmosPackageParams{
		Name: "vpc-pending", URI: sourceDir, TargetPath: filepath.Join(base, "pending-target"), PkgType: install.PkgTypeLocal,
	})

	filtered, err := install.FilterPending(config, []install.VendorPackage{materialized, pending}, install.InstallOptions{})

	require.NoError(t, err)
	require.Len(t, filtered, 1, "only the not-yet-materialized package should remain")
	assert.Equal(t, "vpc-pending", filtered[0].Name)
}

// TestFilterMaterializedVendorPackages_PropagatesError proves a package whose targetPath can't be
// related back to the project's BasePath surfaces the vendor lock verification error, naming the
// failing package, instead of silently treating it as pending.
func TestFilterMaterializedVendorPackages_PropagatesError(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	pkgs := []install.VendorPackage{
		install.NewAtmosVendorPackage(&install.AtmosPackageParams{Name: "vpc", URI: "source", TargetPath: t.TempDir(), PkgType: install.PkgTypeLocal}),
	}

	result, err := install.FilterPending(config, pkgs, install.InstallOptions{})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "verify vendor lock for vpc")
}

// TestExecuteAtmosVendorInternal_PropagatesMaterializationCheckError proves ExecuteAtmosVendorInternal
// surfaces the vendor lock verification error (rather than swallowing it or proceeding to install)
// when a resolved target can't be related back to the project's BasePath.
func TestExecuteAtmosVendorInternal_PropagatesMaterializationCheckError(t *testing.T) {
	vendorDir := t.TempDir()
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	// atmosConfig.BasePath deliberately points elsewhere from vendorDir, so the resolved target
	// (joined against vendorDir) can't be related back to it.
	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}

	opts := &executeVendorOptions{
		vendorConfigFileName: filepath.Join(vendorDir, "vendor.yaml"),
		atmosConfig:          atmosConfig,
		atmosVendorSpec: schema.AtmosVendorSpec{
			Sources: []schema.AtmosVendorSource{
				{
					Component: "vpc",
					Source:    sourceDir,
					Targets:   schema.AtmosVendorTargets{{Path: "components/terraform/vpc"}},
				},
			},
		},
	}

	err := ExecuteAtmosVendorInternal(opts)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "verify vendor lock for vpc")
}

// TestExecuteAtmosVendorInternal_AllMaterialized_NoOp proves a vendor.yaml whose only source is
// already vendored and unchanged filters down to zero packages and returns without error, instead
// of calling executeVendorModel with an empty list or re-copying the source.
func TestExecuteAtmosVendorInternal_AllMaterialized_NoOp(t *testing.T) {
	vendorDir := t.TempDir()
	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# vpc\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{BasePath: vendorDir}
	newOpts := func() *executeVendorOptions {
		return &executeVendorOptions{
			vendorConfigFileName: filepath.Join(vendorDir, "vendor.yaml"),
			atmosConfig:          atmosConfig,
			atmosVendorSpec: schema.AtmosVendorSpec{
				Sources: []schema.AtmosVendorSource{
					{
						Component: "vpc",
						Source:    sourceDir,
						Targets:   schema.AtmosVendorTargets{{Path: "components/terraform/vpc"}},
					},
				},
			},
		}
	}

	require.NoError(t, ExecuteAtmosVendorInternal(newOpts()))
	target := filepath.Join(vendorDir, "components", "terraform", "vpc")
	assert.FileExists(t, filepath.Join(target, "main.tf"))

	// Add a new file to the source after the first pull; if the second call re-pulls instead of
	// skipping the already-materialized source, this file would show up too.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "extra.tf"), []byte("# added later\n"), 0o644))

	err := ExecuteAtmosVendorInternal(newOpts())
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(target, "extra.tf"), "an already-materialized source must be skipped, not re-copied")
}
