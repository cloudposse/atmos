package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

var _ = schema.AtmosConfiguration{BasePathAbsolute: "", BasePathConfigDir: "", CliConfigPath: ""}

func TestVendorCleanCmdMultipleConfigSources(t *testing.T) {
	for _, mode := range []string{"remove", "dry-run", "modified"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			viper.Reset()
			t.Cleanup(viper.Reset)
			for _, env := range []string{"ATMOS_BASE_PATH", "ATMOS_CLI_CONFIG_PATH", "ATMOS_CONFIG", "ATMOS_CONFIG_PATH", "ATMOS_PROFILE", "TEST_GIT_ROOT"} {
				t.Setenv(env, "")
				require.NoError(t, os.Unsetenv(env))
			}
			t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
			sourceDir := filepath.Join(root, "primary")
			overlayDir := filepath.Join(root, "overlay")
			require.NoError(t, os.MkdirAll(sourceDir, 0o755))
			require.NoError(t, os.MkdirAll(overlayDir, 0o755))
			primary := filepath.Join(sourceDir, "main.yaml")
			overlay := filepath.Join(overlayDir, "extra.yaml")
			require.NoError(t, os.WriteFile(primary, []byte("logs:\n  level: Warning\n"), 0o644))
			require.NoError(t, os.WriteFile(overlay, []byte("settings:\n  terminal:\n    pager: 'false'\n"), 0o644))
			cmd := newVendorCleanTestCmd()
			require.NoError(t, cmd.Flags().Set("config", primary+","+overlay))
			if mode == "dry-run" {
				require.NoError(t, cmd.Flags().Set("dry-run", "true"))
			}
			info, err := e.ProcessCommandLineArgs("terraform", cmd, nil, nil)
			require.NoError(t, err)
			config, err := cfg.InitCliConfig(info, false)
			require.NoError(t, err)
			require.Empty(t, config.BasePath, "fixture must exercise the omitted base_path fallback")
			require.Contains(t, config.CliConfigPath, ";", "fixture must combine multiple config sources")
			require.Equal(t, sourceDir, config.BasePathConfigDir)
			require.Equal(t, sourceDir, config.BasePathAbsolute)
			target := filepath.Join(sourceDir, "vendor")
			require.NoError(t, os.MkdirAll(target, 0o755))
			file := filepath.Join(target, "owned.txt")
			require.NoError(t, os.WriteFile(file, []byte("original"), 0o644))
			files, err := lockfile.Inventory(target)
			require.NoError(t, err)
			receipt := lockfile.New()
			receipt.Artifacts["owned"] = lockfile.Artifact{Name: "mock", Kind: "source", Target: "vendor", Files: files}
			// Write the receipt at the physical project root, independently of the fallback under test.
			require.NoError(t, lockfile.Save(&schema.AtmosConfiguration{BasePath: sourceDir}, receipt))
			before := readFile(t, filepath.Join(sourceDir, lockfile.DefaultFileName))
			if mode == "modified" {
				require.NoError(t, os.WriteFile(file, []byte("modified"), 0o644))
			}
			stderr := setupVendorUICapture(t)
			err = vendorCleanCmd.RunE(cmd, nil)
			if mode == "modified" {
				require.ErrorIs(t, err, errModifiedVendorFiles)
			} else {
				require.NoError(t, err)
			}
			output := plainOutput(stderr.String())
			switch mode {
			case "remove":
				assert.NoFileExists(t, file)
				assert.Contains(t, output, "Removed vendor/owned.txt")
			case "dry-run":
				assert.Equal(t, "original", readFile(t, file))
				assert.Contains(t, output, "Would remove vendor/owned.txt")
			case "modified":
				assert.Equal(t, "modified", readFile(t, file))
				assert.Contains(t, output, "Preserved modified vendor file vendor/owned.txt")
			}
			assert.NotContains(t, output, sourceDir, "display paths must be relative to the selected project root")
			assert.Equal(t, before, readFile(t, filepath.Join(sourceDir, lockfile.DefaultFileName)))
		})
	}
}
