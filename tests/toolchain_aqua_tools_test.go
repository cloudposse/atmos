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
	"github.com/cloudposse/atmos/toolchain/installer"
)

// Tool version constants for maintainability.
const (
	versionKubectl   = "1.31.4"
	versionGum       = "0.17.0"
	versionK9s       = "0.32.7"
	versionHelm      = "3.16.3"
	versionJq        = "1.7.1"
	versionOpentofu  = "1.9.0"
	versionKots      = "1.127.0"
	versionReplicate = "0.124.1" // Non-existent tool for error testing.
)

// TestToolchainAquaTools_KubectlBinaryNaming verifies that kubectl is installed
// with the correct binary name "kubectl" instead of "kubernetes".
//
// This tests the fix for 3-segment Aqua package names where:
// - Package name: "kubernetes/kubernetes/kubectl"
// - Expected binary: "kubectl" (extracted from last segment)
// - Wrong binary: "kubernetes" (falling back to repo_name)
//
// NOTE: This test requires network access to download kubectl from dl.k8s.io.
func TestToolchainAquaTools_KubectlBinaryNaming(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_KubectlBinaryNaming")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install kubectl.
	cmd := exec.Command(atmosBinary, "toolchain", "install", "kubernetes/kubectl@"+versionKubectl)
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	t.Logf("Install output:\n%s", string(output))

	require.NoError(t, err, "toolchain install kubernetes/kubectl@%s should succeed", versionKubectl)

	// Verify the binary is named "kubectl" NOT "kubernetes".
	correctBinaryName := installer.EnsureWindowsExeExtension("kubectl")
	wrongBinaryName := installer.EnsureWindowsExeExtension("kubernetes")

	correctPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", versionKubectl, correctBinaryName)
	wrongPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", versionKubectl, wrongBinaryName)

	// The correct binary should exist.
	info, err := os.Stat(correctPath)
	require.NoError(t, err, "Binary should exist at %s with name 'kubectl'", correctPath)
	require.False(t, info.IsDir(), "Binary path should be a file, not a directory")
	t.Logf("✓ Binary correctly named 'kubectl' at: %s (size: %d bytes)", correctPath, info.Size())

	// The wrong binary should NOT exist.
	_, err = os.Stat(wrongPath)
	assert.True(t, os.IsNotExist(err), "Binary should NOT be named 'kubernetes' at %s", wrongPath)

	// Verify the output message shows correct binary name.
	assert.Contains(t, string(output), "kubectl", "Output should mention kubectl binary name")
	assert.NotContains(t, string(output), "/kubernetes (", "Output should NOT show 'kubernetes' as binary name")
}

// TestToolchainAquaTools_KubectlExecutable verifies that the installed kubectl
// binary is executable and produces expected output.
func TestToolchainAquaTools_KubectlExecutable(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_KubectlExecutable")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install kubectl.
	installCmd := exec.Command(atmosBinary, "toolchain", "install", "kubernetes/kubectl@"+versionKubectl)
	installCmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	_, err := installCmd.CombinedOutput()
	require.NoError(t, err, "toolchain install should succeed")

	// Get the binary path.
	binaryName := installer.EnsureWindowsExeExtension("kubectl")
	binaryPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", versionKubectl, binaryName)

	// Execute kubectl version.
	execCmd := exec.Command(binaryPath, "version", "--client", "--output=yaml")
	output, err := execCmd.CombinedOutput()
	t.Logf("kubectl version output:\n%s", string(output))

	require.NoError(t, err, "kubectl should execute successfully")
	assert.Contains(t, string(output), "clientVersion", "Output should contain clientVersion")
	assert.Contains(t, string(output), "v"+versionKubectl, "Output should contain version v%s", versionKubectl)
}

// TestToolchainAquaTools_InstallAllTools verifies that all tools in the fixture
// can be installed successfully and have correct binary names.
//
// On Linux and macOS: installs ALL tools including replicatedhq/kots.
// On Windows: installs only cross-platform tools (skips kots which doesn't support Windows).
func TestToolchainAquaTools_InstallAllTools(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_InstallAllTools")()

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Cross-platform tools (work on Windows, macOS, Linux).
	// Note: kubernetes/kubectl specifically tests the 3-segment package name fix.
	crossPlatformTools := []struct {
		name       string // Tool name for installation (owner/repo@version)
		binaryName string // Expected binary name
		owner      string // Owner for path construction
		repo       string // Repo for path construction
		version    string // Version for path construction
	}{
		{"charmbracelet/gum@" + versionGum, "gum", "charmbracelet", "gum", versionGum},
		{"derailed/k9s@" + versionK9s, "k9s", "derailed", "k9s", versionK9s},
		{"helm/helm@" + versionHelm, "helm", "helm", "helm", versionHelm},
		{"jqlang/jq@" + versionJq, "jq", "jqlang", "jq", versionJq},
		// kubectl: Tests binary naming fix for 3-segment package names.
		{"kubernetes/kubectl@" + versionKubectl, "kubectl", "kubernetes", "kubectl", versionKubectl},
		{"opentofu/opentofu@" + versionOpentofu, "tofu", "opentofu", "opentofu", versionOpentofu},
	}

	// Platform-specific tools (only darwin and linux).
	darwinLinuxOnlyTools := []struct {
		name       string
		binaryName string
		owner      string
		repo       string
		version    string
	}{
		// kots: Tests platform detection - only supports darwin and linux.
		{"replicatedhq/kots@" + versionKots, "kubectl-kots", "replicatedhq", "kots", versionKots},
	}

	// Install cross-platform tools on all platforms.
	for _, tool := range crossPlatformTools {
		t.Run("Install_"+tool.binaryName, func(t *testing.T) {
			// Install the tool.
			cmd := exec.Command(atmosBinary, "toolchain", "install", tool.name)
			cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
			output, err := cmd.CombinedOutput()
			t.Logf("Install output for %s:\n%s", tool.name, string(output))

			require.NoError(t, err, "toolchain install %s should succeed", tool.name)

			// Verify the binary exists with correct name.
			binaryPath := getBinaryPath(toolsDir, tool.owner, tool.repo, tool.version, tool.binaryName)
			assertBinaryExists(t, binaryPath, tool.binaryName)
		})
	}

	// Install darwin/linux-only tools only on those platforms.
	if runtime.GOOS != "windows" {
		for _, tool := range darwinLinuxOnlyTools {
			t.Run("Install_"+tool.binaryName, func(t *testing.T) {
				// Install the tool.
				cmd := exec.Command(atmosBinary, "toolchain", "install", tool.name)
				cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
				output, err := cmd.CombinedOutput()
				t.Logf("Install output for %s:\n%s", tool.name, string(output))

				require.NoError(t, err, "toolchain install %s should succeed on %s", tool.name, runtime.GOOS)

				// Verify the binary exists with correct name.
				binaryPath := getBinaryPath(toolsDir, tool.owner, tool.repo, tool.version, tool.binaryName)
				assertBinaryExists(t, binaryPath, tool.binaryName)
			})
		}
	}
}

// TestToolchainAquaTools_ToolsList verifies that `atmos toolchain list` shows
// correct binary names for all tools including kubectl.
func TestToolchainAquaTools_ToolsList(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_ToolsList")()

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Run toolchain list.
	cmd := exec.Command(atmosBinary, "toolchain", "list")
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	t.Logf("Toolchain list output:\n%s", string(output))

	require.NoError(t, err, "toolchain list should succeed")

	// Verify kubectl shows correct binary name in the BINARY column.
	outputStr := string(output)
	assert.Contains(t, outputStr, "kubectl", "List should show kubectl as the binary name")

	// Verify all expected tools are listed.
	expectedTools := []string{
		"charmbracelet/gum",
		"derailed/k9s",
		"helm/helm",
		"jqlang/jq",
		"kubernetes/kubectl",
		"opentofu/opentofu",
		"replicatedhq/kots",
	}

	for _, tool := range expectedTools {
		assert.Contains(t, outputStr, tool, "List should contain %s", tool)
	}
}

// TestToolchainAquaTools_WindowsKubectl verifies that kubectl is installed
// with .exe extension on Windows.
func TestToolchainAquaTools_WindowsKubectl(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_WindowsKubectl")()

	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install kubectl.
	installCmd := exec.Command(atmosBinary, "toolchain", "install", "kubernetes/kubectl@"+versionKubectl)
	installCmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := installCmd.CombinedOutput()
	t.Logf("Install output:\n%s", string(output))
	require.NoError(t, err, "toolchain install should succeed")

	// Verify .exe extension is present and binary is named kubectl.exe NOT kubernetes.exe.
	correctPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", versionKubectl, "kubectl.exe")
	wrongPath := filepath.Join(toolsDir, "bin", "kubernetes", "kubectl", versionKubectl, "kubernetes.exe")

	_, err = os.Stat(correctPath)
	require.NoError(t, err, "Binary should exist at %s with name 'kubectl.exe'", correctPath)

	_, err = os.Stat(wrongPath)
	assert.True(t, os.IsNotExist(err), "Binary should NOT be named 'kubernetes.exe' at %s", wrongPath)

	// Verify the binary can be executed.
	// Note: --short flag was removed in kubectl 1.28+, use --client only.
	execCmd := exec.Command(correctPath, "version", "--client")
	execOutput, err := execCmd.CombinedOutput()
	t.Logf("Execution output:\n%s", string(execOutput))
	require.NoError(t, err, "kubectl.exe should be executable")
	assert.Contains(t, string(execOutput), "v"+versionKubectl, "Output should indicate kubectl version")
}

// TestToolchainAquaTools_KotsInstall verifies that replicatedhq/kots
// installs successfully on Linux and macOS (platforms it supports).
//
// NOTE: This tool only supports darwin and linux, NOT Windows.
// The Aqua registry entry has: supported_envs: [darwin, linux].
func TestToolchainAquaTools_KotsInstall(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_KotsInstall")()

	// This tool only supports darwin and linux.
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows - replicatedhq/kots does not support Windows")
	}

	// Skip on macOS CI to prevent timeouts.
	skipInMacOSCI(t)

	if testing.Short() {
		t.Skip("Skipping toolchain integration test in short mode (requires network)")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Clean up any existing toolchain installations.
	toolsDir := ".tools"
	os.RemoveAll(toolsDir)
	defer os.RemoveAll(toolsDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Install kots.
	cmd := exec.Command(atmosBinary, "toolchain", "install", "replicatedhq/kots@"+versionKots)
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	t.Logf("Install output:\n%s", string(output))

	require.NoError(t, err, "toolchain install replicatedhq/kots@%s should succeed on %s/%s", versionKots, runtime.GOOS, runtime.GOARCH)

	// Verify the binary exists (kots installs as kubectl-kots).
	binaryName := installer.EnsureWindowsExeExtension("kubectl-kots")
	binaryPath := filepath.Join(toolsDir, "bin", "replicatedhq", "kots", versionKots, binaryName)

	info, err := os.Stat(binaryPath)
	require.NoError(t, err, "Binary should exist at %s", binaryPath)
	require.False(t, info.IsDir(), "Binary path should be a file, not a directory")
	t.Logf("✓ kots installed successfully at: %s (size: %d bytes)", binaryPath, info.Size())
}

// TestToolchainAquaTools_WindowsKotsPlatformError verifies that on Windows,
// attempting to install replicatedhq/kots shows a helpful platform error message
// with WSL hints instead of attempting a download that would fail.
//
// This tests the pre-flight platform compatibility check feature.
// The Aqua registry entry has: supported_envs: [darwin, linux] (no Windows).
func TestToolchainAquaTools_WindowsKotsPlatformError(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_WindowsKotsPlatformError")()

	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific platform error test on non-Windows platform")
	}

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Attempt to install kots on Windows - should fail with platform error.
	cmd := exec.Command(atmosBinary, "toolchain", "install", "replicatedhq/kots@"+versionKots)
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	t.Logf("Install output:\n%s", outputStr)

	// The command should fail.
	require.Error(t, err, "toolchain install should fail on Windows for kots")

	// Verify the error message contains platform-specific information.
	assert.Contains(t, outputStr, "does not support", "Error should mention platform not supported")

	// Verify WSL hint is shown since Linux is supported.
	assert.Contains(t, outputStr, "WSL", "Error should suggest WSL as an alternative")

	// Verify it shows the supported platforms.
	assert.True(t,
		strings.Contains(outputStr, "darwin") || strings.Contains(outputStr, "linux"),
		"Error should list supported platforms (darwin, linux)")

	t.Logf("✓ Platform error correctly shown for Windows with WSL hint")
}

// TestToolchainAquaTools_NonExistentToolError verifies that attempting to install
// a tool that doesn't exist in any registry shows a clear "tool not in registry" error.
//
// This uses replicatedhq/replicated which does NOT exist in the Aqua registry
// (only replicatedhq/kots and replicatedhq/outdated exist).
func TestToolchainAquaTools_NonExistentToolError(t *testing.T) {
	defer perf.Track(nil, "tests.TestToolchainAquaTools_NonExistentToolError")()

	workDir := "fixtures/scenarios/toolchain-aqua-tools"
	t.Chdir(workDir)

	// Build atmos binary for testing.
	atmosBinary := buildAtmosBinary(t)

	// Attempt to install replicatedhq/replicated which doesn't exist in the registry.
	cmd := exec.Command(atmosBinary, "toolchain", "install", "replicatedhq/replicated@"+versionReplicate)
	cmd.Env = append(os.Environ(), "ATMOS_LOGS_LEVEL=Info")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	t.Logf("Install output:\n%s", outputStr)

	// The command should fail.
	require.Error(t, err, "toolchain install should fail for non-existent tool")

	// Verify the error message indicates the tool was not found in the registry.
	assert.Contains(t, outputStr, "not in registry",
		"Error should indicate tool was not found in registry")

	// Verify the error message mentions the tool name.
	assert.Contains(t, outputStr, "replicatedhq/replicated",
		"Error should mention the tool name")

	// Verify helpful hints are provided.
	assert.True(t,
		strings.Contains(outputStr, "search") || strings.Contains(outputStr, "registry"),
		"Error should suggest searching the registry or checking configuration")

	t.Logf("✓ Non-existent tool error correctly shown for replicatedhq/replicated")
}
