package vendoring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestVendorPull_TemplateTokenInjection tests all documented manual template-based
// token injection formats to ensure they work as documented.
//
// This test verifies that users can manually specify tokens in vendor.yaml using
// Go templates like {{env "GITHUB_TOKEN"}}, as shown in the documentation.
func TestVendorPull_TemplateTokenInjection(t *testing.T) {
	// Set up environment variables that templates will reference.
	testToken := "ghp_test_token_123"
	customToken := "custom_token_456"
	testUsername := "test_user"
	testPassword := "test_pass"

	// Set test tokens using t.Setenv for automatic cleanup.
	t.Setenv("GITHUB_TOKEN", testToken)
	t.Setenv("CUSTOM_GIT_TOKEN", customToken)
	t.Setenv("GIT_USERNAME", testUsername)
	t.Setenv("GIT_PASSWORD", testPassword)

	// Load config from test scenario.
	configPath := filepath.Join("..", "..", "tests", "fixtures", "scenarios", "vendor-template-tokens")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: configPath,
	}

	// Load vendor config.
	vendorConfigFile := filepath.Join(configPath, "vendor.yaml")
	vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(&atmosConfig, vendorConfigFile, true)
	require.NoError(t, err, "Failed to read vendor config")

	// Verify vendor config was loaded.
	require.NotNil(t, vendorConfig, "Vendor config should not be nil")
	require.NotEmpty(t, vendorConfig.Spec.Sources, "Vendor config should have sources")

	// Test each documented format.
	tests := []struct {
		componentName         string
		description           string
		expectedTokenInURL    string
		expectedUsernameInURL string
		shouldContainToken    bool
	}{
		{
			componentName:         "test-single-quotes-env",
			description:           "Single-quoted YAML with {{env \"GITHUB_TOKEN\"}} (RECOMMENDED)",
			expectedTokenInURL:    testToken,
			expectedUsernameInURL: "x-access-token",
			shouldContainToken:    true,
		},
		{
			componentName:         "test-folded-scalar-env",
			description:           "Folded scalar (>-) with {{env \"GITHUB_TOKEN\"}} (ALTERNATIVE)",
			expectedTokenInURL:    testToken,
			expectedUsernameInURL: "x-access-token",
			shouldContainToken:    true,
		},
		{
			componentName:         "test-custom-token-var",
			description:           "Custom token variable {{env \"CUSTOM_GIT_TOKEN\"}}",
			expectedTokenInURL:    customToken,
			expectedUsernameInURL: customToken, // Used as username when not in user:pass format
			shouldContainToken:    true,
		},
		{
			componentName:         "test-user-pass-template",
			description:           "Username:password both from templates",
			expectedTokenInURL:    testPassword,
			expectedUsernameInURL: testUsername,
			shouldContainToken:    true,
		},
		{
			componentName:         "test-version-template",
			description:           "Version template with env token template",
			expectedTokenInURL:    testToken,
			expectedUsernameInURL: "x-access-token",
			shouldContainToken:    true,
		},
		{
			componentName:         "test-native-injection",
			description:           "Native injection (no template, for comparison)",
			expectedTokenInURL:    "", // Native injection happens later in the process
			expectedUsernameInURL: "",
			shouldContainToken:    false, // Template processing doesn't add token, detector does
		},
	}

	for _, tt := range tests {
		t.Run(tt.componentName, func(t *testing.T) {
			// Find the source config for this component.
			var sourceConfig *schema.AtmosVendorSource
			for i := range vendorConfig.Spec.Sources {
				if vendorConfig.Spec.Sources[i].Component == tt.componentName {
					sourceConfig = &vendorConfig.Spec.Sources[i]
					break
				}
			}

			require.NotNil(t, sourceConfig, "Should find source config for %s", tt.componentName)
			require.NotEmpty(t, sourceConfig.Source, "Source URL should not be empty")

			// Process the template in the source field (simulates what happens during vendor pull).
			tmplData := struct {
				Component string
				Version   string
			}{sourceConfig.Component, sourceConfig.Version}

			processedURL, err := exec.ProcessTmpl(&atmosConfig, "test-source", sourceConfig.Source, tmplData, false)
			require.NoError(t, err, "Template processing should succeed for %s", tt.componentName)

			t.Logf("Testing: %s - %s", tt.componentName, tt.description)
			// Avoid logging full URLs that may embed credentials.

			if tt.shouldContainToken {
				// Verify token was injected via template.
				assert.Contains(t, processedURL, tt.expectedTokenInURL,
					"Template should inject token into URL for %s", tt.componentName)

				// Verify username was injected via template (if different from token).
				if tt.expectedUsernameInURL != tt.expectedTokenInURL {
					assert.Contains(t, processedURL, tt.expectedUsernameInURL,
						"Template should inject username into URL for %s", tt.componentName)
				}

				// Verify URL structure.
				assert.Contains(t, processedURL, "@github.com",
					"URL should contain credentials before hostname")
			}

			// Verify version template was processed.
			assert.Contains(t, processedURL, "0.25.0",
				"Version template should be processed")

			// Verify no template syntax remains in processed URL.
			assert.NotContains(t, processedURL, "{{",
				"Processed URL should not contain template opening delimiters")
			assert.NotContains(t, processedURL, "}}",
				"Processed URL should not contain template closing delimiters")
		})
	}
}

// TestVendorPull_TemplateTokenInjection_ErrorCases tests error handling for
// template-based token injection.
func TestVendorPull_TemplateTokenInjection_ErrorCases(t *testing.T) {
	tests := []struct {
		name           string
		vendorYAML     string
		envSetup       map[string]string
		expectError    bool
		errorContains  string
		expectEmptyURL bool // For cases where missing env var results in empty credentials
	}{
		{
			name: "Missing environment variable returns empty string",
			vendorYAML: `
apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: "test"
      source: 'git::https://{{env "NONEXISTENT_TOKEN"}}@github.com/org/repo.git?ref=main'
      version: "1.0.0"
      targets: ["."]
`,
			envSetup:       map[string]string{},
			expectError:    false, // Gomplate env returns empty string, not error
			expectEmptyURL: true,  // Verify empty credentials
		},
		{
			name: "Invalid template syntax",
			vendorYAML: `
apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: "test"
      source: 'git::https://{{env MISSING_QUOTES}}@github.com/org/repo.git?ref=main'
      version: "1.0.0"
      targets: ["."]
`,
			envSetup:      map[string]string{},
			expectError:   true,
			errorContains: "template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary test directory.
			tempDir := t.TempDir()

			// Set up environment.
			for key, val := range tt.envSetup {
				t.Setenv(key, val)
			}

			// Write vendor.yaml.
			vendorFile := filepath.Join(tempDir, "vendor.yaml")
			err := os.WriteFile(vendorFile, []byte(tt.vendorYAML), 0o644)
			require.NoError(t, err)

			// Write minimal atmos.yaml.
			atmosYAML := `
base_path: "./"
`
			atmosFile := filepath.Join(tempDir, "atmos.yaml")
			err = os.WriteFile(atmosFile, []byte(atmosYAML), 0o644)
			require.NoError(t, err)

			// Load config.
			atmosConfig := schema.AtmosConfiguration{
				BasePath: tempDir,
			}

			// Read vendor config.
			vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(&atmosConfig, vendorFile, true)
			require.NoError(t, err, "Vendor config should parse successfully")

			// Process template (this is where errors should occur).
			if len(vendorConfig.Spec.Sources) == 0 {
				require.Fail(t, "No sources found in vendor config")
				return
			}

			source := vendorConfig.Spec.Sources[0]
			tmplData := struct {
				Component string
				Version   string
			}{source.Component, source.Version}

			processedURL, err := exec.ProcessTmpl(&atmosConfig, "test-source", source.Source, tmplData, false)

			if tt.expectError {
				require.Error(t, err, "Should get error for: %s", tt.name)
				if tt.errorContains != "" {
					assert.Contains(t, strings.ToLower(err.Error()), strings.ToLower(tt.errorContains),
						"Error should mention the problem")
				}
				return
			}

			require.NoError(t, err, "Should not error for: %s", tt.name)
			if tt.expectEmptyURL {
				// Verify empty credentials resulted from missing env var.
				assert.Contains(t, processedURL, "://@",
					"Missing env var should result in empty credentials")
			}
		})
	}
}

// TestVendorPull_TemplateVsNativeInjection tests that template-based injection
// takes precedence over native injection (user credentials in URL always win).
func TestVendorPull_TemplateVsNativeInjection(t *testing.T) {
	// This test verifies the documented precedence:
	// 1. User-specified credentials in URL (from templates) - HIGHEST PRIORITY
	// 2. Native automatic injection - FALLBACK

	testToken := "ghp_template_token"
	atmosToken := "ghp_native_token"

	t.Setenv("GITHUB_TOKEN", testToken)
	t.Setenv("ATMOS_GITHUB_TOKEN", atmosToken)

	vendorYAML := `
apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    # Template-based injection (user-specified).
    - component: "template-creds"
      source: 'git::https://x-access-token:{{env "GITHUB_TOKEN"}}@github.com/org/repo.git?ref=v1'
      version: "1.0.0"
      targets: ["."]

    # Native injection (automatic).
    - component: "native-creds"
      source: "github.com/org/repo.git?ref=v1"
      version: "1.0.0"
      targets: ["."]
`

	tempDir := t.TempDir()
	vendorFile := filepath.Join(tempDir, "vendor.yaml")
	err := os.WriteFile(vendorFile, []byte(vendorYAML), 0o644)
	require.NoError(t, err)

	atmosYAML := `
base_path: "./"
settings:
  inject_github_token: true
`
	atmosFile := filepath.Join(tempDir, "atmos.yaml")
	err = os.WriteFile(atmosFile, []byte(atmosYAML), 0o644)
	require.NoError(t, err)

	atmosConfig := schema.AtmosConfiguration{
		BasePath: tempDir,
	}

	vendorConfig, _, _, err := ReadAndProcessVendorConfigFile(&atmosConfig, vendorFile, true)
	require.NoError(t, err)

	// Verify template-based credentials are in the URL.
	var templateSource, nativeSource *schema.AtmosVendorSource
	for i := range vendorConfig.Spec.Sources {
		if vendorConfig.Spec.Sources[i].Component == "template-creds" {
			templateSource = &vendorConfig.Spec.Sources[i]
		}
		if vendorConfig.Spec.Sources[i].Component == "native-creds" {
			nativeSource = &vendorConfig.Spec.Sources[i]
		}
	}

	require.NotNil(t, templateSource, "Should find template-creds source")
	require.NotNil(t, nativeSource, "Should find native-creds source")

	// Process templates for both sources.
	templateTmplData := struct {
		Component string
		Version   string
	}{templateSource.Component, templateSource.Version}

	nativeTmplData := struct {
		Component string
		Version   string
	}{nativeSource.Component, nativeSource.Version}

	templateProcessedURL, err := exec.ProcessTmpl(&atmosConfig, "template-source", templateSource.Source, templateTmplData, false)
	require.NoError(t, err, "Template processing should succeed for template-creds")

	nativeProcessedURL, err := exec.ProcessTmpl(&atmosConfig, "native-source", nativeSource.Source, nativeTmplData, false)
	require.NoError(t, err, "Template processing should succeed for native-creds")

	// Template source should have credentials from template processing.
	assert.Contains(t, templateProcessedURL, testToken,
		"Template-based source should contain token from env template")
	assert.Contains(t, templateProcessedURL, "x-access-token:"+testToken+"@",
		"Template-based source should have credentials before @")

	// Native source should NOT have credentials yet (added later by detector).
	assert.NotContains(t, nativeProcessedURL, atmosToken,
		"Native source should not have token after template processing (added later)")
	assert.NotContains(t, nativeProcessedURL, "@",
		"Native source should not have @ indicating credentials yet")
}
