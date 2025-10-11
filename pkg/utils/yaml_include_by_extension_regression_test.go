package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestIncludeLocalFileRelativePath tests the regression where local relative paths
// were being incorrectly sent to go-getter instead of being resolved locally.
// This reproduces the issue reported in DEV-3643.
func TestIncludeLocalFileRelativePath(t *testing.T) {
	// Create a temporary directory structure.
	tmpDir := t.TempDir()

	// Create the directory structure.
	stacksDir := filepath.Join(tmpDir, "stacks")
	objectsDir := filepath.Join(stacksDir, "objects")
	catalogDir := filepath.Join(tmpDir, "catalog", "waf")

	err := os.MkdirAll(objectsDir, 0o755)
	assert.NoError(t, err)
	err = os.MkdirAll(catalogDir, 0o755)
	assert.NoError(t, err)

	// Create the included file.
	ipsContent := `# IP addresses
addresses:
  - "3.1.36.99/32"
  - "3.1.219.207/32"
`
	includedFile := filepath.Join(objectsDir, "ips-org-gateways.yaml")
	err = os.WriteFile(includedFile, []byte(ipsContent), 0o644)
	assert.NoError(t, err)

	// Create a catalog file with !include directive.
	catalogContent := `components:
  terraform:
    waf:
      settings:
        ips:
          org:
            inet_gws: !include "stacks/objects/ips-org-gateways.yaml"
      vars:
        name: waf
`
	catalogFile := filepath.Join(catalogDir, "waf.yaml")
	err = os.WriteFile(catalogFile, []byte(catalogContent), 0o644)
	assert.NoError(t, err)

	// Create atmos config.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:               tmpDir,
		StacksBaseAbsolutePath: stacksDir,
	}

	// Parse the YAML with the include.
	var node yaml.Node
	err = yaml.Unmarshal([]byte(catalogContent), &node)
	assert.NoError(t, err)

	// Process custom tags (including !include).
	err = processCustomTags(atmosConfig, &node, catalogFile)
	assert.NoError(t, err)

	// Verify the content was included correctly.
	var result map[string]any
	err = node.Decode(&result)
	assert.NoError(t, err)

	// Check that the addresses were included.
	components := result["components"].(map[string]any)
	terraform := components["terraform"].(map[string]any)
	waf := terraform["waf"].(map[string]any)
	settings := waf["settings"].(map[string]any)
	ips := settings["ips"].(map[string]any)
	org := ips["org"].(map[string]any)
	inetGws := org["inet_gws"].(map[string]any)
	addresses := inetGws["addresses"].([]any)

	assert.Len(t, addresses, 2)
	assert.Equal(t, "3.1.36.99/32", addresses[0])
	assert.Equal(t, "3.1.219.207/32", addresses[1])
}

// TestIncludeLocalFileNotFound tests that when a local file is not found,
// we get a clear error message and don't try to download it with go-getter.
func TestIncludeLocalFileNotFound(t *testing.T) {
	// Create a temporary directory.
	tmpDir := t.TempDir()

	// Create a catalog file with !include directive pointing to non-existent file.
	catalogContent := `components:
  terraform:
    waf:
      settings:
        ips: !include "stacks/objects/non-existent.yaml"
`
	catalogFile := filepath.Join(tmpDir, "catalog.yaml")
	err := os.WriteFile(catalogFile, []byte(catalogContent), 0o644)
	assert.NoError(t, err)

	// Create atmos config.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tmpDir,
	}

	// Parse the YAML with the include.
	var node yaml.Node
	err = yaml.Unmarshal([]byte(catalogContent), &node)
	assert.NoError(t, err)

	// Process custom tags - this should fail with a clear error.
	err = processCustomTags(atmosConfig, &node, catalogFile)
	assert.Error(t, err)

	// The error should NOT contain "relative paths require a module with a pwd"
	// which would indicate go-getter was incorrectly invoked.
	assert.NotContains(t, err.Error(), "relative paths require a module with a pwd")
	assert.NotContains(t, err.Error(), "relative path")
	assert.NotContains(t, err.Error(), "pwd")
	assert.Contains(t, err.Error(), "could not find local file")
}

// TestIncludeGoGetterShorthand tests that go-getter shorthand paths like
// "github.com/org/repo" are recognized as remote and processed correctly.
func TestIncludeGoGetterShorthand(t *testing.T) {
	// Test that shouldFetchRemote recognizes common go-getter shorthands.
	testCases := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"github.com/hashicorp/terraform-aws-vault", true, "GitHub shorthand"},
		{"gitlab.com/myorg/myrepo", true, "GitLab shorthand"},
		{"https://github.com/org/repo", true, "Full HTTPS URL"},
		{"git::https://github.com/org/repo", true, "Git protocol"},
		{"s3::https://s3.amazonaws.com/bucket/key", true, "S3 URL"},
		{"./local/path", false, "Local relative path"},
		{"/absolute/local/path", false, "Local absolute path"},
		{"local-file.yaml", false, "Simple filename"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := shouldFetchRemote(tc.path)
			assert.Equal(t, tc.expected, result, "shouldFetchRemote(%s) = %v, want %v", tc.path, result, tc.expected)
		})
	}
}
