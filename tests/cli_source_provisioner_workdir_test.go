package tests

import (
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

// TestJITSource_MetadataComponentSubpath verifies that a JIT source component with
// metadata.component: exports provisions the full repo into the workdir and that the
// exports/ subdirectory is accessible within it. Regression test for GitHub issue #2364.
//
// The component uses the full terraform-null-label repo (no //subpath in URI).
// The metadata.component: exports setting tells atmos the Terraform module lives at exports/.
// The fix in provisionComponentSource updates WorkdirPathKey to <workdir>/exports/.
func TestJITSource_MetadataComponentSubpath(t *testing.T) {
	t.Chdir("./fixtures/scenarios/source-provisioner-workdir")

	t.Cleanup(func() {
		_ = os.RemoveAll(".workdir")
	})

	// --dry-run triggers provisionComponentSource (applying the fix) without
	// requiring tofu to be installed or cloud credentials.
	resetViperState()
	cmd.RootCmd.SetArgs([]string{
		"terraform", "plan", "null-label-exports",
		"--stack", "dev",
		"--dry-run",
	})
	_ = cmd.Execute() // error expected (no terraform binary needed); provisioning still runs

	// Verify the workdir root was created — the full repo was cloned here.
	workdirRoot := filepath.Join(".workdir", "terraform", "dev-null-label-exports")
	info, statErr := os.Stat(workdirRoot)
	require.NoError(t, statErr, "workdir root should exist at %s after provisioning", workdirRoot)
	require.True(t, info.IsDir(), "workdir root should be a directory")

	// Verify exports/ subdir exists within the workdir.
	// terraform-null-label@0.25.0 has exports/exports.tf at the repo root.
	// Its presence confirms the full repo was cloned (not just the subdir).
	exportsDir := filepath.Join(workdirRoot, "exports")
	exportsInfo, statErr := os.Stat(exportsDir)
	require.NoError(t, statErr, "exports/ subdir should exist within workdir at %s", exportsDir)
	require.True(t, exportsInfo.IsDir(), "exports/ should be a directory")

	// Confirm exports/ contains a .tf file — proving it's the correct TF module subdir.
	exportsTfPath := filepath.Join(exportsDir, "exports.tf")
	_, statErr = os.Stat(exportsTfPath)
	require.NoError(t, statErr, "exports/exports.tf should exist (confirming correct module subdir)")
}
