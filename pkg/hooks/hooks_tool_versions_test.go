package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestInstallDepsAddsInstalledManifestTools verifies that hooks put .tool-versions
// tools on the hook PATH only when they are already installed, and never install them.
func TestInstallDepsAddsInstalledManifestTools(t *testing.T) {
	for _, tc := range []struct {
		name      string
		installed bool
	}{
		{"installed manifest tool joins the hook PATH", true},
		{"missing manifest tool is neither installed nor added", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			previous := toolchain.GetAtmosConfig()
			t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
			config := &schema.AtmosConfiguration{
				BasePath: root, BasePathAbsolute: root,
				Toolchain: schema.Toolchain{InstallPath: filepath.Join(root, "tools")},
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, ".tool-versions"), []byte("jqlang/jq 1.7.1\n"), 0o600))
			binaryDir := filepath.Join(root, "tools", "bin", "jqlang", "jq", "1.7.1")
			if tc.installed {
				require.NoError(t, os.MkdirAll(binaryDir, 0o755))
				name := "jq"
				if runtime.GOOS == "windows" {
					name += ".exe"
				}
				require.NoError(t, os.WriteFile(filepath.Join(binaryDir, name), []byte("fixture"), 0o755))
			}

			h := &Hooks{}
			require.NoError(t, h.installDeps(config, &schema.ConfigAndStacksInfo{}, nil))

			if tc.installed {
				assert.Contains(t, h.toolchainPATH, binaryDir)
				return
			}
			assert.Empty(t, h.toolchainPATH)
			assert.NoDirExists(t, binaryDir, "hooks must not download manifest tools")
		})
	}
}
