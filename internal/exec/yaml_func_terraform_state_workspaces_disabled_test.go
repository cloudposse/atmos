package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestYamlFuncTerraformStateWorkspacesDisabled tests that the !terraform.state YAML function
// works correctly when Terraform workspaces are disabled (workspaces_enabled: false).
//
// When workspaces are disabled:
// - The workspace name is "default"
// - For local backend: state is stored at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate)
// - For S3 backend: state is stored at <key> (not <workspace_key_prefix>/default/<key>)
//
// See: https://github.com/cloudposse/atmos/issues/1920
func TestYamlFuncTerraformStateWorkspacesDisabled(t *testing.T) {
	if _, lookErr := exec.LookPath("tofu"); lookErr != nil {
		if _, lookErr2 := exec.LookPath("terraform"); lookErr2 != nil {
			t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH (required for !terraform.state workspaces-disabled integration test)")
		}
	}
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "test"

	// Define the working directory (workspaces-disabled fixture).
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function-workspaces-disabled"

	// Compute the absolute path to the mock component before changing directories so that the
	// cleanup defer below uses a stable path regardless of the working-directory changes made
	// by t.Chdir further down.  Use the direct path (not a multi-hop via workDir) so a rename
	// of the scenarios directory does not silently produce a wrong path.
	mockComponentPath, err := filepath.Abs("../../tests/fixtures/components/terraform/mock")
	if err != nil {
		t.Fatalf("Failed to compute absolute mock component path: %v", err)
	}

	defer func() {
		// Delete the generated files and folders after the test.
		// Clean up terraform state files.
		err := os.RemoveAll(filepath.Join(mockComponentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(mockComponentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		// When workspaces are disabled, state is stored at terraform.tfstate (not in terraform.tfstate.d/).
		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate"))
		// Ignore error if file doesn't exist.
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}

		err = os.Remove(filepath.Join(mockComponentPath, "terraform.tfstate.backup"))
		// Ignore error if file doesn't exist.
		if err != nil && !os.IsNotExist(err) {
			assert.NoError(t, err)
		}
	}()

	t.Chdir(workDir)

	// Deploy component-1 first to create terraform state.
	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	require.NoError(t, err, "Failed to execute 'ExecuteTerraform' for component-1")

	// Initialize CLI config.
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)

	// Verify that workspaces are disabled.
	require.NotNil(t, atmosConfig.Components.Terraform.WorkspacesEnabled)
	assert.False(t, *atmosConfig.Components.Terraform.WorkspacesEnabled,
		"Expected workspaces to be disabled in this test fixture")

	// Test !terraform.state can read outputs from component-1.
	// When workspaces are disabled, the state should be at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate).
	d, err := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-a", d, "Expected to read 'foo' output from component-1 state")

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack, nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-b", d, "Expected to read 'bar' output from component-1 state")

	d, err = processTagTerraformState(&atmosConfig, "!terraform.state component-1 test baz", "", nil)
	require.NoError(t, err)
	assert.Equal(t, "component-1-c", d, "Expected to read 'baz' output from component-1 state")

	// Verify component-2 can use !terraform.state to reference component-1's outputs.
	res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "component-2",
		Stack:                stack,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		Skip:                 nil,
		AuthManager:          nil,
	})
	require.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	require.NoError(t, err)
	assert.Contains(t, y, "foo: component-1-a", "component-2 should have foo from component-1 state")
	assert.Contains(t, y, "bar: component-1-b", "component-2 should have bar from component-1 state")
	assert.Contains(t, y, "baz: component-1-c", "component-2 should have baz from component-1 state")
}

// TestWorkspacesDisabledStateLocation verifies that when workspaces are disabled,
// the terraform state is stored at the correct location (terraform.tfstate, not terraform.tfstate.d/default/terraform.tfstate).
func TestWorkspacesDisabledStateLocation(t *testing.T) {
	if _, lookErr := exec.LookPath("tofu"); lookErr != nil {
		if _, lookErr2 := exec.LookPath("terraform"); lookErr2 != nil {
			t.Skip("skipping: neither 'tofu' nor 'terraform' binary found in PATH (required for workspaces-disabled state location test)")
		}
	}
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	require.NoError(t, err)

	err = os.Unsetenv("ATMOS_BASE_PATH")
	require.NoError(t, err)

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "test"

	// Get the absolute path to the mock component before changing directories.
	// The path is relative to the current working directory (internal/exec).
	mockComponentPath, err := filepath.Abs("../../tests/fixtures/components/terraform/mock")
	require.NoError(t, err)

	defer func() {
		// Clean up terraform state files from the shared fixture directory.
		// Failing to remove these would leak state into subsequent test runs.
		assert.NoError(t, os.RemoveAll(filepath.Join(mockComponentPath, ".terraform")))
		assert.NoError(t, os.RemoveAll(filepath.Join(mockComponentPath, "terraform.tfstate.d")))

		// Single files may be locked briefly on Windows; retry before failing.
		for _, name := range []string{"terraform.tfstate", "terraform.tfstate.backup"} {
			removeWithRetry(t, filepath.Join(mockComponentPath, name))
		}
	}()

	// Define the working directory (workspaces-disabled fixture).
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function-workspaces-disabled"
	t.Chdir(workDir)

	// Deploy component-1.
	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	// On Windows, the previous test (TestYamlFuncTerraformStateWorkspacesDisabled) may
	// briefly retain a file-lock on terraform.tfstate after exiting.  Retry to avoid a
	// transient "file locked by another process" error when acquiring the state lock.
	err = executeTerraformWithRetry(info)
	require.NoError(t, err, "Failed to deploy component-1")

	// Verify that the state file is at terraform.tfstate (not terraform.tfstate.d/default/terraform.tfstate).
	stateFilePath := filepath.Join(mockComponentPath, "terraform.tfstate")
	wrongStatePath := filepath.Join(mockComponentPath, "terraform.tfstate.d", "default", "terraform.tfstate")

	// State should exist at the correct location.
	_, err = os.Stat(stateFilePath)
	assert.NoError(t, err, "State file should exist at %s when workspaces are disabled", stateFilePath)

	// State should NOT exist at the wrong location.
	_, err = os.Stat(wrongStatePath)
	assert.True(t, os.IsNotExist(err), "State file should NOT exist at %s when workspaces are disabled", wrongStatePath)
}

// executeTerraformWithRetry calls ExecuteTerraform and retries on Windows when the
// error is a transient state-file lock ("file locked by another process").  All tests
// in this package share the same mock-component directory, so a brief OS-level lock
// held by a just-exited Terraform process can prevent the next operation from
// acquiring the state lock.  Retrying is safe because deploy is idempotent.
func executeTerraformWithRetry(info schema.ConfigAndStacksInfo) error {
	const maxAttempts = 3
	for i := range maxAttempts {
		err := ExecuteTerraform(info)
		if err == nil {
			return nil
		}
		if i < maxAttempts-1 && runtime.GOOS == "windows" {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return err
	}
	return nil
}

// removeWithRetry removes a file, retrying on Windows where brief file locks
// can cause transient failures.  Missing files are not an error.  If the file
// still exists after all retries the test is failed so stale state is never
// silently left in a shared fixture.
func removeWithRetry(t *testing.T, path string) {
	t.Helper()
	const maxAttempts = 3
	for i := range maxAttempts {
		err := os.Remove(path)
		if err == nil || os.IsNotExist(err) {
			return
		}
		if i < maxAttempts-1 && runtime.GOOS == "windows" {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		assert.NoError(t, err, "failed to remove %s after %d attempt(s)", path, i+1)
	}
}
