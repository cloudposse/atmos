package ansible

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestEnsureDependenciesUsesInstalledManifestTools verifies that ansible components
// get the executable and other installed .tool-versions tools on the subprocess PATH
// without modifying the process environment.
func TestEnsureDependenciesUsesInstalledManifestTools(t *testing.T) {
	root := t.TempDir()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{
		BasePath: root, BasePathAbsolute: root,
		Toolchain: schema.Toolchain{InstallPath: filepath.Join(root, "tools")},
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte("ansible/ansible 11.0.0\njqlang/jq 1.7.1\n"), 0o600))
	jqDir := filepath.Join(root, "tools", "bin", "jqlang", "jq", "1.7.1")
	ansibleDir := filepath.Join(root, "tools", "bin", "ansible", "ansible", "11.0.0")
	for dir, name := range map[string]string{jqDir: "jq", ansibleDir: "ansible"} {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("fixture"), 0o755))
	}
	originalPATH := os.Getenv("PATH")

	info := &schema.ConfigAndStacksInfo{ComponentEnvList: []string{"EXISTING=1"}}
	envList, err := ensureDependencies(config, info)
	require.NoError(t, err)

	require.Len(t, envList, 2)
	assert.Equal(t, "EXISTING=1", envList[0])
	require.True(t, strings.HasPrefix(envList[1], "PATH="))
	assert.Contains(t, envList[1], ansibleDir, "the selected ansible executable is on PATH")
	assert.Contains(t, envList[1], jqDir, "installed manifest tools are on PATH")
	assert.Equal(t, originalPATH, os.Getenv("PATH"), "the process PATH is not modified")
}

func TestEnsureDependenciesWithoutManifest(t *testing.T) {
	root := t.TempDir()
	previous := toolchain.GetAtmosConfig()
	t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
	config := &schema.AtmosConfiguration{BasePath: root, BasePathAbsolute: root}

	envList, err := ensureDependencies(config, &schema.ConfigAndStacksInfo{ComponentEnvList: []string{"EXISTING=1"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"EXISTING=1"}, envList)
}
