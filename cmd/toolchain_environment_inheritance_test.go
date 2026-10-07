package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestInstalledProjectToolsChildProcess exercises startup using the environment inherited by a real child process.
func TestInstalledProjectToolsChildProcess(t *testing.T) {
	root := os.Getenv("_ATMOS_TEST_PROJECT")
	if root == "" {
		return
	}
	_ = NewTestKit(t)
	config := &schema.AtmosConfiguration{BasePathAbsolute: root}
	config.Toolchain.InstallPath = os.Getenv("_ATMOS_TEST_INSTALL_PATH")
	config.Toolchain.Install = schema.ToolchainInstallNever
	t.Chdir(root)

	// The child must start with the parent's selection to reproduce the leak.
	assert.Contains(t, filepath.SplitList(os.Getenv("PATH")), os.Getenv("_ATMOS_TEST_PARENT_DIR"))
	err := applyInstalledProjectTools(config)
	if os.Getenv("_ATMOS_TEST_EXPECT_ERROR") == "true" {
		require.Error(t, err)
	} else {
		require.NoError(t, err)
	}
	assert.Equal(t, os.Getenv("_ATMOS_TEST_EXPECTED_PATH"), os.Getenv("PATH"))
	binary, err := exec.LookPath("terraform")
	require.NoError(t, err)
	assert.Equal(t, os.Getenv("_ATMOS_TEST_TERRAFORM"), binary)
	// Repeated initialization must retain the same baseline, including after errors.
	_ = applyInstalledProjectTools(config)
	assert.Equal(t, os.Getenv("_ATMOS_TEST_EXPECTED_PATH"), os.Getenv("PATH"))
}

// TestInstalledProjectToolsReplacesParentSelections verifies project changes remove only Atmos-inserted directories.
func TestInstalledProjectToolsReplacesParentSelections(t *testing.T) {
	for _, kind := range []string{"absent", "empty", "uninstalled", "unreadable", "different-tool", "different-version", "same-version", "user-path"} {
		t.Run(kind, func(t *testing.T) {
			_ = NewTestKit(t)
			t.Setenv("_ATMOS_TOOLCHAIN_PATH", "")
			parent, child := t.TempDir(), t.TempDir()
			config := &schema.AtmosConfiguration{BasePathAbsolute: parent}
			installPath := useNeverInstallToolchain(t, config)
			parentBinary := installCommandTool(t, installPath, "hashicorp", "terraform", "1.15.8")
			systemBinary := installCommandTool(t, t.TempDir(), "system", "terraform", "fallback")
			basePath := filepath.Dir(systemBinary)
			if kind == "user-path" {
				basePath = filepath.Dir(parentBinary) + string(os.PathListSeparator) + basePath
			}
			t.Setenv("PATH", basePath)
			require.NoError(t, os.WriteFile(filepath.Join(parent, ".tool-versions"), []byte("hashicorp/terraform 1.15.8\n"), 0o600))
			require.NoError(t, applyInstalledProjectTools(config))
			binary, err := exec.LookPath("terraform")
			require.NoError(t, err)
			require.Equal(t, parentBinary, binary)

			expectedPath, expectedBinary := basePath, systemBinary
			manifest := filepath.Join(child, ".tool-versions")
			switch kind {
			case "empty":
				require.NoError(t, os.WriteFile(manifest, nil, 0o600))
			case "uninstalled":
				require.NoError(t, os.WriteFile(manifest, []byte("hashicorp/terraform 1.14.0\n"), 0o600))
			case "unreadable":
				require.NoError(t, os.Mkdir(manifest, 0o755))
			case "different-tool":
				jq := installCommandTool(t, installPath, "jqlang", "jq", "1.7.1")
				require.NoError(t, os.WriteFile(manifest, []byte("jqlang/jq 1.7.1\n"), 0o600))
				expectedPath = filepath.Dir(jq) + string(os.PathListSeparator) + basePath
			case "different-version", "same-version":
				version := "1.15.6"
				if kind == "same-version" {
					version = "1.15.8"
				}
				expectedBinary = installCommandTool(t, installPath, "hashicorp", "terraform", version)
				require.NoError(t, os.WriteFile(manifest, []byte("hashicorp/terraform "+version+"\n"), 0o600))
				expectedPath = filepath.Dir(expectedBinary) + string(os.PathListSeparator) + basePath
			case "user-path":
				expectedBinary = parentBinary
			}
			t.Setenv("_ATMOS_TEST_PROJECT", child)
			t.Setenv("_ATMOS_TEST_INSTALL_PATH", installPath)
			t.Setenv("_ATMOS_TEST_EXPECTED_PATH", expectedPath)
			t.Setenv("_ATMOS_TEST_TERRAFORM", expectedBinary)
			t.Setenv("_ATMOS_TEST_PARENT_DIR", filepath.Dir(parentBinary))
			t.Setenv("_ATMOS_TEST_EXPECT_ERROR", map[bool]string{true: "true", false: "false"}[kind == "unreadable"])
			testBinary, err := os.Executable()
			require.NoError(t, err)
			command := exec.Command(testBinary, "-test.run=^TestInstalledProjectToolsChildProcess$")
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", strings.TrimSpace(string(output)))
			assert.NoDirExists(t, filepath.Join(installPath, "bin", "hashicorp", "terraform", "1.14.0"))
		})
	}
}
