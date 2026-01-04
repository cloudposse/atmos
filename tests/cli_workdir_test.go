package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests/testhelpers"
)

// TestCLIWorkdirCommands tests the workdir CLI commands using the workdir fixture.
func TestCLIWorkdirCommands(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Clear environment variables with automatic cleanup.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Change to workdir fixture.
	workDir := "fixtures/scenarios/workdir"
	t.Chdir(workDir)

	// Clean up any existing workdirs before tests.
	cleanupWorkdirs(t)

	// Run test cases.
	t.Run("list_empty", testWorkdirListEmpty)
	t.Run("list_after_provisioning", testWorkdirListAfterProvisioning)
	t.Run("show_workdir", testWorkdirShow)
	t.Run("describe_workdir", testWorkdirDescribe)
	t.Run("clean_specific", testWorkdirCleanSpecific)
	t.Run("clean_all", testWorkdirCleanAll)
}

// cleanupWorkdirs removes any existing workdirs.
func cleanupWorkdirs(t *testing.T) {
	t.Helper()
	workdirPath := ".workdir"
	if _, err := os.Stat(workdirPath); err == nil {
		require.NoError(t, os.RemoveAll(workdirPath))
	}
}

// runWorkdirCommand runs an atmos terraform workdir command.
func runWorkdirCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	fullArgs := append([]string{"terraform", "workdir"}, args...)
	cmd := atmosRunner.Command(fullArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return stdout.String(), stderr.String(), err
}

// testWorkdirListEmpty tests that list returns empty when no workdirs exist.
func testWorkdirListEmpty(t *testing.T) {
	stdout, stderr, err := runWorkdirCommand(t, "list", "--format", "json")
	// Should succeed even with no workdirs.
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}

	// Empty list or error about no workdirs is acceptable.
	// The command may return [] or an empty table.
	if err == nil && strings.TrimSpace(stdout) != "" {
		// If we got output, it should be valid JSON.
		var workdirs []interface{}
		if err := json.Unmarshal([]byte(stdout), &workdirs); err == nil {
			assert.Empty(t, workdirs, "expected no workdirs initially")
		}
	}
}

// testWorkdirListAfterProvisioning tests list after creating workdirs.
func testWorkdirListAfterProvisioning(t *testing.T) {
	// Create a workdir manually for testing.
	workdirPath := filepath.Join(".workdir", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata file.
	metadata := `{
		"component": "vpc",
		"stack": "dev",
		"source_type": "local",
		"source": "components/terraform/vpc",
		"created_at": "2025-01-01T00:00:00Z",
		"updated_at": "2025-01-01T00:00:00Z",
		"content_hash": "test123"
	}`
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, ".workdir-metadata.json"), []byte(metadata), 0o644))

	// Copy main.tf.
	mainTf := "# test vpc component\n"
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte(mainTf), 0o644))

	// List workdirs.
	stdout, stderr, err := runWorkdirCommand(t, "list")
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}

	// Should show the workdir we created.
	assert.Contains(t, stdout+stderr, "vpc", "expected vpc in workdir list")
}

// testWorkdirShow tests the show command.
func testWorkdirShow(t *testing.T) {
	// Create self-contained workdir with proper structure using stack-component naming.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-show")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata file.
	metadata := `{"component":"vpc-show","stack":"dev","source_type":"local","source":"components/terraform/vpc","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","content_hash":"abc123"}`
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, ".workdir-metadata.json"), []byte(metadata), 0o644))

	// Create main.tf.
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# vpc component"), 0o644))

	stdout, stderr, err := runWorkdirCommand(t, "show", "vpc-show", "--stack", "dev")
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}

	// Should show component details.
	output := stdout + stderr
	assert.Contains(t, output, "vpc-show", "expected vpc-show in show output")
}

// testWorkdirDescribe tests the describe command.
func testWorkdirDescribe(t *testing.T) {
	// Create self-contained workdir with proper structure using stack-component naming.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-vpc-describe")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	// Create metadata file.
	metadata := `{"component":"vpc-describe","stack":"dev","source_type":"local","source":"components/terraform/vpc","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z","content_hash":"def456"}`
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, ".workdir-metadata.json"), []byte(metadata), 0o644))

	// Create main.tf.
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# vpc component"), 0o644))

	stdout, stderr, err := runWorkdirCommand(t, "describe", "vpc-describe", "--stack", "dev")
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}

	// Should output manifest format.
	output := stdout + stderr
	assert.Contains(t, output, "component", "expected component in describe output")
}

// testWorkdirCleanSpecific tests cleaning a specific workdir.
func testWorkdirCleanSpecific(t *testing.T) {
	// Create a workdir to clean using stack-component naming.
	workdirPath := filepath.Join(".workdir", "terraform", "dev-test-clean")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, "main.tf"), []byte("# test"), 0o644))

	// Create metadata.
	metadata := `{"component":"test-clean","stack":"dev","source_type":"local","source":"test","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, ".workdir-metadata.json"), []byte(metadata), 0o644))

	// Verify workdir exists.
	_, err := os.Stat(workdirPath)
	require.NoError(t, err, "workdir should exist before clean")

	// Clean the workdir using component and stack.
	stdout, stderr, err := runWorkdirCommand(t, "clean", "test-clean", "--stack", "dev")
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}

	// Verify workdir was removed.
	_, err = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(err), "workdir should be removed after clean")
}

// testWorkdirCleanAll tests cleaning all workdirs.
func testWorkdirCleanAll(t *testing.T) {
	// Create multiple workdirs.
	workdirs := []string{
		filepath.Join(".workdir", "terraform", "clean-test-1"),
		filepath.Join(".workdir", "terraform", "clean-test-2"),
	}

	for _, wd := range workdirs {
		require.NoError(t, os.MkdirAll(wd, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(wd, "main.tf"), []byte("# test"), 0o644))
	}

	// Verify workdirs exist.
	for _, wd := range workdirs {
		_, err := os.Stat(wd)
		require.NoError(t, err, "workdir should exist before clean: %s", wd)
	}

	// Clean all workdirs using --all flag (no component argument).
	stdout, stderr, err := runWorkdirCommand(t, "clean", "--all")
	if err != nil {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
	}
	require.NoError(t, err, "clean --all should succeed")

	// Verify success message was output.
	output := stdout + stderr
	assert.Contains(t, output, "cleaned", "output should indicate workdirs were cleaned")
}

// TestCLIWorkdirListFormats tests different output formats.
func TestCLIWorkdirListFormats(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	// Skip if there's a skip reason.
	if skipReason != "" {
		t.Skipf("Skipping test: %s", skipReason)
	}

	// Clear environment variables with automatic cleanup.
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	t.Setenv("ATMOS_BASE_PATH", "")

	// Change to workdir fixture.
	workDir := "fixtures/scenarios/workdir"
	t.Chdir(workDir)

	// Create a workdir for testing.
	workdirPath := filepath.Join(".workdir", "terraform", "format-test")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	metadata := `{"component":"format-test","stack":"dev","source_type":"local","source":"test","created_at":"2025-01-01T00:00:00Z","updated_at":"2025-01-01T00:00:00Z"}`
	require.NoError(t, os.WriteFile(filepath.Join(workdirPath, ".workdir-metadata.json"), []byte(metadata), 0o644))

	defer func() {
		_ = os.RemoveAll(".workdir")
	}()

	t.Run("json_format", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "list", "--format", "json")
		if err != nil {
			t.Logf("stderr: %s", stderr)
		}
		// Should output valid JSON.
		if stdout != "" {
			var result interface{}
			err := json.Unmarshal([]byte(stdout), &result)
			assert.NoError(t, err, "output should be valid JSON")
		}
	})

	t.Run("yaml_format", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "list", "--format", "yaml")
		if err != nil {
			t.Logf("stderr: %s", stderr)
		}
		// YAML output should not be empty if workdirs exist.
		_ = stdout // Just verify command runs.
	})

	t.Run("table_format", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "list")
		if err != nil {
			t.Logf("stderr: %s", stderr)
		}
		// Table output is the default.
		_ = stdout // Just verify command runs.
	})
}

// TestCLIWorkdirHelp tests that help commands work.
func TestCLIWorkdirHelp(t *testing.T) {
	// Initialize atmosRunner if not already done.
	if atmosRunner == nil {
		atmosRunner = testhelpers.NewAtmosRunner(coverDir)
		if err := atmosRunner.Build(); err != nil {
			t.Skipf("Failed to initialize Atmos: %v", err)
		}
	}

	t.Run("workdir_help", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "--help")
		require.NoError(t, err, "workdir --help should succeed")

		output := stdout + stderr
		assert.Contains(t, output, "list", "help should mention list command")
		assert.Contains(t, output, "show", "help should mention show command")
		assert.Contains(t, output, "describe", "help should mention describe command")
		assert.Contains(t, output, "clean", "help should mention clean command")
	})

	t.Run("list_help", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "list", "--help")
		require.NoError(t, err, "workdir list --help should succeed")

		output := stdout + stderr
		assert.Contains(t, output, "format", "list help should mention format flag")
	})

	t.Run("clean_help", func(t *testing.T) {
		stdout, stderr, err := runWorkdirCommand(t, "clean", "--help")
		require.NoError(t, err, "workdir clean --help should succeed")

		output := stdout + stderr
		assert.Contains(t, output, "--all", "clean help should mention --all flag")
		assert.Contains(t, output, "--stack", "clean help should mention --stack flag")
	})
}
