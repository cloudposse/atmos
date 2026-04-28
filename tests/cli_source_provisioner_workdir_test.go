package tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd"
)

// Note: resetViperState is defined in cli_source_provisioner_test.go
// and shared across test files in this package.

// TestSourceWorkdir_SourceOnly tests source describe for component with source but no workdir.
func TestSourceWorkdir_SourceOnly(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-remote", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_SourceWithWorkdir tests source describe for component with both source and workdir.
func TestSourceWorkdir_SourceWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_LocalWithWorkdir_NoSource tests that local component with workdir has no source.
func TestSourceWorkdir_LocalWithWorkdir_NoSource(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "describe", "mock-workdir", "--stack", "dev"})

	err := cmd.Execute()
	// Should return error because component has no source configured.
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "source") || strings.Contains(err.Error(), "uri"),
		"Expected error about missing source")
}

// TestSourceWorkdir_DescribeComponent_SourceOnly tests describe component shows source config.
func TestSourceWorkdir_DescribeComponent_SourceOnly(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "vpc-remote", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DescribeComponent_SourceWithWorkdir tests describe component shows both configs.
func TestSourceWorkdir_DescribeComponent_SourceWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DescribeComponent_LocalWithWorkdir tests describe component shows workdir config.
func TestSourceWorkdir_DescribeComponent_LocalWithWorkdir(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	cmd.RootCmd.SetArgs([]string{"describe", "component", "mock-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.NoError(t, err)
}

// TestSourceWorkdir_DeleteMissingForce tests that delete requires --force flag.
func TestSourceWorkdir_DeleteMissingForce(t *testing.T) {
	resetViperState() // Prevent flag leakage from previous tests
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	// Create the target directory so delete has something to operate on.
	// With workdir enabled, the target directory is .workdir/terraform/<stack>-<component>.
	targetDir := ".workdir/terraform/dev-vpc-remote-workdir"
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	cmd.RootCmd.SetArgs([]string{"terraform", "source", "delete", "vpc-remote-workdir", "--stack", "dev"})

	err := cmd.Execute()
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "force") || strings.Contains(err.Error(), "--force") ||
		strings.Contains(err.Error(), "interactive"),
		"Expected error about missing --force flag or non-interactive mode")
}

// TestJITSource_MetadataComponentSubpath is the end-to-end regression guard
// for issue #2364 on the `terraform generate varfile` path (which calls
// AutoProvisionSource via tryJITProvision, separate from ExecuteTerraform's
// provisionComponentSource). Runs the command against the full
// terraform-null-label repo with metadata.component: exports and asserts the
// generated *.terraform.tfvars.json lands at <workdir>/exports/, not the
// workdir root. Reverting the fix in tryJITProvision moves the varfile to
// the wrong location and fails this test.
func TestJITSource_MetadataComponentSubpath(t *testing.T) {
	RequireExecutable(t, "git", "JIT source provisioning clones a remote repo")
	RequireGitHubAccess(t)

	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	resetViperState()
	cmd.RootCmd.SetArgs([]string{
		"terraform", "generate", "varfile", "null-label-exports",
		"--stack", "dev",
	})
	require.NoError(t, cmd.Execute(), "terraform generate varfile should succeed")

	workdirRoot := filepath.Join(".workdir", "terraform", "dev-null-label-exports")
	rootInfo, statErr := os.Stat(workdirRoot)
	require.NoError(t, statErr, "workdir root should exist at %s after provisioning", workdirRoot)
	require.True(t, rootInfo.IsDir(), "workdir root should be a directory")

	exportsDir := filepath.Join(workdirRoot, "exports")
	exportsInfo, statErr := os.Stat(exportsDir)
	require.NoError(t, statErr, "exports/ subdir should exist within workdir at %s", exportsDir)
	require.True(t, exportsInfo.IsDir(), "exports/ should be a directory")

	// Load-bearing regression assertion: the generated varfile must land in
	// the metadata.component subpath, not the workdir root.
	varfilesAtRoot, err := filepath.Glob(filepath.Join(workdirRoot, "*.terraform.tfvars.json"))
	require.NoError(t, err)
	assert.Empty(t, varfilesAtRoot,
		"metadata.component subpath ignored — varfile generated at workdir root: %v", varfilesAtRoot)

	varfilesInSubpath, err := filepath.Glob(filepath.Join(exportsDir, "*.terraform.tfvars.json"))
	require.NoError(t, err)
	assert.NotEmpty(t, varfilesInSubpath,
		"varfile must be generated inside %s when metadata.component is honored", exportsDir)
}

// TestJITSource_MetadataComponentSubpath_TerraformShell guards the sibling
// code path: `atmos terraform shell` runs ExecuteProvisioners directly
// (instead of going through provisionComponentSource), so without the
// applyWorkdirSubpathToSection call wired into ExecuteTerraformShell the
// shell would land in the workdir root, ignoring metadata.component.
//
// Asserts via dry-run output that both the working directory and component
// path printed by printShellDryRunInfo include the metadata.component
// subpath. Reverting the fix in terraform_shell.go fails these assertions.
func TestJITSource_MetadataComponentSubpath_TerraformShell(t *testing.T) {
	RequireExecutable(t, "git", "JIT source provisioning clones a remote repo")
	RequireGitHubAccess(t)

	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	// Capture stderr (where ui.Writeln output goes) for the dry-run banner.
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	resetViperState()
	cmd.RootCmd.SetArgs([]string{
		"terraform", "shell", "null-label-exports",
		"--stack", "dev",
		"--dry-run",
	})
	execErr := cmd.Execute()

	// Close the writer so the reader returns EOF, then drain it.
	require.NoError(t, w.Close())
	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)
	require.NoError(t, r.Close())

	require.NoError(t, execErr, "terraform shell --dry-run should succeed; output: %s", buf.String())

	output := buf.String()

	// Both fields are driven by ComponentSection[WorkdirPathKey]; with the fix,
	// that key holds <workdir>/exports/ — without it, the bare workdir root.
	expectedSuffix := filepath.Join("dev-null-label-exports", "exports")
	assert.Contains(t, output, "Working directory: ",
		"dry-run should print working directory; got: %s", output)
	assert.Contains(t, output, expectedSuffix,
		"working directory and component path must include metadata.component subpath %q; got: %s",
		expectedSuffix, output)
}
