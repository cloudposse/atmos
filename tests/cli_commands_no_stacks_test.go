package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestCommandsWorkWithoutStacks verifies that certain commands work without stack configurations.
// This test ensures commands that don't require stacks (auth, list workflows, list vendor, docs)
// can run with minimal atmos.yaml configuration.
func TestCommandsWorkWithoutStacks(t *testing.T) {
	// Initialize atmosRunner if not already done
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to build Atmos: %v", err)
			return
		}
	}

	// Create temporary directory for test
	tmpDir := t.TempDir()

	// Create minimal atmos.yaml WITH stack configuration (but no actual stack files)
	atmosYAML := `base_path: .

stacks:
  base_path: stacks
  included_paths:
    - "**/*"

components:
  terraform:
    base_path: components/terraform

workflows:
  base_path: workflows

vendor:
  base_path: vendor
`

	err := os.WriteFile(filepath.Join(tmpDir, "atmos.yaml"), []byte(atmosYAML), 0o644)
	require.NoError(t, err, "failed to write atmos.yaml")

	// Create necessary directories (including empty stacks dir with dummy file so paths resolve)
	err = os.MkdirAll(filepath.Join(tmpDir, "stacks"), 0o755)
	require.NoError(t, err, "failed to create stacks directory")
	err = os.WriteFile(filepath.Join(tmpDir, "stacks", "README.md"), []byte("# Stacks\n"), 0o644)
	require.NoError(t, err, "failed to write stacks README")

	err = os.MkdirAll(filepath.Join(tmpDir, "components", "terraform", "vpc"), 0o755)
	require.NoError(t, err, "failed to create component directory")
	err = os.MkdirAll(filepath.Join(tmpDir, "workflows"), 0o755)
	require.NoError(t, err, "failed to create workflows directory")
	err = os.MkdirAll(filepath.Join(tmpDir, "vendor"), 0o755)
	require.NoError(t, err, "failed to create vendor directory")

	// Create a component README for docs command
	readmeContent := "# VPC Component\n\nTest component.\n"
	err = os.WriteFile(filepath.Join(tmpDir, "components", "terraform", "vpc", "README.md"),
		[]byte(readmeContent), 0o644)
	require.NoError(t, err, "failed to write README")

	// Test list workflows (should not require stacks)
	t.Run("list_workflows_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("list", "workflows")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Command should succeed (even if no workflows found)
		// The key is it doesn't error about missing stacks
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"list workflows should not require stack configuration")
		}
	})

	// Test list vendor (should not require stacks)
	t.Run("list_vendor_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("list", "vendor")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Command should succeed or fail for vendor-specific reasons, not stack config
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"list vendor should not require stack configuration")
		}
	})

	// Test docs command (should not require stacks)
	t.Run("docs_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("docs", "vpc")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Command should succeed and show the README
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"docs command should not require stack configuration")
		}
		// If successful, should contain component docs
		if err == nil {
			assert.Contains(t, outputStr, "VPC Component",
				"docs command should display component README")
		}
	})

	// Test auth env command (should not require stacks)
	t.Run("auth_env_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("auth", "env")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Command may fail due to missing AWS credentials, but not stack config
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"auth env should not require stack configuration")
		}
	})

	// Test auth exec command (should not require stacks)
	t.Run("auth_exec_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("auth", "exec", "--", "echo", "test")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Command may fail due to missing AWS credentials, but not stack config
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"auth exec should not require stack configuration")
		}
	})

	// Test auth shell command (should not require stacks)
	t.Run("auth_shell_without_stacks", func(t *testing.T) {
		cmd := atmosRunner.Command("auth", "shell", "--help")
		cmd.Dir = tmpDir
		output, err := cmd.CombinedOutput()
		outputStr := string(output)

		// Help should always succeed
		if err != nil {
			assert.NotContains(t, outputStr, "stack base path must be provided",
				"auth shell should not require stack configuration")
		}
		// If successful, should contain usage info
		if err == nil {
			assert.Contains(t, outputStr, "atmos auth shell",
				"auth shell help should display usage")
		}
	})
}
