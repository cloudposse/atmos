package dependencies

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestInstalledProjectToolsNeverProvision(t *testing.T) {
	for _, policy := range schema.ToolchainInstallValues {
		t.Run(string(policy), func(t *testing.T) {
			config, rec, option := installPolicyFixture(t, policy)
			// Neither an unselected version nor an unlisted cached tool belongs
			// in the baseline. The selected Terraform version is still missing.
			installTools(t, config,
				manifestTool{"hashicorp", "terraform", "1.14.0"},
				manifestTool{"hashicorp", "packer", "1.11.0"})

			_, err := ForInstalledProjectTools(config, option)

			require.NoError(t, err)
			assert.Zero(t, rec.ensureCalls)
			assert.Equal(t, jqOnly, rec.built)
		})
	}
}

func TestInstalledProjectToolsManifestHandling(t *testing.T) {
	for _, kind := range []string{"absent", "empty", "unreadable", "custom"} {
		t.Run(kind, func(t *testing.T) {
			config, root := defaultsFixture(t, "")
			manifest := filepath.Join(root, ".tool-versions")
			switch kind {
			case "absent":
				require.NoError(t, os.Remove(manifest))
			case "unreadable":
				require.NoError(t, os.Remove(manifest))
				require.NoError(t, os.Mkdir(manifest, 0o755))
			case "custom":
				config.Toolchain.FilePath = "project-tools"
				require.NoError(t, os.WriteFile(filepath.Join(root, "project-tools"), []byte("jqlang/jq 1.7.1\n"), 0o600))
				installTools(t, config, manifestTool{"jqlang", "jq", "1.7.1"})
				subdir := filepath.Join(root, "component")
				require.NoError(t, os.Mkdir(subdir, 0o755))
				t.Chdir(subdir)
			}
			rec, option := recordingProvisioner(t, config)

			env, err := ForInstalledProjectTools(config, option)

			assert.Zero(t, rec.ensureCalls)
			if kind == "unreadable" {
				require.ErrorContains(t, err, "failed to load .tool-versions")
				assert.Nil(t, env)
				return
			}
			require.NoError(t, err)
			if kind == "custom" {
				assert.Equal(t, jqOnly, rec.built)
				return
			}
			assert.Empty(t, env.ToolchainDirs())
			assert.Empty(t, env.EnvVars())
		})
	}
}
