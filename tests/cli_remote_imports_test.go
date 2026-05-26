package tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
	stackimports "github.com/cloudposse/atmos/pkg/stack/imports"
)

func initRemoteImportsGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()

	repoDir := t.TempDir()
	runRemoteImportsGit(t, repoDir, "init")
	runRemoteImportsGit(t, repoDir, "checkout", "-b", "main")
	runRemoteImportsGit(t, repoDir, "config", "user.email", "test@example.com")
	runRemoteImportsGit(t, repoDir, "config", "user.name", "Test User")

	for name, content := range files {
		path := filepath.Join(repoDir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	runRemoteImportsGit(t, repoDir, "add", ".")
	runRemoteImportsGit(t, repoDir, "commit", "-m", "initial")
	return repoDir
}

func runRemoteImportsGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func remoteImportsGitFileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func executeRootCommand(t *testing.T, args ...string) string {
	t.Helper()

	oldStdout := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	defer reader.Close()
	os.Stdout = writer
	defer func() {
		os.Stdout = oldStdout
	}()

	readDone := make(chan struct {
		output []byte
		err    error
	}, 1)
	go func() {
		output, readErr := io.ReadAll(reader)
		readDone <- struct {
			output []byte
			err    error
		}{output: output, err: readErr}
	}()

	cmd.RootCmd.SetArgs(args)
	execErr := cmd.Execute()
	require.NoError(t, writer.Close())
	readResult := <-readDone
	require.NoError(t, readResult.err)
	require.NoError(t, execErr)
	return string(readResult.output)
}

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
	// Create a mock HTTP server that returns 404.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
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

	// Create stack that imports from the mock server (which returns 404) with skip_if_missing.
	stackContent := fmt.Sprintf(`
import:
  - path: "%s/config.yaml"
    skip_if_missing: true

vars:
  stage: test

components:
  terraform:
    myapp:
      vars:
        name: "test-app"
`, server.URL)
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Change to temp directory.
	t.Chdir(tempDir)

	// Run atmos describe stacks - should succeed because skip_if_missing is true.
	cmd.RootCmd.SetArgs([]string{"describe", "stacks", "--format", "yaml"})
	err = cmd.Execute()
	require.NoError(t, err, "atmos describe stacks should succeed with skip_if_missing remote import")
}

func TestRemoteStackImports_GitDirectoryAndSkipIfMissing(t *testing.T) {
	repoDir := initRemoteImportsGitRepo(t, map[string]string{
		"remote/base.yaml": `
components:
  terraform:
    myapp:
      vars:
        from_base: true
`,
		"remote/nested/override.yaml": `
components:
  terraform:
    myapp:
      vars:
        from_nested: true
`,
		"remote/ignored.txt": "ignored\n",
	})

	tempDir := t.TempDir()

	componentDir := filepath.Join(tempDir, "components", "terraform", "myapp")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	componentMain := `
variable "from_base" { default = false }
variable "from_nested" { default = false }
output "from_base" { value = var.from_base }
output "from_nested" { value = var.from_nested }
`
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte(componentMain), 0o644)
	require.NoError(t, err)

	stacksDir := filepath.Join(tempDir, "stacks", "deploy")
	err = os.MkdirAll(stacksDir, 0o755)
	require.NoError(t, err)

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

	repoURI := remoteImportsGitFileURI(repoDir)
	stackContent := fmt.Sprintf(`
import:
  - git::%s//remote?ref=main
  - path: "git::%s//missing?ref=main"
    skip_if_missing: true

vars:
  stage: test

components:
  terraform:
    myapp:
      vars:
        local_override: "yes"
`, repoURI, repoURI)
	err = os.WriteFile(filepath.Join(stacksDir, "test.yaml"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	t.Chdir(tempDir)

	output := executeRootCommand(t, "describe", "stacks", "--format", "yaml")
	assert.Contains(t, output, "from_base: true")
	assert.Contains(t, output, "from_nested: true")
}
