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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	// Both packages should use the source-level URI and version.
	assert.Equal(t, "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0", pkgs[0].uri)
	assert.Equal(t, "1.398.0", pkgs[0].version)
	assert.Equal(t, "vpc", pkgs[0].name)
	assert.Equal(t, pkgTypeRemote, pkgs[0].pkgType)
	assert.False(t, pkgs[0].sourceIsLocalFile)
	assert.Contains(t, filepath.ToSlash(pkgs[0].targetPath), "components/terraform/vpc")

	assert.Equal(t, "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0", pkgs[1].uri)
	assert.Equal(t, "1.398.0", pkgs[1].version)
	assert.Equal(t, pkgTypeRemote, pkgs[1].pkgType)
	assert.Contains(t, filepath.ToSlash(pkgs[1].targetPath), "components/terraform/vpc-backup")
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.398.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// URI should be re-resolved with the target's version.
	assert.Contains(t, pkgs[0].uri, "ref=2.0.0")
	assert.NotContains(t, pkgs[0].uri, "ref=1.398.0")
	// Version should be the target override.
	assert.Equal(t, "2.0.0", pkgs[0].version)
	// Target path should use the target's version.
	assert.Contains(t, filepath.ToSlash(pkgs[0].targetPath), "vpc/2.0.0")
	assert.Equal(t, "vpc", pkgs[0].name)
	// Package type should be recomputed as remote.
	assert.Equal(t, pkgTypeRemote, pkgs[0].pkgType)
	assert.False(t, pkgs[0].sourceIsLocalFile)
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               resolvedURI,
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 3)

	// First target: plain string, uses source-level URI and version.
	assert.Equal(t, resolvedURI, pkgs[0].uri)
	assert.Equal(t, "2.1.0", pkgs[0].version)
	assert.Contains(t, filepath.ToSlash(pkgs[0].targetPath), "components/terraform/vpc")

	// Second target: per-target version override, URI re-resolved with 3.0.0.
	assert.Contains(t, pkgs[1].uri, "ref=3.0.0")
	assert.NotContains(t, pkgs[1].uri, "ref=2.1.0")
	assert.Equal(t, "3.0.0", pkgs[1].version)
	assert.Contains(t, filepath.ToSlash(pkgs[1].targetPath), "vpc/3.0.0")

	// Third target: plain string, uses source-level URI and version.
	assert.Equal(t, resolvedURI, pkgs[2].uri)
	assert.Equal(t, "2.1.0", pkgs[2].version)
	assert.Contains(t, filepath.ToSlash(pkgs[2].targetPath), "components/terraform/vpc-legacy")
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               "github.com/org/repo.git//modules/vpc?ref=1.0.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Path should have templates expanded.
	assert.Contains(t, filepath.ToSlash(pkgs[0].targetPath), "components/terraform/vpc/1.0.0")
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               uri,
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Name should be the URI since Component is empty.
	assert.Equal(t, uri, pkgs[0].name)
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               localFile,
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeLocal,
		SourceIsLocalFile: true,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)

	// Source classification should detect the local file.
	assert.Equal(t, pkgTypeLocal, pkgs[0].pkgType)
	assert.True(t, pkgs[0].sourceIsLocalFile)
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
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    "",
		URI:               "github.com/org/terraform-aws-vpc.git//modules/vpc?ref=1.0.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	// First target: uses source-level defaults.
	assert.Equal(t, pkgTypeRemote, pkgs[0].pkgType)
	assert.False(t, pkgs[0].sourceIsLocalFile)
	assert.Equal(t, "1.0.0", pkgs[0].version)

	// Second target: version override, classification recomputed (still remote in this case).
	assert.Equal(t, pkgTypeRemote, pkgs[1].pkgType)
	assert.False(t, pkgs[1].sourceIsLocalFile)
	assert.Equal(t, "2.0.0", pkgs[1].version)
	assert.Contains(t, pkgs[1].uri, "ref=2.0.0")
	assert.Contains(t, filepath.ToSlash(pkgs[1].targetPath), "vpc/2.0.0")
}

// Compile-time sentinels for the schema fields this test file depends on: a rename of either
// `Vendor.BasePath` or `BasePath` must fail the build rather than silently skip the new behavior.
var (
	_ = schema.AtmosConfiguration{BasePath: ""}
	_ = schema.Vendor{BasePath: ""}
)

// TestResolveVendorTargetBasePath covers which root relative `targets` are anchored to, depending on
// whether atmos.yaml declares the vendor manifest location via `vendor.base_path`.
func TestResolveVendorTargetBasePath(t *testing.T) {
	tests := []struct {
		name                 string
		atmosConfig          *schema.AtmosConfiguration
		vendorConfigFileName string
		expected             string
	}{
		{
			// The reported bug: with `vendor.base_path` pointing at a manifest in a config
			// directory, relative targets must resolve against the Atmos `base_path`, not against
			// the directory holding vendor.yaml.
			name: "vendor.base_path configured resolves targets against the Atmos base path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.Join("repo", "root"),
				Vendor:   schema.Vendor{BasePath: filepath.Join("config", "vendor.yaml")},
			},
			vendorConfigFileName: filepath.Join("repo", "root", "config", "vendor.yaml"),
			expected:             filepath.Join("repo", "root"),
		},
		{
			name: "absolute vendor.base_path still resolves targets against the Atmos base path",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.Join("repo", "root"),
				Vendor:   schema.Vendor{BasePath: filepath.Join("elsewhere", "vendor.yaml")},
			},
			vendorConfigFileName: filepath.Join("elsewhere", "vendor.yaml"),
			expected:             filepath.Join("repo", "root"),
		},
		{
			// No `vendor.base_path`: the manifest was discovered next to the working directory, so
			// targets stay relative to the manifest itself.
			name: "vendor.base_path not configured keeps targets relative to the manifest",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: filepath.Join("repo", "root"),
			},
			vendorConfigFileName: filepath.Join("some", "dir", "vendor.yaml"),
			expected:             filepath.Join("some", "dir"),
		},
		{
			name: "empty Atmos base path keeps targets relative to the manifest",
			atmosConfig: &schema.AtmosConfiguration{
				Vendor: schema.Vendor{BasePath: "vendor.yaml"},
			},
			vendorConfigFileName: filepath.Join("some", "dir", "vendor.yaml"),
			expected:             filepath.Join("some", "dir"),
		},
		{
			name:                 "nil Atmos config keeps targets relative to the manifest",
			atmosConfig:          nil,
			vendorConfigFileName: filepath.Join("some", "dir", "vendor.yaml"),
			expected:             filepath.Join("some", "dir"),
		},
		{
			name: "manifest alongside atmos.yaml resolves to the same directory as before",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: ".",
				Vendor:   schema.Vendor{BasePath: "./vendor.yaml"},
			},
			vendorConfigFileName: "vendor.yaml",
			expected:             ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveVendorTargetBasePath(tt.atmosConfig, tt.vendorConfigFileName))
		})
	}
}

// TestResolveVendorSourceBasePath covers which directory a relative local `source` is anchored to
// once sources from imported manifests have been merged into one flat list.
func TestResolveVendorSourceBasePath(t *testing.T) {
	rootDir := filepath.Join("repo", "root")
	importDir := filepath.Join("repo", "root", "vendor", "nested")

	tests := []struct {
		name     string
		source   *schema.AtmosVendorSource
		expected string
	}{
		{
			// The gap CodeRabbit flagged: an imported manifest's source must anchor to that
			// manifest, not to the root manifest that imported it.
			name:     "source carries the declaring manifest's directory",
			source:   &schema.AtmosVendorSource{Component: "vpc", BasePath: importDir},
			expected: importDir,
		},
		{
			name:     "source without provenance falls back to the root manifest directory",
			source:   &schema.AtmosVendorSource{Component: "vpc"},
			expected: rootDir,
		},
		{
			name:     "nil source falls back to the root manifest directory",
			source:   nil,
			expected: rootDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveVendorSourceBasePath(tt.source, rootDir))
		})
	}
}

// TestMergeVendorConfigFiles_RecordsDeclaringManifest verifies that each merged source remembers the
// manifest file it came from, which is what lets an imported source anchor to its own directory.
func TestMergeVendorConfigFiles_RecordsDeclaringManifest(t *testing.T) {
	tempDir := t.TempDir()
	manifestDir := filepath.Join(tempDir, "vendor")
	require.NoError(t, os.MkdirAll(manifestDir, 0o755))

	manifest := filepath.Join(manifestDir, "imported.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
metadata:
  name: imported
spec:
  sources:
    - component: "vpc"
      source: "./local-src"
      targets:
        - "components/terraform/vpc"
    - component: "eks"
      source: "./local-src"
      file: "explicitly-declared.yaml"
      targets:
        - "components/terraform/eks"
`), 0o644))

	vendorConfig, err := mergeVendorConfigFiles([]string{manifest})
	require.NoError(t, err)
	require.Len(t, vendorConfig.Spec.Sources, 2)

	assert.Equal(t, manifestDir, vendorConfig.Spec.Sources[0].BasePath)
	assert.Equal(t, manifest, vendorConfig.Spec.Sources[0].File)

	// A `file` set in the manifest is informational and must not be clobbered, but provenance is
	// still recorded from the file actually read.
	assert.Equal(t, manifestDir, vendorConfig.Spec.Sources[1].BasePath)
	assert.Equal(t, "explicitly-declared.yaml", vendorConfig.Spec.Sources[1].File)
}

// TestResolveVendorTargetPath covers how a single target path is anchored to the target base path.
func TestResolveVendorTargetPath(t *testing.T) {
	// Derive the absolute target from t.TempDir(): a separator-rooted path like `\opt\vendored\vpc`
	// is NOT absolute on Windows (filepath.IsAbs requires a volume name), which would silently turn
	// this into a relative-path test there.
	absTarget := filepath.Join(t.TempDir(), "vendored", "vpc")
	require.True(t, filepath.IsAbs(absTarget), "the test needs a platform-absolute target path")

	tests := []struct {
		name           string
		targetBasePath string
		target         string
		expected       string
	}{
		{
			name:           "relative target is joined to the target base path",
			targetBasePath: filepath.Join("repo", "root"),
			target:         filepath.Join("components", "terraform", "vpc"),
			expected:       filepath.Join("repo", "root", "components", "terraform", "vpc"),
		},
		{
			// `targets` documents support for absolute paths; joining them to the base path would
			// nest them under it instead.
			name:           "absolute target is used as-is",
			targetBasePath: filepath.Join("repo", "root"),
			target:         absTarget,
			expected:       absTarget,
		},
		{
			name:           "empty target base path leaves the target unchanged",
			targetBasePath: "",
			target:         filepath.Join("components", "terraform", "vpc"),
			expected:       filepath.Join("components", "terraform", "vpc"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, resolveVendorTargetPath(tt.targetBasePath, tt.target))
		})
	}
}

// TestProcessTargets_TargetBasePathOverridesVendorConfigDir verifies that relative targets resolve
// against TargetBasePath (the Atmos `base_path`), while the vendor config directory keeps serving as
// the anchor for resolving local sources.
func TestProcessTargets_TargetBasePathOverridesVendorConfigDir(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/org/repo.git//modules/vpc?ref={{.Version}}",
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: "components/terraform/{{.Component}}"},
			{Path: "components/terraform/{{.Component}}/{{.Version}}", Version: "2.0.0"},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    filepath.Join("repo", "root", "config"),
		TargetBasePath:    filepath.Join("repo", "root"),
		URI:               "github.com/org/repo.git//modules/vpc?ref=1.0.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 2)

	assert.Equal(t, filepath.Join("repo", "root", "components", "terraform", "vpc"), pkgs[0].targetPath)
	assert.Equal(t, filepath.Join("repo", "root", "components", "terraform", "vpc", "2.0.0"), pkgs[1].targetPath)

	// The vendor config directory must not leak into the target paths.
	for _, p := range pkgs {
		assert.NotContains(t, filepath.ToSlash(p.targetPath), "config/components")
	}
}

// TestProcessTargets_AbsoluteTargetPath verifies that absolute targets, which are documented as
// supported, are not nested under the target base path.
func TestProcessTargets_AbsoluteTargetPath(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	// t.TempDir() gives a platform-absolute path; a separator-rooted path like `\opt\vendored\vpc`
	// fails filepath.IsAbs on Windows, so this test would assert nothing there.
	absTarget := filepath.Join(t.TempDir(), "vendored", "vpc")
	require.True(t, filepath.IsAbs(absTarget), "the test needs a platform-absolute target path")

	source := schema.AtmosVendorSource{
		Component: "vpc",
		Source:    "github.com/org/repo.git//modules/vpc?ref=1.0.0",
		Version:   "1.0.0",
		Targets: schema.AtmosVendorTargets{
			{Path: absTarget},
		},
	}
	tmplData := struct{ Component, Version string }{"vpc", "1.0.0"}

	pkgs, err := processTargets(&processTargetsParams{
		AtmosConfig:       atmosConfig,
		IndexSource:       0,
		Source:            &source,
		TemplateData:      tmplData,
		SourceBasePath:    filepath.Join("repo", "root", "config"),
		TargetBasePath:    filepath.Join("repo", "root"),
		URI:               "github.com/org/repo.git//modules/vpc?ref=1.0.0",
		SourceTemplate:    source.Source,
		PkgType:           pkgTypeRemote,
		SourceIsLocalFile: false,
	})

	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, absTarget, pkgs[0].targetPath)
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
