package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TestToolchainPathEnvironmentOverrides verifies the paths consumed by installs
// after loading project configuration from outside the project directory.
func TestToolchainPathEnvironmentOverrides(t *testing.T) {
	for _, configured := range []bool{false, true} {
		for _, absolute := range []bool{false, true} {
			name := "relative"
			if absolute {
				name = "absolute"
			}
			if configured {
				name += "_overrides_configured_aliases"
			}
			t.Run(name, func(t *testing.T) {
				project := t.TempDir()
				project, err := filepath.EvalSymlinks(project)
				require.NoError(t, err)
				t.Chdir(t.TempDir())
				content := "base_path: .\n"
				if configured {
					content += "toolchain:\n  file_path: ignored-file\n  versions_file: ignored-alias\n  install_path: ignored-tools\n"
				}
				require.NoError(t, os.WriteFile(filepath.Join(project, "atmos.yaml"), []byte(content), 0o644))
				t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
				manifest, install := "env-versions", "env-tools"
				if absolute {
					override := t.TempDir()
					manifest, install = filepath.Join(override, manifest), filepath.Join(override, install)
				}
				t.Setenv("ATMOS_TOOLCHAIN_FILE_PATH", manifest)
				t.Setenv("ATMOS_TOOLCHAIN_INSTALL_PATH", install)
				cfg, err := config.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
				require.NoError(t, err)
				require.Equal(t, manifest, cfg.Toolchain.FilePath)
				require.Equal(t, manifest, cfg.Toolchain.VersionsFile)
				require.Equal(t, install, cfg.Toolchain.InstallPath)
				previous := toolchain.GetAtmosConfig()
				t.Cleanup(func() { toolchain.SetAtmosConfig(previous) })
				toolchain.SetAtmosConfig(&cfg)
				if !absolute {
					manifest, install = filepath.Join(project, manifest), filepath.Join(project, install)
				}
				require.Equal(t, manifest, toolchain.GetToolVersionsFilePath())
				require.Equal(t, install, toolchain.GetInstallPath())
			})
		}
	}
}
