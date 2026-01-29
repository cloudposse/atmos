package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	stackimports "github.com/cloudposse/atmos/pkg/stack/imports"
)

// TestRemoteStackImports_LocalPath verifies that local import detection works correctly.
func TestRemoteStackImports_LocalPath(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		expected bool
	}{
		{"catalog path", "catalog/vpc", true},
		{"relative path", "./config.yaml", true},
		{"absolute path", "/path/to/file.yaml", true},
		{"github shorthand", "github.com/org/repo//path", false},
		{"https url", "https://example.com/file.yaml", false},
		{"git url", "git::https://github.com/org/repo.git", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stackimports.IsLocalPath(tt.uri)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRemoteStackImports_MockServer tests remote imports using a mock HTTP server.
func TestRemoteStackImports_MockServer(t *testing.T) {
	// Create mock HTTP server serving YAML content.
	remoteContent := `
vars:
  imported_from: "remote"
  remote_test: true
  remote_value: "from-mock-server"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(remoteContent))
	}))
	defer server.Close()

	// Create temporary test fixture.
	tempDir := t.TempDir()

	// Create components directory.
	componentDir := filepath.Join(tempDir, "components", "terraform", "myapp")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create minimal component.
	componentMain := `
variable "imported_from" { default = "" }
variable "remote_test" { default = false }
variable "remote_value" { default = "" }
output "imported_from" { value = var.imported_from }
output "remote_test" { value = var.remote_test }
output "remote_value" { value = var.remote_value }
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(componentMain), 0o644)
	require.NoError(t, err)

	// Create stacks directory.
	stacksDir := filepath.Join(tempDir, "stacks", "deploy")
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	// Create atmos.yaml.
	atmosConfig := `
base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.stage }}"
`
	err = os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create stack that imports from mock server.
	stackContent := fmt.Sprintf(`
import:
  - %s/shared.yaml

vars:
  stage: test

components:
  terraform:
    myapp:
      vars:
        local_override: "yes"
`, server.URL)
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Change to temp directory.
	t.Chdir(tempDir)

	// Run atmos describe stacks.
	cmd.RootCmd.SetArgs([]string{"describe", "stacks", "--format", "yaml"})
	err = cmd.Execute()
	require.NoError(t, err, "atmos describe stacks should succeed with remote import")
}

// TestRemoteStackImports_SkipIfMissing tests that skip_if_missing works for remote imports.
func TestRemoteStackImports_SkipIfMissing(t *testing.T) {
	// Create temporary test fixture.
	tempDir := t.TempDir()

	// Create components directory.
	componentDir := filepath.Join(tempDir, "components", "terraform", "myapp")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create minimal component.
	componentMain := `
variable "name" { default = "myapp" }
output "name" { value = var.name }
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(componentMain), 0o644)
	require.NoError(t, err)

	// Create stacks directory.
	stacksDir := filepath.Join(tempDir, "stacks", "deploy")
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

	// Create atmos.yaml.
	atmosConfig := `
base_path: "./"
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{ .vars.stage }}"
`
	err = os.WriteFile(filepath.Join(tempDir, "atmos.yaml"), []byte(atmosConfig), 0o644)
	require.NoError(t, err)

	// Create stack that imports from non-existent URL with skip_if_missing.
	stackContent := `
import:
  - path: "https://nonexistent.invalid/config.yaml"
    skip_if_missing: true

vars:
  stage: test

components:
  terraform:
    myapp:
      vars:
        name: "test-app"
`
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Change to temp directory.
	t.Chdir(tempDir)

	// Run atmos describe stacks - should succeed because skip_if_missing is true.
	cmd.RootCmd.SetArgs([]string{"describe", "stacks", "--format", "yaml"})
	err = cmd.Execute()
	require.NoError(t, err, "atmos describe stacks should succeed with skip_if_missing remote import")
}
