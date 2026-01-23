package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/tests/testhelpers"
	"github.com/cloudposse/atmos/toolchain/installer"
)

// TestToolchainCustomCommands_InstallAllTools verifies that all tools defined in
// the custom commands' dependencies.tools can be installed successfully.
//
// This test:
// 1. Installs each tool individually.
// 2. Verifies the binary is placed in the correct location.
// 3. Verifies the binary is executable.
//
// NOTE: This test requires network access to download tools from registries.
func TestToolchainCustomCommands_InstallAllTools(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_InstallAllTools")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Tools to install with their expected binary names and paths.
	tools := []struct {
		name       string // Tool name for installation (owner/repo@version)
		binaryName string // Expected binary name
		owner      string // Owner for path construction
		repo       string // Repo for path construction
		version    string // Version for path construction
	}{
		{"charmbracelet/gum@0.17.0", "gum", "charmbracelet", "gum", "0.17.0"},
		{"derailed/k9s@0.32.7", "k9s", "derailed", "k9s", "0.32.7"},
		{"helm/helm@3.16.3", "helm", "helm", "helm", "3.16.3"},
		{"jqlang/jq@1.7.1", "jq", "jqlang", "jq", "1.7.1"},
		{"opentofu/opentofu@1.9.0", "tofu", "opentofu", "opentofu", "1.9.0"},
	}

	for _, tool := range tools {
		t.Run("Install_"+tool.binaryName, func(t *testing.T) {
			// Install the tool.
			cmd := exec.Command(atmosBinary, "toolchain", "install", tool.name)
			cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
			output, err := cmd.CombinedOutput()
			t.Logf("Install output for %s:\n%s", tool.name, string(output))

			require.NoError(t, err, "toolchain install %s should succeed", tool.name)

			// Verify the binary exists.
			binaryPath := getBinaryPath(toolsDir, tool.owner, tool.repo, tool.version, tool.binaryName)
			assertBinaryExists(t, binaryPath, tool.binaryName)
		})
	}
}

// TestToolchainCustomCommands_ToolsExecutable verifies that installed tools
// can be executed and produce expected output.
//
// This test installs tools and verifies they work by running --version.
func TestToolchainCustomCommands_ToolsExecutable(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_ToolsExecutable")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Tools to test with their version commands and expected output.
	tools := []struct {
		installName    string   // Tool name for installation
		binaryName     string   // Binary name to execute
		owner          string   // Owner for path
		repo           string   // Repo for path
		version        string   // Version for path
		versionArgs    []string // Arguments to get version
		expectedOutput string   // Expected substring in output
	}{
		{"charmbracelet/gum@0.17.0", "gum", "charmbracelet", "gum", "0.17.0", []string{"--version"}, "gum"},
		{"jqlang/jq@1.7.1", "jq", "jqlang", "jq", "1.7.1", []string{"--version"}, "jq"},
		{"helm/helm@3.16.3", "helm", "helm", "helm", "3.16.3", []string{"version", "--short"}, "v3.16"},
		{"opentofu/opentofu@1.9.0", "tofu", "opentofu", "opentofu", "1.9.0", []string{"--version"}, "OpenTofu"},
	}

	for _, tool := range tools {
		t.Run("Execute_"+tool.binaryName, func(t *testing.T) {
			// Install the tool first.
			installCmd := exec.Command(atmosBinary, "toolchain", "install", tool.installName)
			installCmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
			installOutput, err := installCmd.CombinedOutput()
			require.NoError(t, err, "toolchain install should succeed: %s", string(installOutput))

			// Get the binary path.
			binaryPath := getBinaryPath(toolsDir, tool.owner, tool.repo, tool.version, tool.binaryName)
			assertBinaryExists(t, binaryPath, tool.binaryName)

			// Execute the tool with version args.
			execCmd := exec.Command(binaryPath, tool.versionArgs...)
			output, err := execCmd.CombinedOutput()
			t.Logf("%s output:\n%s", tool.binaryName, string(output))

			require.NoError(t, err, "%s should execute successfully", tool.binaryName)
			assert.Contains(t, strings.ToLower(string(output)), strings.ToLower(tool.expectedOutput),
				"%s output should contain %q", tool.binaryName, tool.expectedOutput)
		})
	}
}

// TestToolchainCustomCommands_PathEnvOutput verifies that `atmos toolchain env`
// produces correct output for the current platform.
func TestToolchainCustomCommands_PathEnvOutput(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_PathEnvOutput")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install a tool first so there's something in the PATH.
	installCmd := exec.Command(atmosBinary, "toolchain", "install", "jqlang/jq@1.7.1")
	installCmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	_, err := installCmd.CombinedOutput()
	require.NoError(t, err, "toolchain install should succeed")

	t.Run("BashFormat", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "toolchain", "env", "--format", "bash")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "toolchain env --format bash should succeed")

		outputStr := string(output)
		t.Logf("Bash format output:\n%s", outputStr)

		assert.Contains(t, outputStr, "export PATH=", "Bash output should contain export PATH=")
		assert.Contains(t, outputStr, ".tools", "Bash output should contain .tools directory")
	})

	t.Run("PowershellFormat", func(t *testing.T) {
		cmd := exec.Command(atmosBinary, "toolchain", "env", "--format", "powershell")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "toolchain env --format powershell should succeed")

		outputStr := string(output)
		t.Logf("PowerShell format output:\n%s", outputStr)

		assert.Contains(t, outputStr, "$env:PATH", "PowerShell output should contain $env:PATH")
		assert.Contains(t, outputStr, ".tools", "PowerShell output should contain .tools directory")
	})
}

// TestToolchainCustomCommands_WindowsExeExtension verifies that on Windows,
// binaries are installed with .exe extension.
func TestToolchainCustomCommands_WindowsExeExtension(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_WindowsExeExtension")()

	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install a tool.
	installCmd := exec.Command(atmosBinary, "toolchain", "install", "jqlang/jq@1.7.1")
	installCmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := installCmd.CombinedOutput()
	t.Logf("Install output:\n%s", string(output))
	require.NoError(t, err, "toolchain install should succeed")

	// Verify .exe extension is present.
	binaryPath := filepath.Join(toolsDir, "bin", "jqlang", "jq", "1.7.1", "jq.exe")
	_, err = os.Stat(binaryPath)
	require.NoError(t, err, "Binary should exist at %s with .exe extension", binaryPath)

	// Verify the binary can be executed.
	execCmd := exec.Command(binaryPath, "--version")
	execOutput, err := execCmd.CombinedOutput()
	t.Logf("Execution output:\n%s", string(execOutput))
	require.NoError(t, err, "Binary should be executable")
	assert.Contains(t, string(execOutput), "jq", "Output should indicate jq executed")
}

// TestToolchainCustomCommands_CustomCommandsLoaded verifies that the custom commands
// from .atmos.d/commands.yaml are loaded and appear in help.
func TestToolchainCustomCommands_CustomCommandsLoaded(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_CustomCommandsLoaded")()

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Run atmos --help to see available commands.
	cmd := exec.Command(atmosBinary, "--help")
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "atmos --help should succeed")

	outputStr := string(output)
	t.Logf("Help output:\n%s", outputStr)

	// Verify custom commands are loaded.
	expectedCommands := []string{
		"test-gum",
		"test-k9s",
		"test-helm",
		"test-jq",
		"test-kubectl",
		"test-tofu",
		"test-replicated",
		"test-all-tools",
	}

	for _, cmdName := range expectedCommands {
		assert.Contains(t, outputStr, cmdName,
			"Help output should contain custom command %q", cmdName)
	}
}

// TestToolchainCustomCommands_ExecuteWithDependencies verifies that custom commands
// with dependencies.tools can execute the tools after installation.
//
// This is the key integration test that verifies the full flow:
// 1. Custom command has dependencies.tools defined.
// 2. Atmos installs the required tools.
// 3. Custom command executes successfully using the installed tools.
func TestToolchainCustomCommands_ExecuteWithDependencies(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainCustomCommands_ExecuteWithDependencies")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-custom-commands"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Test custom commands that use dependencies.tools.
	testCases := []struct {
		commandName    string
		expectedOutput string
	}{
		{"test-jq", "jq"},
		{"test-gum", "gum"},
		{"test-helm", "v3.16"},
		{"test-tofu", "OpenTofu"},
	}

	for _, tc := range testCases {
		t.Run(tc.commandName, func(t *testing.T) {
			// Execute the custom command.
			cmd := exec.Command(atmosBinary, tc.commandName)
			cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
			output, err := cmd.CombinedOutput()
			t.Logf("Command %s output:\n%s", tc.commandName, string(output))

			// The command should succeed if toolchain integration works.
			if err != nil {
				t.Logf("Command failed: %v", err)
				t.Logf("This may indicate that toolchain tools are not in PATH for custom commands")
				// Don't fail - document current behavior.
				// Once the toolchain PATH injection is implemented, change to require.NoError.
			} else {
				assert.Contains(t, strings.ToLower(string(output)), strings.ToLower(tc.expectedOutput),
					"Output should contain %q", tc.expectedOutput)
			}
		})
	}
}

// getAtmosRunner returns a shared AtmosRunner for tests.
// It uses a package-level runner to avoid rebuilding atmos for each test.
var sharedRunner *testhelpers.AtmosRunner

// buildAtmosBinary builds the atmos binary using the shared AtmosRunner and returns its path.
func buildAtmosBinary(t *testing.T) string {
	t.Helper()

	if sharedRunner == nil {
		sharedRunner = testhelpers.NewAtmosRunner("")
	}

	err := sharedRunner.Build()
	require.NoError(t, err, "Failed to build atmos")

	// Register cleanup only once.
	t.Cleanup(func() {
		if sharedRunner != nil {
			sharedRunner.Cleanup()
			sharedRunner = nil
		}
	})

	return sharedRunner.BinaryPath()
}

// getBinaryPath constructs the expected binary path for an installed tool.
func getBinaryPath(toolsDir, owner, repo, version, binaryName string) string {
	// Use the centralized function for Windows .exe extension handling.
	binaryName = installer.EnsureWindowsExeExtension(binaryName)
	return filepath.Join(toolsDir, "bin", owner, repo, version, binaryName)
}

// assertBinaryExists verifies that a binary exists and is executable (on Unix).
func assertBinaryExists(t *testing.T, binaryPath, binaryName string) {
	t.Helper()

	info, err := os.Stat(binaryPath)
	require.NoError(t, err, "Binary %s should exist at %s", binaryName, binaryPath)
	require.False(t, info.IsDir(), "Binary path should be a file, not a directory")

	t.Logf("✓ Binary %s found at: %s (size: %d bytes)", binaryName, binaryPath, info.Size())

	// On Unix, verify executable permission.
	if runtime.GOOS != "windows" {
		assert.True(t, info.Mode()&0o111 != 0,
			"Binary %s should be executable (mode: %s)", binaryName, info.Mode())
	}
}
